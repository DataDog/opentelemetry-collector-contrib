// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package watcher // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// fakeDocker is a scriptable dockerAPI. inspects maps id -> response; list is
// returned by ContainerList; eventBatches are served one channel per Events call
// (each followed by an error to trigger reconnect, unless it's the last).
type fakeDocker struct {
	mu           sync.Mutex
	inspects     map[string]container.InspectResponse
	inspectErr   map[string]error
	inspectCalls map[string]int
	list         []container.Summary
}

func newFakeDocker() *fakeDocker {
	return &fakeDocker{
		inspects:     map[string]container.InspectResponse{},
		inspectErr:   map[string]error{},
		inspectCalls: map[string]int{},
	}
}

func (f *fakeDocker) setInspect(id, name, image, imageID string, running, paused bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspects[id] = container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    id,
			Name:  name,
			Image: imageID,
			State: &container.State{Running: running, Paused: paused},
		},
		Config: &container.Config{Image: image},
	}
}

func (f *fakeDocker) ContainerInspect(_ context.Context, id string) (container.InspectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectCalls[id]++
	if err := f.inspectErr[id]; err != nil {
		return container.InspectResponse{}, err
	}
	insp, ok := f.inspects[id]
	if !ok {
		return container.InspectResponse{}, errors.New("no such container")
	}
	return insp, nil
}

func (f *fakeDocker) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list, nil
}

func (f *fakeDocker) callCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inspectCalls[id]
}

// Events satisfies dockerAPI for tests that don't exercise Run; the ones that do
// use runFake, which overrides it.
func (*fakeDocker) Events(_ context.Context, _ events.ListOptions) (<-chan events.Message, <-chan error) {
	return make(chan events.Message), make(chan error)
}

func testWatcher(t *testing.T, f dockerAPI, excl imageMatcher) *Watcher {
	return New(f, excl, newCache(), extractConfig{}, time.Minute, 0, zaptest.NewLogger(t))
}

type allowAll struct{}

func (allowAll) Matches(string) bool { return false }

type excludeImage struct{ img string }

func (e excludeImage) Matches(image string) bool { return image == e.img }

func TestInitialSync_WarmsCacheAndSkipsExcluded(t *testing.T) {
	f := newFakeDocker()
	f.list = []container.Summary{{ID: "keep"}, {ID: "drop"}}
	f.setInspect("keep", "/keep", "nginx:1", "sha256:a", true, false)
	f.setInspect("drop", "/drop", "excluded:1", "sha256:b", true, false)

	w := New(f, excludeImage{img: "excluded:1"}, newCache(), extractConfig{}, time.Minute, 0, zaptest.NewLogger(t))
	require.NoError(t, w.InitialSync(t.Context()))

	got, ok := w.cache.Get("keep")
	require.True(t, ok)
	assert.Equal(t, "keep", got.name)

	_, ok = w.cache.Get("drop")
	assert.False(t, ok, "excluded image must not be cached")
}

func TestHandleEvent_StartThenRename(t *testing.T) {
	f := newFakeDocker()
	f.setInspect("c1", "/old", "nginx:1", "sha256:a", true, false)
	w := testWatcher(t, f, allowAll{})

	w.handleEvent(t.Context(), events.Message{
		Action: "start", TimeNano: 100,
		Actor: events.Actor{ID: "c1"},
	})
	got, ok := w.cache.Get("c1")
	require.True(t, ok)
	assert.Equal(t, "old", got.name)

	w.handleEvent(t.Context(), events.Message{
		Action: "rename", TimeNano: 200,
		Actor: events.Actor{ID: "c1", Attributes: map[string]string{"name": "new"}},
	})
	got, ok = w.cache.Get("c1")
	require.True(t, ok)
	assert.Equal(t, "new", got.name, "rename must update the name")
}

func TestHandleEvent_ExitMarksExpiring(t *testing.T) {
	f := newFakeDocker()
	f.setInspect("c1", "/c", "nginx:1", "sha256:a", true, false)
	w := testWatcher(t, f, allowAll{})

	w.handleEvent(t.Context(), events.Message{Action: "start", TimeNano: 100, Actor: events.Actor{ID: "c1"}})
	const dieNano = 200
	w.handleEvent(t.Context(), events.Message{Action: "die", TimeNano: dieNano, Actor: events.Actor{ID: "c1"}})

	w.cache.mu.RLock()
	at, marked := w.cache.expiring["c1"]
	w.cache.mu.RUnlock()
	require.True(t, marked, "exit event must set a deadline")
	// Deadline is derived from the EVENT time, not wall-clock.
	assert.Equal(t, time.Unix(0, dieNano).Add(w.grace), at)
}

func TestInspectOnMiss_RunningCachesNoDeadline(t *testing.T) {
	f := newFakeDocker()
	f.setInspect("c1", "/c", "nginx:1", "sha256:a", true, false)
	w := testWatcher(t, f, allowAll{})

	e, ok := w.inspectOnMiss(t.Context(), "c1")
	require.True(t, ok)
	assert.Equal(t, "c1", e.id)
	assert.Equal(t, int64(0), e.stamp, "on-miss entry has the lowest stamp")

	w.cache.mu.RLock()
	_, marked := w.cache.expiring["c1"]
	w.cache.mu.RUnlock()
	assert.False(t, marked, "a running on-miss must not set a deadline")
}

func TestInspectOnMiss_NotRunningMarksExpiring(t *testing.T) {
	f := newFakeDocker()
	f.setInspect("c1", "/c", "nginx:1", "sha256:a", false, false) // stopped
	w := testWatcher(t, f, allowAll{})

	before := time.Now()
	_, ok := w.inspectOnMiss(t.Context(), "c1")
	require.True(t, ok)
	assertOnMissDeadline(t, w, "c1", before)
}

func TestInspectOnMiss_PausedMarksExpiring(t *testing.T) {
	f := newFakeDocker()
	f.setInspect("c1", "/c", "nginx:1", "sha256:a", true, true) // paused: Running && Paused
	w := testWatcher(t, f, allowAll{})

	before := time.Now()
	_, ok := w.inspectOnMiss(t.Context(), "c1")
	require.True(t, ok)
	assertOnMissDeadline(t, w, "c1", before)
}

// assertOnMissDeadline checks id has a deadline in (before+grace, now+grace].
// On-miss uses wall-clock, so a window check is the right assertion.
func assertOnMissDeadline(t *testing.T, w *Watcher, id string, before time.Time) {
	t.Helper()
	w.cache.mu.RLock()
	at, marked := w.cache.expiring[id]
	w.cache.mu.RUnlock()
	require.True(t, marked, "not-active on-miss must set a deadline (missed-exit guard)")
	assert.False(t, at.Before(before.Add(w.grace)), "deadline must be >= before+grace")
	assert.False(t, at.After(time.Now().Add(w.grace)), "deadline must be <= now+grace")
}

func TestInspectOnMiss_ExcludedNegativeCached(t *testing.T) {
	f := newFakeDocker()
	f.setInspect("c1", "/c", "excluded:1", "sha256:a", true, false)
	w := testWatcher(t, f, excludeImage{img: "excluded:1"})

	_, ok := w.inspectOnMiss(t.Context(), "c1")
	assert.False(t, ok)
	_, ok = w.inspectOnMiss(t.Context(), "c1")
	assert.False(t, ok)
	assert.Equal(t, 1, f.callCount("c1"), "excluded id must be negative-cached, inspected once")
}

func TestInspectOnMiss_UnknownNegativeCached(t *testing.T) {
	f := newFakeDocker() // no inspect registered → error
	w := testWatcher(t, f, allowAll{})

	_, ok := w.inspectOnMiss(t.Context(), "ghost")
	assert.False(t, ok)
	_, ok = w.inspectOnMiss(t.Context(), "ghost")
	assert.False(t, ok)
	assert.Equal(t, 1, f.callCount("ghost"), "unknown id must be negative-cached, inspected once")
}

func TestInspectOnMiss_DefersToEventEntry(t *testing.T) {
	f := newFakeDocker()
	f.setInspect("c1", "/from-inspect", "nginx:1", "sha256:a", true, false)
	w := testWatcher(t, f, allowAll{})

	// An event-sourced entry already exists (stamp 100).
	w.cache.put(&entry{id: "c1", name: "from-event"}, 100)

	e, ok := w.inspectOnMiss(t.Context(), "c1")
	require.True(t, ok)
	assert.Equal(t, "from-event", e.name, "on-miss must not clobber an event-sourced entry")
}

// runFake serves one batch per Events call, signaling a broken stream after
// each so Run re-subscribes — verifying it resumes and keeps consuming.
type runFake struct {
	*fakeDocker
	mu        sync.Mutex
	batches   [][]events.Message
	call      int
	sinceGot  []string
	closeEvCh bool
}

// closeEvCh selects how runFake signals a broken stream: false (default) uses
// Docker's real semantics — send an error then close errCh, leaving evCh open;
// true closes evCh instead, to cover Run's closed-channel branch.
func (r *runFake) Events(ctx context.Context, opts events.ListOptions) (<-chan events.Message, <-chan error) {
	r.mu.Lock()
	r.sinceGot = append(r.sinceGot, opts.Since)
	var batch []events.Message
	if r.call < len(r.batches) {
		batch = r.batches[r.call]
	}
	r.call++
	closeEv := r.closeEvCh
	r.mu.Unlock()

	ev := make(chan events.Message)
	er := make(chan error)
	// Blocking sends on unbuffered channels: each event is fully consumed by Run
	// before the reconnect signal, so there is no evCh/errCh select race.
	go func() {
		for i := range batch {
			select {
			case ev <- batch[i]:
			case <-ctx.Done():
				return
			}
		}
		if closeEv {
			close(ev)
			return
		}
		select {
		case er <- errors.New("stream closed"): // Docker: error on errCh, evCh stays open
		case <-ctx.Done():
		}
	}()
	return ev, er
}

func runReconnectTest(t *testing.T, closeEvCh bool) {
	fd := newFakeDocker()
	fd.setInspect("c1", "/c1", "nginx:1", "sha256:a", true, false)
	fd.setInspect("c2", "/c2", "nginx:1", "sha256:b", true, false)
	r := &runFake{
		fakeDocker: fd,
		closeEvCh:  closeEvCh,
		batches: [][]events.Message{
			{{Action: "start", TimeNano: 1000, Actor: events.Actor{ID: "c1"}}},
			{{Action: "start", TimeNano: 2000, Actor: events.Actor{ID: "c2"}}},
		},
	}
	w := New(r, allowAll{}, newCache(), extractConfig{}, time.Minute, 0, zaptest.NewLogger(t))
	w.reconnectBackoff = time.Millisecond

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	require.Eventually(t, func() bool {
		_, a := w.cache.Get("c1")
		_, b := w.cache.Get("c2")
		return a && b
	}, 2*time.Second, 5*time.Millisecond, "both events across a reconnect must be consumed")

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	require.GreaterOrEqual(t, len(r.sinceGot), 2, "Run must have re-subscribed")
	assert.Empty(t, r.sinceGot[0], "first subscribe has no since")
	// Resume from the last consumed event (c1 @ TimeNano 1000), exact RFC3339Nano.
	assert.Equal(t, time.Unix(0, 1000).Format(time.RFC3339Nano), r.sinceGot[1])
}

func TestRun_ReconnectViaErrChResumesFromLastTime(t *testing.T) {
	runReconnectTest(t, false) // Docker's real error-channel path
}

func TestRun_ReconnectViaClosedEvCh(t *testing.T) {
	runReconnectTest(t, true) // closed message-channel path
}
