// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package watcher // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"
)

// Config carries the parameters the Manager needs. Docker SDK construction is
// the caller's responsibility
type Config struct {
	ExcludedImages []string
	Labels         []FieldExtractConfig
	EnvVars        []FieldExtractConfig
	Grace          time.Duration
	// Timeout bounds each Docker API call; 0 means no timeout.
	Timeout time.Duration
}

// Manager owns the full lifecycle of the Docker-facing side: SDK client,
// matcher, cache, watcher, and cache janitor. The root processor holds a
// *Manager and calls Enrich for each resource.
type Manager struct {
	w      *Watcher
	cache  *cache
	logger *zap.Logger

	cancel context.CancelFunc
}

// janitorInterval controls how often the cache janitor sweeps expired entries.
const janitorInterval = 30 * time.Second

// NewManager builds a Manager from cfg.
func NewManager(sdk dockerAPI, cfg Config, logger *zap.Logger) (*Manager, error) {
	excl, err := NewImageMatcher(cfg.ExcludedImages)
	if err != nil {
		return nil, err
	}
	extract := ResolveExtract(cfg.Labels, cfg.EnvVars)
	c := newCache()
	w := New(sdk, excl, c, extract, cfg.Grace, cfg.Timeout, logger)
	return &Manager{
		w:      w,
		cache:  c,
		logger: logger,
	}, nil
}

// Start runs InitialSync, then launches the event loop and cache janitor as
// background goroutines. The returned context cancel is stored internally;
// Shutdown cancels it.
func (m *Manager) Start(ctx context.Context) error {
	if err := m.w.InitialSync(ctx); err != nil {
		return err
	}
	bgCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.w.Run(bgCtx)
	go m.cache.janitor(bgCtx, janitorInterval)
	return nil
}

// Shutdown cancels the background context, stopping Run and the janitor.
func (m *Manager) Shutdown(_ context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

// Enrich looks up id in the cache and, on a miss, performs an inline inspect.
// Found attributes are merged onto attrs without overwriting existing non-empty
// values (delegated to entry.writeTo). id must already be normalised (no
// docker:// prefix; exactly 64 hex characters).
func (m *Manager) Enrich(ctx context.Context, id string, attrs pcommon.Map) {
	e, ok := m.cache.Get(id)
	if !ok {
		e, ok = m.w.inspectOnMiss(ctx, id)
	}
	if ok && e != nil {
		e.writeTo(attrs)
	}
}
