// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package watcher owns the Docker-facing side of the processor: the container
// metadata cache, the event loop that keeps it fresh, and the on-miss inspect
// self-heal path.
package watcher // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"

import (
	"context"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	dfilters "github.com/docker/docker/api/types/filters"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// defaultReconnectBackoff is how long Run waits before re-subscribing after an
// events stream error.
const defaultReconnectBackoff = 3 * time.Second

// dockerAPI is the subset of the Docker SDK the watcher needs. *client.Client
// satisfies it directly.
type dockerAPI interface {
	ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error)
	ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error)
	Events(ctx context.Context, opts events.ListOptions) (<-chan events.Message, <-chan error)
}

// imageMatcher reports whether an image reference is excluded.
type imageMatcher interface {
	Matches(image string) bool
}

// fieldFilter maps a source key (label or env var name) to its target attribute
// name, or reports ok=false when the key is not selected.
type fieldFilter interface {
	Map(key string) (attrName string, ok bool)
}

// extractConfig is the resolved label/env extraction rules that build reads.
type extractConfig struct {
	labels  fieldFilter
	envVars fieldFilter
}

// Watcher keeps the cache fresh from the Docker events stream and self-heals
// cache misses with an inline inspect.
type Watcher struct {
	sdk     dockerAPI
	excl    imageMatcher
	cache   *cache
	cfg     extractConfig
	grace   time.Duration
	timeout time.Duration // per-call Docker API timeout; 0 = no timeout
	logger  *zap.Logger

	sf       singleflight.Group
	negCache *lru.LRU[string, time.Time] // id -> time it was cached as unknown/excluded

	reconnectBackoff time.Duration
}

// negCacheSize bounds the negative cache; negCacheTTL is how long an unknown or
// excluded id is remembered so a stream of distinct garbage ids costs one
// inspect each, not one per span. TTL is checked lazily on read (no background
// goroutine, nothing to leak).
const (
	negCacheSize = 1024
	negCacheTTL  = 10 * time.Second
)

func New(sdk dockerAPI, excl imageMatcher, c *cache, cfg extractConfig, grace, timeout time.Duration, logger *zap.Logger) *Watcher {
	return &Watcher{
		sdk:              sdk,
		excl:             excl,
		cache:            c,
		cfg:              cfg,
		grace:            grace,
		timeout:          timeout,
		logger:           logger,
		negCache:         lru.NewLRU[string, time.Time](negCacheSize, nil, 0),
		reconnectBackoff: defaultReconnectBackoff,
	}
}

// withTimeout derives a context bounded by the configured Docker API timeout. A
// zero timeout means no bound; the returned cancel is always safe to call.
func (w *Watcher) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if w.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, w.timeout)
}

// inspect calls ContainerInspect under the API timeout, logging and swallowing
// errors (ok=false).
func (w *Watcher) inspect(ctx context.Context, id string) (container.InspectResponse, bool) {
	ic, cancel := w.withTimeout(ctx)
	defer cancel()
	insp, err := w.sdk.ContainerInspect(ic, id)
	if err != nil {
		w.logger.Debug("container inspect failed", zap.String("id", id), zap.Error(err))
		return container.InspectResponse{}, false
	}
	return insp, true
}

// negCacheHas reports whether id was recently cached as unknown/excluded and the
// entry is still within TTL. Expired entries are dropped on access.
func (w *Watcher) negCacheHas(id string) bool {
	at, ok := w.negCache.Get(id)
	if !ok {
		return false
	}
	if time.Since(at) > negCacheTTL {
		w.negCache.Remove(id)
		return false
	}
	return true
}

// InitialSync lists running containers and warms the cache. Failures to inspect
// a single container are logged and skipped, not fatal.
func (w *Watcher) InitialSync(ctx context.Context) error {
	listCtx, cancel := w.withTimeout(ctx)
	list, err := w.sdk.ContainerList(listCtx, container.ListOptions{
		Filters: dfilters.NewArgs(dfilters.Arg("status", "running")),
	})
	cancel()
	if err != nil {
		return err
	}
	for i := range list {
		id := list[i].ID
		insp, ok := w.inspect(ctx, id)
		if !ok {
			continue
		}
		e, ok := build(&insp, w.cfg, w.excl)
		if !ok {
			continue
		}
		w.cache.put(e, stampOf(&insp))
		// A container listed as running may have exited before we inspected it.
		// Its die event fired before Run subscribed, so mark it expiring now
		if notActive(insp.State) {
			w.cache.markExpiring(id, time.Now().Add(w.grace))
		}
	}
	return nil
}

// Run subscribes to the events stream and drives the cache until ctx is done. On
// a stream error it waits reconnectBackoff and re-subscribes from the last seen
// event time.
func (w *Watcher) Run(ctx context.Context) {
	since := time.Time{}
	for {
		opts := events.ListOptions{Filters: eventFilter()}
		if !since.IsZero() {
			opts.Since = since.Format(time.RFC3339Nano)
		}
		evCh, errCh := w.sdk.Events(ctx, opts)

		for done := false; !done; {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-evCh:
				if !ok {
					done = true // stream closed → re-subscribe from `since`
					break
				}
				w.handleEvent(ctx, ev)
				if t := time.Unix(0, ev.TimeNano); t.After(since) {
					since = t
				}
			case err := <-errCh:
				if ctx.Err() != nil {
					return
				}
				w.logger.Debug("events stream error; reconnecting", zap.Error(err))
				done = true // re-subscribe from `since`
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(w.reconnectBackoff):
		}
	}
}

func (w *Watcher) handleEvent(ctx context.Context, ev events.Message) {
	id := ev.Actor.ID
	switch ev.Action {
	case "start", "unpause":
		insp, ok := w.inspect(ctx, id)
		if !ok {
			return
		}
		if e, ok := build(&insp, w.cfg, w.excl); ok {
			w.cache.put(e, ev.TimeNano)
		}
	case "rename":
		// Patch the name on a copy; keep any pending expiry (rename also fires on
		// stopped containers).
		if old, ok := w.cache.Get(id); ok {
			e := *old
			e.name = ev.Actor.Attributes["name"]
			w.cache.putKeepingExpiry(&e, ev.TimeNano)
		}
	case "die", "stop", "pause", "destroy":
		w.cache.markExpiring(id, time.Unix(0, ev.TimeNano).Add(w.grace))
	}
}

// inspectOnMiss inspects a container absent from the cache and stores it. It is
// the read-side self-heal for a container we started after our snapshot or whose
// event we missed.
func (w *Watcher) inspectOnMiss(ctx context.Context, id string) (*entry, bool) {
	if w.negCacheHas(id) {
		return nil, false
	}
	v, _, _ := w.sf.Do(id, func() (any, error) {
		insp, ok := w.inspect(ctx, id)
		if !ok {
			return nil, nil
		}
		return &insp, nil
	})
	if v == nil {
		w.negCache.Add(id, time.Now())
		return nil, false
	}
	insp := *v.(*container.InspectResponse)
	e, ok := build(&insp, w.cfg, w.excl)
	if !ok {
		w.negCache.Add(id, time.Now()) // excluded: don't re-inspect every span
		return nil, false
	}
	e.stamp = 0 // lowest: any event outranks an on-miss entry
	// A not-active result means we missed the exit event; give it a deadline so
	// it can't linger.
	if w.cache.putIfAbsentWithExpiry(e, notActive(insp.State), time.Now().Add(w.grace)) {
		return e, true
	}
	// An event populated it between our Get and here — use that entry.
	return w.cache.Get(id)
}

// stampOf returns a monotonic stamp for an initial-sync entry from the
// container's start time, or 0 when unavailable. Any real event outranks it.
func stampOf(insp *container.InspectResponse) int64 {
	if insp.State == nil || insp.State.StartedAt == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, insp.State.StartedAt)
	if err != nil {
		return 0
	}
	return t.UnixNano()
}

func eventFilter() dfilters.Args {
	f := dfilters.NewArgs()
	f.Add("type", "container")
	for _, a := range []string{"start", "unpause", "rename", "die", "stop", "pause", "destroy"} {
		f.Add("event", a)
	}
	return f
}
