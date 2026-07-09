# Steps 4–7 — serial spine (after A/B/C merge)

These are NOT parallel; each depends on prior work. Same branch, same per-step gate
(behavioral tests under `-race` + `make lint` + `go mod tidy` clean).

**Every step here also passes the Codex review gate before it's done:** after the a+b gate, ask Codex
to review the step's diff, address ALL P0 findings, and PUSH BACK to the user on any P0 you disagree
with (don't silently comply or ignore). Not done until P0s are resolved or overruled.

## Step 4 — watcher (needs B entry + C cache + the dockerAPI seam)

**Scope:** `internal/watcher/watcher.go` (fill bodies), `watcher_test.go` (fake `dockerAPI`).

Implement:
- `New(sdk dockerAPI, excl imageMatcher, c *cache, cfg extractConfig, grace time.Duration, log) *watcher`
- `InitialSync(ctx)` — `ContainerList(status=running)` → per id `ContainerInspect` → `buildOrSkip` →
  `put(e, stampFromStartedAt-or-0)`. Apply exclusion (buildOrSkip already does).
- `Run(ctx)` — event loop: `Events(since=lastTime)`, reconnect w/ 3s backoff + `lastTime` resume
  (reference `internal/docker.ContainerEventLoop`, re-implemented). Handle actions per DESIGN §3.4:
  - `start`/`unpause`: `buildOrSkip(inspect(id))` → `put(e, ev.TimeNano)`
  - `rename`: `Get(id)`; if ok, copy, set `name = attrs["name"]`, `putKeepingExpiry(&copy, ev.TimeNano)`
  - `die`/`stop`/`pause`/`destroy`: `markExpiring(id, now+grace)`
  - `update`: ignored
- `inspectOnMiss(ctx, id) (*entry, bool)` — negCache check → singleflight `Do` → `buildOrSkip` →
  `putIfAbsent`; if inserted && `notActive` → `markExpiring(id, now+grace)`; if not inserted → `Get`.
  singleflight = `golang.org/x/sync/singleflight`. negCache = `hashicorp/golang-lru/v2/expirable`
  `LRU[string, struct{}]` (TTL + size cap). **No inspect concurrency cap** (DESIGN §3.4 / finding #6).

**Tests (fake dockerAPI, scripted events; `-race`):**
- InitialSync warms cache; excluded image skipped.
- start→rename→both interleavings vs. an in-flight on-miss end on new name (the stale-write race).
- missed-destroy: on-miss of a not-running container → markExpiring set.
- exclusion applied on event AND on-miss AND initial-sync paths.
- notActive(paused) caught.
- reconnect resumes from lastTime (fake returns error then a new channel).

## Step 5 — processor wiring (needs A config + Step 4)

**Scope:** `processor.go` (replace no-op bodies), `processor_test.go`.
- `Start`: open own SDK handle (`docker.NewClientWithOpts` w/ endpoint, version, our User-Agent —
  NOT internal/docker.Client), build `imageMatcher` + `extractConfig` from Config, `newCache()`,
  `watcher.New(...)`, `InitialSync`, `go Run`, `go janitor`.
- `Shutdown`: cancel background ctx.
- `processResource`: `normalizeID` (strip `docker://`, require 64-char; else return) → `Get` →
  on miss `inspectOnMiss` → `writeTo`.
- `processTraces/Metrics/Logs/Profiles`: iterate resources → `processResource`.

**Tests (fake client + pre-warmed cache):** enrich each signal incl. profiles; `docker://` strip;
short-id → clean no-op; no-overwrite of existing non-empty attrs.

## Step 6 — integration (needs Step 5)

**Scope:** `integration_test.go` (build tag `integration`). `testcontainers` spins a real container;
assert live enrichment + one event-driven mutation (rename or stop→grace). Runs in the
`integration-test` make lane; skipped in normal `go test`.

## Step 7 — docs + register

**Scope:** finalize README (config reference, supported attributes, documented limitations: short-id,
stale-hit per DESIGN §4.4), add to the fork's distribution builder manifest + `versions.yaml` +
`.github/CODEOWNERS`. Gate: distro build includes the component; existing tests pass in-distro.
