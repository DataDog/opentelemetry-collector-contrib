# Stream B — Step 2: entry + attribute derivation

**Scope:** `internal/watcher/entry.go` (+ `entry_test.go`). Pure logic, **no Docker daemon**, no cache.
**Depends on:** Step 0 (frozen contracts). **Parallel with:** Stream A (config), Stream C (cache) — disjoint files.
**Gate (a+b):** `entry_test.go` passes under `go test ./... -race`; `make lint` clean; `go mod tidy` clean.
Remove the `//nolint:unused[,revive]` directives from every function you implement.

## What you fill in

Three functions, signatures already frozen in `entry.go`:

```go
func build(insp *container.InspectResponse, cfg extractConfig, excl imageMatcher) (e *entry, ok bool)
func (e *entry) writeTo(attrs pcommon.Map)
func notActive(s *container.State) bool
```

### `notActive(s *container.State) bool`
Return `!s.Running || s.Paused`. A paused container reports `Running==true && Paused==true`
(verified: moby `container/state.go:76`), so `!Running` alone misses it. Guard `s == nil` → treat as
not active (`true`).

### `build(insp, cfg, excl) (*entry, bool)`
Derive the immutable `entry` from an inspect response. **Exclusion first:** if
`excl.Matches(insp.Config.Image)` → return `(nil, false)` (buildOrSkip; applied on every inspect path).
Otherwise populate every field per the table below and return `(e, true)`. Do **not** set `stamp` here
(the caller stamps: event `TimeNano`, or 0 for on-miss).

Attribute derivation — **dockerstats-compat, verified against `receiver/dockerstatsreceiver/receiver.go`
(~line 148), NOT strict semconv** (DESIGN §3.3):

| entry field   | attribute            | source                              | notes |
|---------------|----------------------|-------------------------------------|-------|
| `id`          | `container.id`        | `insp.ID`                           | |
| `name`        | `container.name`      | `strings.TrimPrefix(insp.Name,"/")` | daemon prefixes `/` |
| `imageName`   | `container.image.name`| `insp.Config.Image` **verbatim**    | raw ref, may include tag/digest; do NOT parse/split |
| `imageID`     | `container.image.id`  | `insp.Image`                        | inspect field, NOT a parsed digest |
| `command`     | `container.command_line` | `strings.Join(insp.Config.Cmd," ")` | **Cmd only** (dockerstats parity); NOT Entrypoint+Cmd |
| `runtimeName` | `container.runtime.name` | constant `"docker"`              | current-semconv key |
| `imageTags`   | `container.image.tags` | best-effort tag via `ParseImageName`| **empty slice for digest-only ref; NEVER fabricate `latest`** |
| `labels`      | `container.label.*`   | `insp.Config.Labels` filtered by `cfg.labels.Map` | |
| `env`         | (mapped)              | `insp.Config.Env` filtered by `cfg.envVars.Map` | |

- **Image tags:** call `internal/common/docker.ParseImageName(insp.Config.Image)`. If it yields a tag,
  `imageTags = []string{tag}`. If the ref is digest-only (no tag) → `imageTags = nil`. Do **not** let
  `ParseImageName`'s default-to-`latest` leak in for a digest-only ref — check the ref has a tag
  section first, or strip a fabricated `latest` when the source had none. Add a test that pins this.
- **Env parsing:** split each `insp.Config.Env` entry with `strings.SplitN(v,"=",2)` (NOT
  `strings.Split` — it corrupts `KEY=a=b`; DESIGN §4.1.3). Key = part[0]; keep only if
  `cfg.envVars.Map(key)` returns ok; store under the mapped attr name. Empty-value vars: keep them
  (unlike the buggy shared helper which drops them).
- **Labels:** for each `insp.Config.Labels`, keep only if `cfg.labels.Map(key)` returns ok; store under
  the mapped name. If no labels selected, leave `labels` nil (not an empty map) — cheaper.

### `(e *entry) writeTo(attrs pcommon.Map)`
Copy pre-derived values onto the resource. **No-overwrite of non-empty** (DESIGN §1.5): before each
`PutStr`, check `attrs.Get(key)` — if present AND its `.Str() != ""`, skip (an existing empty string
IS overwritten). Scalars via `PutStr`. Skip empty scalar values entirely.

- `container.image.tags`: **only if `len(e.imageTags) > 0`** — `s := attrs.PutEmptySlice(...)`, then
  `s.AppendEmpty().SetStr(t)` per tag. Omit the attribute when empty. Respect the same
  no-overwrite-non-empty rule (if a non-empty tags slice already exists, skip).
- `labels`/`env`: range and `PutStr` each mapped entry, same no-overwrite rule.

## Test list (each must be a real assertion that fails if the logic is wrong)

`entry_test.go`, table-driven over synthetic `*container.InspectResponse`:

1. **Full running container** → asserts every attribute exact (id/name/image.name/image.id/command_line/runtime.name).
2. **Name `/`-trim** → `insp.Name="/foo"` → `container.name == "foo"`.
3. **Digest-only image** (`repo@sha256:...`) → `imageTags` empty, attribute omitted, **no `latest`**.
4. **Tagged image** (`repo:1.2`) → `imageTags == ["1.2"]`, emitted as a Slice.
5. **Excluded image** → `excl.Matches` true → `build` returns `(nil, false)`.
6. **Env with `=` in value** (`KEY=a=b=c`) → value == `a=b=c` (SplitN correctness).
7. **Env opt-in** → only configured keys present; unconfigured dropped; secret-looking key not leaked.
8. **Label filter** → only configured labels mapped; correct attr names.
9. **paused** (`Running=true,Paused=true`) → `notActive` true. **stopped** → true. **running** → false. **nil** → true.
10. **writeTo no-overwrite** → pre-populate `attrs` with a non-empty `container.name`; assert `writeTo`
    does NOT change it; but an existing EMPTY `container.name` IS overwritten.
11. **writeTo empty tags** → `imageTags==nil` → no `container.image.tags` key on the resource.

Use a fake `imageMatcher` (returns configurable bool) and a fake `fieldFilter` (map-backed) — both
trivial in-test structs implementing the frozen interfaces. Do not touch the cache or a real client.

## Notes
- `ParseImageName` is `github.com/open-telemetry/opentelemetry-collector-contrib/internal/common/docker`
  — add it to this module's require (tidy will pin it; it's already in the fork with a replace directive).
- Keep `entry` immutable: `build` returns a fresh `*entry`; never expose a mutator.

## Definition of done
1. Test list above all pass under `go test ./... -race`; `make lint` and `go mod tidy` clean.
2. `//nolint:unused[,revive]` removed from every function implemented.
3. **Codex review:** ask Codex to review this stream's diff. Address ALL P0 findings. If you disagree
   with a P0 (think it's wrong or not blocking), PUSH BACK to the user — do not silently comply or
   silently ignore. Non-P0: judgment. Not done until P0s are fixed or the user overrules them.
