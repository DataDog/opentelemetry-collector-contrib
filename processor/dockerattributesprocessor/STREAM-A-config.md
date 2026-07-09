# Stream A — Step 1: config + factory + extractConfig resolution

**Scope:** `config.go`, `config_test.go`, `factory.go` (fill defaults), `factory_test.go`,
`testdata/config.yaml`, and the `extractConfig`/`fieldFilter`/`imageMatcher` **implementations** the
watcher package declared as interfaces. No Docker daemon (factory creates processors; doesn't connect).
**Depends on:** Step 0. **Parallel with:** B (entry), C (cache) — disjoint files. This stream owns the
public config surface and the resolution INTO the frozen `watcher.extractConfig`.
**Gate (a+b):** config/factory tests pass under `-race`; `make lint`; `go mod tidy` clean.

## Public Config (config.go — replace the empty Step 0 stub)

Embed `internal/docker.Config` and add our fields (DESIGN §3.2/§3.3):

```go
type Config struct {
    docker.Config      `mapstructure:",squash"` // Endpoint, Timeout, ExcludedImages, DockerAPIVersion
    Extract            ExtractConfig       `mapstructure:"extract"`
    Association        []AssociationConfig `mapstructure:"association"` // v1: id-only wiring
    DeleteGracePeriod  time.Duration       `mapstructure:"delete_grace_period"` // default 120s
}

type ExtractConfig struct {
    Metadata []string             `mapstructure:"metadata"`  // which container.* keys (default = full fixed set)
    Labels   []FieldExtractConfig `mapstructure:"labels"`
    EnvVars  []FieldExtractConfig `mapstructure:"env_vars"`  // opt-in per key
}

type FieldExtractConfig struct { // mirror k8sattributes shape
    Key     string `mapstructure:"key"`
    KeyRegex string `mapstructure:"key_regex"`
    TagName string `mapstructure:"tag_name"` // target attr name (default derived)
}

type AssociationConfig struct { // config shape adopted from k8s; v1 wires only container.id
    From string `mapstructure:"from"` // "resource_attribute"
    Name string `mapstructure:"name"` // "container.id"
}
```

### Validate()
- `docker.Config.Validate()` (endpoint non-empty) — delegate.
- `DeleteGracePeriod >= 0`.
- Each `FieldExtractConfig`: not both `Key` and `KeyRegex`; `KeyRegex` compiles.
- Each `AssociationConfig`: `From` in the allowed set; v1 only supports
  `{from: resource_attribute, name: container.id}` — reject other sources with a clear "not supported
  in v1" error (DESIGN §3.2). Empty association slice is allowed (defaults to id).

### createDefaultConfig() (factory.go)
```go
&Config{
    Config:            *docker.NewDefaultConfig(), // unix socket, 5s timeout
    DeleteGracePeriod: 120 * time.Second,
    Extract:           ExtractConfig{Metadata: defaultMetadataKeys()},
}
```
`defaultMetadataKeys()` = the fixed set from DESIGN §3.3 (id, name, image.name, image.tags, image.id,
runtime.name, command_line). Association left empty → treated as id-only.

## Resolution into the frozen contract (the key deliverable)

**DECIDED: the resolver lives INSIDE the `watcher` package** (new file `watcher/resolve.go`), so
nothing gets exported and the frozen contract is unchanged — B and C never rebase. `resolve.go` is
disjoint from B's `entry.go` and C's `cache.go`, so it's still conflict-free on the shared branch.

Stream A adds to the watcher package:

```go
// watcher/resolve.go  (Stream A owns this file)

// ResolveExtract compiles the public config's label/env rules into the internal
// extractConfig that build() reads. Takes the PUBLIC config types as input so the
// root package doesn't need the unexported types.
func ResolveExtract(labels, envVars []FieldExtractConfigInput) extractConfig { ... }

// FieldExtractConfigInput mirrors the public FieldExtractConfig (Key/KeyRegex/TagName)
// but declared here so watcher has no import of the root package (avoids a cycle).
type FieldExtractConfigInput struct{ Key, KeyRegex, TagName string }

// concrete fieldFilter impls (unexported), built by ResolveExtract:
type exactFilter map[string]string          // src key -> attr name
type regexFilter struct{ re *regexp.Regexp; tagTmpl string }
// both satisfy the frozen fieldFilter interface via Map(key) (string, bool)
```

Root-package config code calls `watcher.ResolveExtract(...)` (mapping its `[]FieldExtractConfig` to
`[]watcher.FieldExtractConfigInput`) to get the `extractConfig` it hands to `watcher.New` in Step 5.

Concrete `fieldFilter` behavior:
- **exact-key** (`exactFilter`): `map[string]string` src→tag; `Map` is a lookup. Default tag name when
  `TagName` empty (e.g. `container.label.<key>` / the env attr convention).
- **regex** (`regexFilter`): compiled `re` + tag template; `Map` matches key, derives name.

Also add `watcher.NewImageMatcher(excluded []string) (imageMatcher, error)` in the watcher package to
build the exclusion matcher from `ExcludedImages` (DESIGN §4.3 — lift or rebuild the `internal/docker`
stringMatcher; a glob/regex-backed impl here is fine). Same rationale: keep the concrete impl next to
the interface, no export churn.

## Test list

`config_test.go`:
1. **Load default** → `testdata/config.yaml` bare `docker_attributes:` → defaults (grace 120s, default
   metadata set, unix endpoint).
2. **Load full** → a named config with endpoint, timeout, excluded_images, extract.labels/env_vars,
   association → asserts exact struct.
3. **Validate: empty endpoint** → error.
4. **Validate: unsupported association** (`from: connection`) → "not supported in v1" error.
5. **Validate: both key and key_regex** → error; **bad key_regex** → error.
6. **Resolution:** a Config → `watcher.NewExtractConfig` produces a FieldFilter that maps configured
   label/env keys and rejects others (unit-test the resolver directly).

`factory_test.go`:
7. **createDefaultConfig** type-asserts to `*Config` with expected defaults.
8. **Create each processor** (traces/metrics/logs/profiles) succeeds with default config (no daemon
   connection at create time — Start is where connection would happen; keep create daemon-free).

## Notes
- Add `internal/docker` to require (fork has it with a replace directive).
- Keep factory create daemon-free: the no-op processor from Step 0 stays until Step 5 wires the watcher.
  Step 1 only enriches the Config + resolution; process* methods remain passthrough.
- `mapstructure:",squash"` on the embedded docker.Config so its fields are top-level in YAML.

## Definition of done
1. Test list above all pass under `go test ./... -race`; `make lint` and `go mod tidy` clean.
2. **Codex review:** ask Codex to review this stream's diff. Address ALL P0 findings. If you disagree
   with a P0 (think it's wrong or not blocking), PUSH BACK to the user — do not silently comply or
   silently ignore. Non-P0: judgment. Not done until P0s are fixed or the user overrules them.
