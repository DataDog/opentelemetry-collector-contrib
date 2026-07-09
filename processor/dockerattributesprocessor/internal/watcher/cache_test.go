// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package watcher // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheGetMissThenHit(t *testing.T) {
	c := newCache()

	got, ok := c.Get("abc")
	assert.Nil(t, got)
	assert.False(t, ok)

	e := &entry{id: "abc", name: "web"}
	c.put(e, 1)

	got, ok = c.Get("abc")
	assert.True(t, ok)
	assert.Same(t, e, got, "Get must return the exact same pointer (zero-copy)")
}

func TestCachePutClearsExpiry(t *testing.T) {
	c := newCache()
	c.markExpiring("abc", time.Unix(100, 0))

	c.mu.RLock()
	_, marked := c.expiring["abc"]
	c.mu.RUnlock()
	require.True(t, marked)

	c.put(&entry{id: "abc"}, 1)

	c.mu.RLock()
	_, stillMarked := c.expiring["abc"]
	c.mu.RUnlock()
	assert.False(t, stillMarked, "put must clear a pending expiry (alive again)")
}

// stamp guard — older write dropped; equal stamp is last-writer-wins.
func TestCachePutStampGuard(t *testing.T) {
	c := newCache()

	e1 := &entry{id: "abc", name: "e1"}
	c.put(e1, 5)

	e2 := &entry{id: "abc", name: "e2"}
	c.put(e2, 3) // older: must drop
	got, _ := c.Get("abc")
	assert.Same(t, e1, got, "older stamp must not overwrite")

	e3 := &entry{id: "abc", name: "e3"}
	c.put(e3, 5) // equal stamp: >= is inclusive, last-writer wins
	got, _ = c.Get("abc")
	assert.Same(t, e3, got, "equal stamp must overwrite (>= inclusive)")
}

// putKeepingExpiry preserves the deadline (rename-un-expires regression guard).
func TestCachePutKeepingExpiryPreservesDeadline(t *testing.T) {
	c := newCache()
	deadline := time.Unix(500, 0)

	c.put(&entry{id: "abc", name: "old"}, 5)
	c.markExpiring("abc", deadline)

	renamed := &entry{id: "abc", name: "new"}
	c.putKeepingExpiry(renamed, 6)

	got, ok := c.Get("abc")
	require.True(t, ok)
	assert.Same(t, renamed, got, "entry must be updated by putKeepingExpiry")

	c.mu.RLock()
	at, marked := c.expiring["abc"]
	c.mu.RUnlock()
	require.True(t, marked, "putKeepingExpiry must NOT clear expiry (would un-expire stopped container)")
	assert.Equal(t, deadline, at, "the deadline must be untouched")
}

// putKeepingExpiry stamp guard — older stamp dropped, expiry untouched.
func TestCachePutKeepingExpiryStampGuard(t *testing.T) {
	c := newCache()
	deadline := time.Unix(500, 0)

	e1 := &entry{id: "abc", name: "e1"}
	c.put(e1, 5)
	c.markExpiring("abc", deadline)

	c.putKeepingExpiry(&entry{id: "abc", name: "e2"}, 3) // older: drop

	got, _ := c.Get("abc")
	assert.Same(t, e1, got, "older stamp must not overwrite via putKeepingExpiry")

	c.mu.RLock()
	at, marked := c.expiring["abc"]
	c.mu.RUnlock()
	require.True(t, marked)
	assert.Equal(t, deadline, at, "expiry must remain untouched even on dropped write")
}

func TestCachePutIfAbsent(t *testing.T) {
	c := newCache()
	future := time.Unix(1<<40, 0)

	e1 := &entry{id: "abc", name: "first"}
	assert.True(t, c.putIfAbsentWithExpiry(e1, false, future), "absent id must be inserted")
	got, _ := c.Get("abc")
	assert.Same(t, e1, got)

	e2 := &entry{id: "abc", name: "second"}
	assert.False(t, c.putIfAbsentWithExpiry(e2, false, future), "present id must not be inserted")
	got, _ = c.Get("abc")
	assert.Same(t, e1, got, "must not clobber an existing entry")
}

// putIfAbsentWithExpiry clears a stale deadline when inserting a running entry,
// and sets one when inserting a not-active entry.
func TestCachePutIfAbsentWithExpiry_Deadline(t *testing.T) {
	deadline := time.Unix(500, 0)

	t.Run("running clears stale deadline", func(t *testing.T) {
		c := newCache()
		c.markExpiring("abc", deadline) // stale deadline for an uncached id (e.g. a die we saw)
		c.putIfAbsentWithExpiry(&entry{id: "abc"}, false, deadline)
		c.mu.RLock()
		_, marked := c.expiring["abc"]
		c.mu.RUnlock()
		assert.False(t, marked, "running insert must clear a stale deadline")
	})

	t.Run("not-active sets deadline", func(t *testing.T) {
		c := newCache()
		c.putIfAbsentWithExpiry(&entry{id: "abc"}, true, deadline)
		c.mu.RLock()
		at := c.expiring["abc"]
		c.mu.RUnlock()
		assert.Equal(t, deadline, at, "not-active insert must set the deadline")
	})
}

// A past deadline is evicted on the next tick; a far-future one is left in place.
// Drives the real janitor rather than a copy of its logic.
func TestCacheJanitorEvictsAtDeadline(t *testing.T) {
	c := newCache()

	c.put(&entry{id: "expired", name: "x"}, 1)
	c.markExpiring("expired", time.Unix(0, 0)) // past deadline → must be evicted

	c.put(&entry{id: "alive", name: "y"}, 1)
	c.markExpiring("alive", time.Now().Add(time.Hour)) // future deadline → must survive

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go c.janitor(ctx, time.Millisecond)

	// The past-due entry is evicted from BOTH maps.
	require.Eventually(t, func() bool {
		_, ok := c.Get("expired")
		if ok {
			return false
		}
		c.mu.RLock()
		_, marked := c.expiring["expired"]
		c.mu.RUnlock()
		return !marked
	}, time.Second, time.Millisecond, "past-due entry must be evicted from byID and expiring")

	// The future-deadline entry survives every tick in that window.
	_, ok := c.Get("alive")
	assert.True(t, ok, "entry with a future deadline must not be evicted")
}

func TestCacheJanitorCtxCancel(t *testing.T) {
	c := newCache()
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() {
		c.janitor(ctx, time.Hour)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("janitor did not return after ctx cancel (goroutine leak)")
	}
}

func TestCacheConcurrency(t *testing.T) {
	c := newCache()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go c.janitor(ctx, time.Millisecond)

	const workers = 16
	const iters = 500
	ids := []string{"a", "b", "c", "d"}

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := range workers {
		go func(w int) {
			defer wg.Done()
			for i := range iters {
				id := ids[(w+i)%len(ids)]
				switch i % 4 {
				case 0:
					c.put(&entry{id: id}, int64(i))
				case 1:
					c.putIfAbsentWithExpiry(&entry{id: id}, false, time.Now().Add(time.Minute))
				case 2:
					c.markExpiring(id, time.Now().Add(time.Millisecond))
				case 3:
					c.Get(id)
				}
			}
		}(w)
	}
	wg.Wait()

	// The real gate is the race detector reporting no data race during the
	// concurrent churn above. Post-run, assert the cache is still internally
	// consistent and usable: entries present in byID have a matching id, and Get
	// agrees with the map. (An entry may or may not be present depending on
	// interleaving, so we don't assert presence — only consistency.)
	c.mu.RLock()
	for id, e := range c.byID {
		assert.Equal(t, id, e.id, "byID key must match the stored entry's id")
	}
	c.mu.RUnlock()
	for _, id := range ids {
		if e, ok := c.Get(id); ok {
			assert.Equal(t, id, e.id, "Get(%q) must return an entry with matching id", id)
		}
	}
}
