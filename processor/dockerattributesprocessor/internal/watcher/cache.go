// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package watcher // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"

import (
	"context"
	"sync"
	"time"
)

// cache holds container metadata keyed by container.id. byID is the store;
// expiring holds delete-after deadlines for exited containers (grace period).
// Stored *entry values are immutable: writers replace the map slot, never mutate
// in place, so Get can hand out the pointer without copying.
type cache struct {
	mu       sync.RWMutex
	byID     map[string]*entry
	expiring map[string]time.Time
}

func newCache() *cache {
	return &cache{
		byID:     make(map[string]*entry),
		expiring: make(map[string]time.Time),
	}
}

// Get returns the entry for id. The returned pointer must not be mutated.
func (c *cache) Get(id string) (*entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.byID[id]
	return e, ok
}

// store writes e under the stamp guard: a write older than the stored entry is
// dropped, guarding against stale or out-of-order writes. Caller holds c.mu.
// Returns whether the write was applied.
func (c *cache) store(e *entry, stamp int64) bool {
	if cur := c.byID[e.id]; cur != nil && stamp < cur.stamp {
		return false
	}
	e.stamp = stamp
	c.byID[e.id] = e
	return true
}

// put stores e and clears any pending expiry — the container is alive.
func (c *cache) put(e *entry, stamp int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.store(e, stamp) {
		delete(c.expiring, e.id)
	}
}

// putKeepingExpiry stores e but leaves any deadline in place. Rename uses this:
// Docker emits rename for stopped containers too, and clearing expiry would keep
// a dead container cached forever.
func (c *cache) putKeepingExpiry(e *entry, stamp int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store(e, stamp)
}

// putIfAbsentWithExpiry stores e only if id is absent, reporting whether it
// inserted. On insert it also sets or clears the deadline atomically: a
// not-active container gets deadline; a running one has any stale deadline
// cleared. Doing both under one lock prevents a stale deadline (e.g. from a die
// event for an id we hadn't cached) from surviving a running insert. The on-miss
// path uses this so it never clobbers a fresher event-sourced entry.
func (c *cache) putIfAbsentWithExpiry(e *entry, notActive bool, deadline time.Time) (inserted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.byID[e.id]; exists {
		return false
	}
	c.byID[e.id] = e
	if notActive {
		c.expiring[e.id] = deadline
	} else {
		delete(c.expiring, e.id)
	}
	return true
}

// markExpiring sets a delete-after deadline for id without deleting immediately.
func (c *cache) markExpiring(id string, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expiring[id] = at
}

// janitor evicts entries past their deadline every interval until ctx is done.
func (c *cache) janitor(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.mu.Lock()
			for id, at := range c.expiring {
				if !now.Before(at) {
					delete(c.byID, id)
					delete(c.expiring, id)
				}
			}
			c.mu.Unlock()
		}
	}
}
