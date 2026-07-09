// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package dockerattributesprocessor

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
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"
)

// fakeDockerClient is a minimal dockerAPI double used in processor tests.
// It satisfies the watcher's unexported dockerAPI interface.
type fakeDockerClient struct {
	mu       sync.Mutex
	inspects map[string]container.InspectResponse
	inspErr  map[string]error
}

func newFakeDockerClient() *fakeDockerClient {
	return &fakeDockerClient{
		inspects: map[string]container.InspectResponse{},
		inspErr:  map[string]error{},
	}
}

// addInspect registers a container inspect response keyed by id.
func (f *fakeDockerClient) addInspect(id, name, image, imageID string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspects[id] = container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    id,
			Name:  "/" + name,
			Image: imageID,
			State: &container.State{Running: running},
		},
		Config: &container.Config{Image: image},
	}
}

func (f *fakeDockerClient) ContainerInspect(_ context.Context, id string) (container.InspectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.inspErr[id]; err != nil {
		return container.InspectResponse{}, err
	}
	insp, ok := f.inspects[id]
	if !ok {
		return container.InspectResponse{}, errors.New("no such container")
	}
	return insp, nil
}

func (f *fakeDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []container.Summary
	for id := range f.inspects {
		out = append(out, container.Summary{ID: id})
	}
	return out, nil
}

func (*fakeDockerClient) Events(_ context.Context, _ events.ListOptions) (<-chan events.Message, <-chan error) {
	return make(chan events.Message), make(chan error)
}

// buildManager constructs a watcher.Manager backed by the fake Docker client.
func buildManager(t *testing.T, sdk *fakeDockerClient) *watcher.Manager {
	t.Helper()
	mgr, err := watcher.NewManager(sdk, watcher.Config{
		Grace: 2 * time.Minute,
	}, zaptest.NewLogger(t))
	require.NoError(t, err)
	return mgr
}

// startedProcessor returns a processor whose manager has been started (InitialSync
// called) with the given fake Docker client.
func startedProcessor(t *testing.T, sdk *fakeDockerClient) *dockerAttributesProcessor {
	t.Helper()
	mgr := buildManager(t, sdk)
	require.NoError(t, mgr.Start(t.Context()))
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })

	cfg := createDefaultConfig().(*Config)
	p := newProcessor(component.TelemetrySettings{Logger: zaptest.NewLogger(t)}, cfg)
	p.manager = mgr
	return p
}

// containerID is the primary test container ID.
const containerID = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

// containerID2 is a secondary container ID used in multi-container tests.
const containerID2 = "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3"

func TestNormalizeID(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantID string
		wantOK bool
	}{
		{
			name:   "plain 64-char hex",
			input:  containerID,
			wantID: containerID,
			wantOK: true,
		},
		{
			name:   "docker:// prefix stripped",
			input:  "docker://" + containerID,
			wantID: containerID,
			wantOK: true,
		},
		{
			name:   "short 12-char id no-op",
			input:  "a1b2c3d4e5f6",
			wantID: "",
			wantOK: false,
		},
		{
			name:   "empty no-op",
			input:  "",
			wantID: "",
			wantOK: false,
		},
		{
			name:   "non-hex no-op",
			input:  "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			wantID: "",
			wantOK: false,
		},
		{
			name:   "63 chars no-op",
			input:  containerID[:63],
			wantID: "",
			wantOK: false,
		},
		{
			name:   "65 chars no-op",
			input:  containerID + "a",
			wantID: "",
			wantOK: false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := normalizeID(tt.input)
			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantID, gotID)
		})
	}
}

func TestProcessTraces_Enrichment(t *testing.T) {
	sdk := newFakeDockerClient()
	sdk.addInspect(containerID, "web", "nginx:1.2", "sha256:deadbeef", true)

	p := startedProcessor(t, sdk)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("container.id", containerID)

	out, err := p.processTraces(t.Context(), td)
	require.NoError(t, err)

	attrs := out.ResourceSpans().At(0).Resource().Attributes()
	assertEnriched(t, attrs, "web", "nginx:1.2", "sha256:deadbeef")
}

func TestProcessMetrics_Enrichment(t *testing.T) {
	sdk := newFakeDockerClient()
	sdk.addInspect(containerID, "metrics-svc", "prom:1", "sha256:cafe", true)

	p := startedProcessor(t, sdk)

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("container.id", containerID)

	out, err := p.processMetrics(t.Context(), md)
	require.NoError(t, err)

	attrs := out.ResourceMetrics().At(0).Resource().Attributes()
	assertEnriched(t, attrs, "metrics-svc", "prom:1", "sha256:cafe")
}

func TestProcessLogs_Enrichment(t *testing.T) {
	sdk := newFakeDockerClient()
	sdk.addInspect(containerID, "log-svc", "fluentd:1", "sha256:babe", true)

	p := startedProcessor(t, sdk)

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("container.id", containerID)

	out, err := p.processLogs(t.Context(), ld)
	require.NoError(t, err)

	attrs := out.ResourceLogs().At(0).Resource().Attributes()
	assertEnriched(t, attrs, "log-svc", "fluentd:1", "sha256:babe")
}

func TestProcessProfiles_Enrichment(t *testing.T) {
	sdk := newFakeDockerClient()
	sdk.addInspect(containerID, "prof-svc", "pyroscope:1", "sha256:f00d", true)

	p := startedProcessor(t, sdk)

	pd := pprofile.NewProfiles()
	rp := pd.ResourceProfiles().AppendEmpty()
	rp.Resource().Attributes().PutStr("container.id", containerID)

	out, err := p.processProfiles(t.Context(), pd)
	require.NoError(t, err)

	attrs := out.ResourceProfiles().At(0).Resource().Attributes()
	assertEnriched(t, attrs, "prof-svc", "pyroscope:1", "sha256:f00d")
}

func TestProcessTraces_DockerPrefixStripped(t *testing.T) {
	sdk := newFakeDockerClient()
	sdk.addInspect(containerID, "prefixed-svc", "nginx:1", "sha256:1234", true)

	p := startedProcessor(t, sdk)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	// The raw resource attribute carries the docker:// scheme prefix.
	rawID := "docker://" + containerID
	rs.Resource().Attributes().PutStr("container.id", rawID)

	out, err := p.processTraces(t.Context(), td)
	require.NoError(t, err)

	attrs := out.ResourceSpans().At(0).Resource().Attributes()

	// container.id keeps its original "docker://..." value because writeTo does not
	// overwrite a pre-existing non-empty attribute; enrichment still succeeded by
	// stripping the prefix for the cache lookup.
	v, ok := attrs.Get("container.id")
	require.True(t, ok)
	assert.Equal(t, rawID, v.Str(), "container.id must retain its original docker:// value")

	// The other enriched attrs must be present (they were absent before the call).
	name, ok := attrs.Get("container.name")
	require.True(t, ok)
	assert.Equal(t, "prefixed-svc", name.Str())

	img, ok := attrs.Get("container.image.name")
	require.True(t, ok)
	assert.Equal(t, "nginx:1", img.Str())

	runtime, ok := attrs.Get("container.runtime.name")
	require.True(t, ok)
	assert.Equal(t, "docker", runtime.Str())
}

func TestProcessTraces_ShortIDNoOp(t *testing.T) {
	sdk := newFakeDockerClient()
	// The short id would match containerID[:12] if we did a prefix lookup — but we don't.
	sdk.addInspect(containerID, "web", "nginx:1", "sha256:1", false /* stopped */)

	p := startedProcessor(t, sdk)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("container.id", containerID[:12]) // 12-char short ID

	out, err := p.processTraces(t.Context(), td)
	require.NoError(t, err)

	attrs := out.ResourceSpans().At(0).Resource().Attributes()
	// Only the original container.id is set; enrichment attributes must be absent.
	assert.Equal(t, 1, attrs.Len(), "short id must produce no enrichment")
	v, ok := attrs.Get("container.id")
	require.True(t, ok)
	assert.Equal(t, containerID[:12], v.Str())
}

func TestProcessTraces_NoOverwriteExistingAttrs(t *testing.T) {
	sdk := newFakeDockerClient()
	sdk.addInspect(containerID, "new-name", "new-image:1", "sha256:new", true)

	p := startedProcessor(t, sdk)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	attrs := rs.Resource().Attributes()
	attrs.PutStr("container.id", containerID)
	attrs.PutStr("container.name", "existing-name") // must not be overwritten

	out, err := p.processTraces(t.Context(), td)
	require.NoError(t, err)

	outAttrs := out.ResourceSpans().At(0).Resource().Attributes()
	name, ok := outAttrs.Get("container.name")
	require.True(t, ok)
	assert.Equal(t, "existing-name", name.Str(), "pre-existing non-empty attr must not be overwritten")

	// Other attrs from the container are still enriched.
	img, ok := outAttrs.Get("container.image.name")
	require.True(t, ok)
	assert.Equal(t, "new-image:1", img.Str())
}

// TestProcessTraces_MultipleResources verifies that multiple resource spans in a
// single batch are each enriched with the correct container metadata.
func TestProcessTraces_MultipleResources(t *testing.T) {
	sdk := newFakeDockerClient()
	sdk.addInspect(containerID, "svc-a", "nginx:1", "sha256:aaa", true)
	sdk.addInspect(containerID2, "svc-b", "redis:7", "sha256:bbb", true)

	p := startedProcessor(t, sdk)

	td := ptrace.NewTraces()
	rs0 := td.ResourceSpans().AppendEmpty()
	rs0.Resource().Attributes().PutStr("container.id", containerID)
	rs1 := td.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("container.id", containerID2)

	out, err := p.processTraces(t.Context(), td)
	require.NoError(t, err)

	assertEnriched(t, out.ResourceSpans().At(0).Resource().Attributes(), "svc-a", "nginx:1", "sha256:aaa")
	assertEnriched(t, out.ResourceSpans().At(1).Resource().Attributes(), "svc-b", "redis:7", "sha256:bbb")
}

func TestProcessTraces_NoContainerID_NoEnrichment(t *testing.T) {
	sdk := newFakeDockerClient()
	p := startedProcessor(t, sdk)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "myservice")

	out, err := p.processTraces(t.Context(), td)
	require.NoError(t, err)
	attrs := out.ResourceSpans().At(0).Resource().Attributes()
	assert.Equal(t, 1, attrs.Len(), "resource without container.id must not be enriched")
}

// assertEnriched checks the core container.* attributes are present and correct.
// It does NOT check container.id since callers may use raw (docker://-prefixed) IDs.
func assertEnriched(t *testing.T, attrs pcommon.Map, wantName, wantImage, wantImageID string) {
	t.Helper()
	check := func(key, want string) {
		t.Helper()
		v, ok := attrs.Get(key)
		require.True(t, ok, "expected attr %q to be present", key)
		assert.Equal(t, want, v.Str(), "attr %q", key)
	}
	check("container.name", wantName)
	check("container.image.name", wantImage)
	check("container.image.id", wantImageID)
	check("container.runtime.name", "docker")
}
