# Stream C — Step 3: cache

**Scope:** `internal/watcher/cache.go` (+ `cache_test.go`). Pure in-memory, **no Docker daemon**, no entry derivation.
**Depends on:** Step 0 (frozen contracts — uses the `entry` type by pointer only; does NOT derive it).
**Parallel with:** Stream A (config), Stream B (entry) — disjoint files.
**Gate (a+b):** `cache_test.go` passes under `go test ./... -race` (concurrency tests mandatory);
`make lint` clean; `go mod tidy` clean. Remove the `//nolint` directives from every method you implement.

## What you fill in

Frozen in `cache.go` (struct + method set fixed):

```go
type cache struct {
    mu       sync.RWMutex
    byID     map[string]*entry
    expiring map[string]time.Time
}
func newCache() *cache
func (c *cache) Get(id string) (*entry, bool)
func (c *cache) put(e *entry, stamp int64)
func (c *cache) putKeepingExpiry(e *entry, stamp int64)
func (c *cache) putIfAbsent(e *entry) (inserted bool)
func (c *cache) markExpiring(id string, at time.Time)
func (c *cache) janitor(ctx context.Context, interval time.Duration)
```

### Invariants (DESIGN Case B + §3.4)
- Entries are **immutable**. Only the map slot is reassigned; never mutate a stored `*entry`.
- `Get` is **read-only** — RLock, return the pointer, no writes to entry or cache state. This is what
  makes reads zero-copy.
- The `stamp` guard is the k8s `StartTime` pattern: a write is applied only if `stamp >= stored.stamp`.

### `newCache()`
Return `&cache{byID: map, expiring: map}` (both initialized).

### `Get(id)` — RLock; `e, ok := c.byID[id]`; return. No mutation.

### `put(e, stamp)` — Lock. Overwrite guard, then CLEAR expiry:
```
cur := c.byID[e.id]
if cur != nil && stamp < cur.stamp { return }   // older write: drop
e.stamp = stamp
c.byID[e.id] = e
delete(c.expiring, e.id)                         // alive again → no deadline
```
Used by start/unpause and startup.

### `putKeepingExpiry(e, stamp)` — Lock. Same overwrite guard, but do **NOT** touch `expiring`:
```
cur := c.byID[e.id]
if cur != nil && stamp < cur.stamp { return }
e.stamp = stamp
c.byID[e.id] = e
// expiring[e.id] left as-is
```
Rename uses this: Docker emits rename for stopped/paused containers too; clearing expiry would
un-expire them (pin forever). **This is a High-severity correctness point — test it explicitly.**

### `putIfAbsent(e) bool` — Lock. Insert only if absent:
```
if _, exists := c.byID[e.id]; exists { return false }
c.byID[e.id] = e   // caller already set e.stamp = 0
return true
```
On-miss path; never clobbers an event-sourced entry.

### `markExpiring(id, at)` — Lock. `c.expiring[id] = at`. (No immediate delete. Don't require the id to
be present — a deadline for an absent id is harmless and simplest.)

### `janitor(ctx, interval)` — ticker loop (k8s deleteLoop equivalent):
```
ticker := time.NewTicker(interval); defer ticker.Stop()
for {
  select {
  case <-ctx.Done(): return
  case now := <-ticker.C:
     Lock; for id, at := range c.expiring {
        if !now.Before(at) { delete(c.byID, id); delete(c.expiring, id) }
     }; Unlock
  }
}
```
Inject `now` via the ticker value so tests are deterministic (no wall-clock). Do NOT call
`time.Now()` inside — use the tick time.

## Test list (real assertions; `-race` mandatory)

`cache_test.go`. Construct `*entry` literals directly (Stream B's `build` not needed — just `&entry{id:...}`):

1. **Get miss / hit** → absent id → `(nil,false)`; after put → `(e,true)`, same pointer.
2. **put clears expiry** → markExpiring(id); put(id) → `expiring[id]` gone.
3. **stamp guard** → put(e1,stamp=5); put(e2,stamp=3) → stored is e1 (older dropped); put(e3,stamp=5)
   → e3 wins (>= is inclusive, last-writer-at-equal-stamp).
4. **putKeepingExpiry preserves deadline** → markExpiring(id,T); putKeepingExpiry(renamed,stamp) →
   entry updated AND `expiring[id]==T` still set. (The rename-un-expires regression guard.)
5. **putKeepingExpiry stamp guard** → older stamp dropped, expiry still untouched.
6. **putIfAbsent** → absent → inserts, returns true; present → returns false, does NOT overwrite.
7. **janitor evicts at deadline** → put + markExpiring(id, tick-1ns before); tick at/after deadline →
   entry gone from byID AND expiring. Tick before deadline → entry stays.
8. **janitor ctx cancel** → cancel ctx → janitor returns (no goroutine leak; assert via a done channel).
9. **CONCURRENCY (-race):** N goroutines doing Get + put + markExpiring on overlapping ids while the
   janitor runs; assert no race, no panic, and final state consistent. This is the core gate.

## Notes
- Do not add a `lastSeen`/idle rule — eviction is purely `expiring`-driven (matches k8s; DESIGN Case C/D).
- `janitor` iterating `expiring` under Lock is O(expiring), fine — expiring is small (only exited containers).
- Keep everything unexported; the watcher (Step 4) is the only caller.

## Definition of done
1. Test list above all pass under `go test ./... -race` (concurrency test mandatory); `make lint` and
   `go mod tidy` clean.
2. `//nolint:unused[,revive]` removed from every method implemented.
3. **Codex review:** ask Codex to review this stream's diff. Address ALL P0 findings. If you disagree
   with a P0 (think it's wrong or not blocking), PUSH BACK to the user — do not silently comply or
   silently ignore. Non-P0: judgment. Not done until P0s are fixed or the user overrules them.
