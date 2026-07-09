// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// fakeMatcher is a trivial imageMatcher: it reports the configured verdict and
// records the image it was asked about.
type fakeMatcher struct {
	exclude bool
	seen    string
}

func (m *fakeMatcher) Matches(image string) bool {
	m.seen = image
	return m.exclude
}

// mapFilter is a map-backed fieldFilter: a source key is selected iff present,
// and maps to its configured attribute name.
type mapFilter map[string]string

func (f mapFilter) Map(key string) (string, bool) {
	attr, ok := f[key]
	return attr, ok
}

// inspectFixture builds a synthetic *container.InspectResponse.
func inspectFixture(id, name, image, imageID string, cmd, env []string, labels map[string]string) *container.InspectResponse {
	return &container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    id,
			Name:  name,
			Image: imageID,
		},
		Config: &container.Config{
			Image:  image,
			Cmd:    cmd,
			Env:    env,
			Labels: labels,
		},
	}
}

func TestBuild_FullRunningContainer(t *testing.T) {
	insp := inspectFixture(
		"abc123", "/web", "nginx:1.2", "sha256:deadbeef",
		[]string{"-g", "daemon off;"}, nil, nil,
	)
	insp.Config.Entrypoint = []string{"nginx"}

	e, ok := build(insp, extractConfig{}, &fakeMatcher{})
	require.True(t, ok)
	require.NotNil(t, e)

	assert.Equal(t, "abc123", e.id)
	assert.Equal(t, "web", e.name)
	assert.Equal(t, "nginx:1.2", e.imageName)
	assert.Equal(t, "sha256:deadbeef", e.imageID)
	assert.Equal(t, "docker", e.runtimeName)
	assert.Equal(t, []string{"1.2"}, e.imageTags)
	assert.Equal(t, int64(0), e.stamp) // build never stamps
	// Command is entrypoint + cmd (semconv): command=executable, args=full list,
	// line=joined.
	assert.Equal(t, "nginx", e.command)
	assert.Equal(t, []string{"nginx", "-g", "daemon off;"}, e.commandArgs)
	assert.Equal(t, "nginx -g daemon off;", e.commandLine)

	attrs := pcommon.NewMap()
	e.writeTo(attrs)
	got := attrs.AsRaw()
	assert.Equal(t, "abc123", got[attrContainerID])
	assert.Equal(t, "web", got[attrContainerName])
	assert.Equal(t, "nginx:1.2", got[attrContainerImageName])
	assert.Equal(t, "sha256:deadbeef", got[attrContainerImageID])
	assert.Equal(t, "nginx", got[attrContainerCommand])
	assert.Equal(t, "nginx -g daemon off;", got[attrContainerCommandLine])
	assert.Equal(t, []any{"nginx", "-g", "daemon off;"}, got[attrContainerCommandArgs])
	assert.Equal(t, "docker", got[attrContainerRuntimeName])
}

func TestBuild_BareDigestImageSuppressesName(t *testing.T) {
	// Locally built / devcontainer images: Config.Image is the bare digest, which
	// duplicates container.image.id and carries no name — so image.name is omitted.
	insp := inspectFixture("id", "/c",
		"sha256:8de24d360e1b083c2e32ce4be1f4eb056ed3fcdd05dc08499b5137f732cf66ae",
		"sha256:8de24d360e1b083c2e32ce4be1f4eb056ed3fcdd05dc08499b5137f732cf66ae",
		nil, nil, nil)
	e, ok := build(insp, extractConfig{}, &fakeMatcher{})
	require.True(t, ok)
	assert.Empty(t, e.imageName, "bare digest must not be emitted as image.name")

	attrs := pcommon.NewMap()
	e.writeTo(attrs)
	_, present := attrs.Get(attrContainerImageName)
	assert.False(t, present, "container.image.name must be omitted for a bare-digest image")
}

func TestBuild_NameSlashTrim(t *testing.T) {
	insp := inspectFixture("id", "/foo", "img", "", nil, nil, nil)
	e, ok := build(insp, extractConfig{}, &fakeMatcher{})
	require.True(t, ok)
	assert.Equal(t, "foo", e.name)
}

func TestBuild_DigestOnlyImage_NoTag(t *testing.T) {
	insp := inspectFixture("id", "/c",
		"repo@sha256:1111111111111111111111111111111111111111111111111111111111111111",
		"", nil, nil, nil)
	e, ok := build(insp, extractConfig{}, &fakeMatcher{})
	require.True(t, ok)
	assert.Empty(t, e.imageTags, "digest-only ref must not fabricate a tag")

	attrs := pcommon.NewMap()
	e.writeTo(attrs)
	_, present := attrs.Get(attrContainerImageTags)
	assert.False(t, present, "container.image.tags must be omitted for digest-only ref")
}

func TestBuild_TaggedImage(t *testing.T) {
	insp := inspectFixture("id", "/c", "repo:1.2", "", nil, nil, nil)
	e, ok := build(insp, extractConfig{}, &fakeMatcher{})
	require.True(t, ok)
	assert.Equal(t, []string{"1.2"}, e.imageTags)

	attrs := pcommon.NewMap()
	e.writeTo(attrs)
	v, present := attrs.Get(attrContainerImageTags)
	require.True(t, present)
	require.Equal(t, pcommon.ValueTypeSlice, v.Type())
	sl := v.Slice()
	require.Equal(t, 1, sl.Len())
	assert.Equal(t, "1.2", sl.At(0).Str())
}

func TestBuild_TaggedAndDigestedImage(t *testing.T) {
	// Explicit tag alongside a digest: the tag is real and must be kept.
	insp := inspectFixture("id", "/c",
		"repo:1.2@sha256:1111111111111111111111111111111111111111111111111111111111111111",
		"", nil, nil, nil)
	e, ok := build(insp, extractConfig{}, &fakeMatcher{})
	require.True(t, ok)
	assert.Equal(t, []string{"1.2"}, e.imageTags)
}

func TestBuild_UntaggedImage_NoFabricatedLatest(t *testing.T) {
	// Untagged refs with no digest: ParseImageName defaults Tag to "latest", but
	// we must NOT fabricate it. Covers bare names and the registry-port form
	// (host:5000/repo), where the ':' is a port, not a tag.
	tests := []struct {
		name  string
		image string
	}{
		{"bare", "nginx"},
		{"repo path", "library/nginx"},
		{"registry port", "host:5000/repo"},
		{"registry port with path", "host:5000/team/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insp := inspectFixture("id", "/c", tt.image, "", nil, nil, nil)
			e, ok := build(insp, extractConfig{}, &fakeMatcher{})
			require.True(t, ok)
			assert.Empty(t, e.imageTags, "untagged ref %q must not fabricate a tag", tt.image)

			attrs := pcommon.NewMap()
			e.writeTo(attrs)
			_, present := attrs.Get(attrContainerImageTags)
			assert.False(t, present, "container.image.tags must be omitted for untagged ref %q", tt.image)
		})
	}
}

func TestBuild_RegistryPortWithTag(t *testing.T) {
	// A real tag alongside a registry port must still be extracted.
	insp := inspectFixture("id", "/c", "host:5000/repo:1.2", "", nil, nil, nil)
	e, ok := build(insp, extractConfig{}, &fakeMatcher{})
	require.True(t, ok)
	assert.Equal(t, []string{"1.2"}, e.imageTags)
}

func TestBuild_ExcludedImage(t *testing.T) {
	insp := inspectFixture("id", "/c", "excluded:latest", "", nil, nil, nil)
	m := &fakeMatcher{exclude: true}
	e, ok := build(insp, extractConfig{}, m)
	assert.False(t, ok)
	assert.Nil(t, e)
	assert.Equal(t, "excluded:latest", m.seen, "matcher must be asked about the raw Config.Image")
}

func TestBuild_EnvWithEqualsInValue(t *testing.T) {
	insp := inspectFixture("id", "/c", "img", "",
		nil, []string{"KEY=a=b=c"}, nil)
	cfg := extractConfig{envVars: mapFilter{"KEY": "attr.key"}}
	e, ok := build(insp, cfg, &fakeMatcher{})
	require.True(t, ok)
	assert.Equal(t, "a=b=c", e.env["attr.key"], "SplitN must preserve '=' in values")
}

func TestBuild_EnvOptIn(t *testing.T) {
	insp := inspectFixture("id", "/c", "img", "",
		nil, []string{"WANTED=yes", "SECRET_TOKEN=hunter2", "EMPTY="}, nil)
	cfg := extractConfig{envVars: mapFilter{"WANTED": "attr.wanted", "EMPTY": "attr.empty"}}
	e, ok := build(insp, cfg, &fakeMatcher{})
	require.True(t, ok)

	assert.Equal(t, "yes", e.env["attr.wanted"])
	// Empty-valued vars are kept when selected.
	val, present := e.env["attr.empty"]
	assert.True(t, present, "empty-valued selected var must be kept")
	assert.Empty(t, val)
	// Unconfigured secret-looking key never leaks under any name.
	assert.NotContains(t, e.env, "SECRET_TOKEN")
	for _, v := range e.env {
		assert.NotEqual(t, "hunter2", v, "unconfigured env value must not leak")
	}
}

func TestBuild_LabelFilter(t *testing.T) {
	insp := inspectFixture("id", "/c", "img", "", nil, nil, map[string]string{
		"com.example.team": "payments",
		"unwanted":         "drop-me",
	})
	cfg := extractConfig{labels: mapFilter{"com.example.team": "container.label.team"}}
	e, ok := build(insp, cfg, &fakeMatcher{})
	require.True(t, ok)

	assert.Equal(t, map[string]string{"container.label.team": "payments"}, e.labels)
	assert.NotContains(t, e.labels, "unwanted")
	assert.NotContains(t, e.labels, "drop-me")
}

func TestBuild_NoLabelsSelected_NilMap(t *testing.T) {
	insp := inspectFixture("id", "/c", "img", "", nil, nil, map[string]string{"x": "y"})
	e, ok := build(insp, extractConfig{}, &fakeMatcher{})
	require.True(t, ok)
	assert.Nil(t, e.labels, "no selected labels must leave labels nil, not empty map")
}

func TestNotActive(t *testing.T) {
	tests := []struct {
		name  string
		state *container.State
		want  bool
	}{
		{"paused", &container.State{Running: true, Paused: true}, true},
		{"stopped", &container.State{Running: false, Paused: false}, true},
		{"running", &container.State{Running: true, Paused: false}, false},
		{"nil", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, notActive(tt.state))
		})
	}
}

func TestWriteTo_NoOverwriteNonEmpty(t *testing.T) {
	e := &entry{id: "new-id", name: "new-name", runtimeName: "docker"}

	attrs := pcommon.NewMap()
	attrs.PutStr(attrContainerName, "existing-name") // non-empty: must be preserved
	attrs.PutStr(attrContainerID, "")                // empty: must be overwritten

	e.writeTo(attrs)

	name, _ := attrs.Get(attrContainerName)
	assert.Equal(t, "existing-name", name.Str(), "non-empty value must not be overwritten")

	id, _ := attrs.Get(attrContainerID)
	assert.Equal(t, "new-id", id.Str(), "existing empty value must be overwritten")
}

func TestWriteTo_NoOverwriteNonEmptyTags(t *testing.T) {
	e := &entry{imageTags: []string{"2.0"}}

	attrs := pcommon.NewMap()
	existing := attrs.PutEmptySlice(attrContainerImageTags)
	existing.AppendEmpty().SetStr("1.0")

	e.writeTo(attrs)

	v, _ := attrs.Get(attrContainerImageTags)
	require.Equal(t, 1, v.Slice().Len())
	assert.Equal(t, "1.0", v.Slice().At(0).Str(), "existing non-empty tags slice must be preserved")
}

func TestWriteTo_EmptyTagsOmitted(t *testing.T) {
	e := &entry{id: "id", imageTags: nil}
	attrs := pcommon.NewMap()
	e.writeTo(attrs)
	_, present := attrs.Get(attrContainerImageTags)
	assert.False(t, present, "nil imageTags must produce no container.image.tags key")
}

func TestWriteTo_EmptyEnvValueEmitted(t *testing.T) {
	// A selected env/label with an empty value is meaningful and must be written
	// (unlike the fixed metadata, where empty means "not present"). This is the
	// writeTo-level counterpart to TestBuild_EnvOptIn's entry-level assertion.
	e := &entry{
		env:    map[string]string{"attr.empty": ""},
		labels: map[string]string{"container.label.blank": ""},
	}
	attrs := pcommon.NewMap()
	e.writeTo(attrs)

	v, present := attrs.Get("attr.empty")
	require.True(t, present, "selected empty-valued env var must be emitted")
	assert.Empty(t, v.Str())

	l, present := attrs.Get("container.label.blank")
	require.True(t, present, "selected empty-valued label must be emitted")
	assert.Empty(t, l.Str())
}

func TestWriteTo_EnvLabelNoOverwriteNonEmpty(t *testing.T) {
	// Even the allow-empty path must not clobber an existing non-empty value.
	e := &entry{env: map[string]string{"attr.k": ""}}
	attrs := pcommon.NewMap()
	attrs.PutStr("attr.k", "existing")
	e.writeTo(attrs)
	v, _ := attrs.Get("attr.k")
	assert.Equal(t, "existing", v.Str(), "empty env value must not overwrite an existing non-empty attr")
}
