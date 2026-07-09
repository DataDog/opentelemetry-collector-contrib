// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package dockerattributesprocessor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/confmap/xconfmap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/docker"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/metadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"
)

func loadConf(t *testing.T, name string) component.Config {
	t.Helper()
	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)

	sub, err := cm.Sub(name)
	require.NoError(t, err)

	cfg := createDefaultConfig()
	require.NoError(t, sub.Unmarshal(cfg))
	return cfg
}

func TestLoadConfig_Default(t *testing.T) {
	cfg := loadConf(t, component.NewID(metadata.Type).String())

	expected := createDefaultConfig().(*Config)
	assert.Equal(t, expected, cfg)

	c := cfg.(*Config)
	assert.Equal(t, 120*time.Second, c.DeleteGracePeriod)
	assert.Equal(t, docker.NewDefaultConfig().Endpoint, c.Endpoint)
	assert.NoError(t, xconfmap.Validate(cfg))
}

func TestLoadConfig_Full(t *testing.T) {
	cfg := loadConf(t, component.NewIDWithName(metadata.Type, "full").String())

	expected := &Config{
		Config: docker.Config{
			Endpoint:         "unix:///var/run/docker.sock",
			Timeout:          20 * time.Second,
			DockerAPIVersion: "1.44",
			ExcludedImages:   []string{"excluded-image", "*/pause"},
		},
		DeleteGracePeriod: 300 * time.Second,
		Extract: ExtractConfig{
			Labels: []FieldExtractConfig{
				{Key: "com.example.team", TagName: "team"},
				{KeyRegex: `com\.example\.(.*)`, TagName: "$1"},
			},
			EnvVars: []FieldExtractConfig{
				{Key: "MY_ENV"},
			},
		},
		Association: []AssociationConfig{
			{From: "resource_attribute", Name: "container.id"},
		},
	}
	assert.Equal(t, expected, cfg)
	assert.NoError(t, xconfmap.Validate(cfg))
}

func TestValidate_EmptyEndpoint(t *testing.T) {
	cfg := loadConf(t, component.NewIDWithName(metadata.Type, "invalid_endpoint").String())
	err := xconfmap.Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint")
}

func TestValidate_UnsupportedAssociation(t *testing.T) {
	cfg := loadConf(t, component.NewIDWithName(metadata.Type, "unsupported_association").String())
	err := xconfmap.Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported in v1")
}

func TestValidate_FieldExtract(t *testing.T) {
	t.Run("both key and key_regex", func(t *testing.T) {
		cfg := loadConf(t, component.NewIDWithName(metadata.Type, "both_key_and_regex").String())
		err := xconfmap.Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only one of key or key_regex")
	})

	t.Run("bad key_regex", func(t *testing.T) {
		cfg := loadConf(t, component.NewIDWithName(metadata.Type, "bad_key_regex").String())
		err := xconfmap.Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid key_regex")
	})

	t.Run("neither key nor key_regex", func(t *testing.T) {
		cfg := createDefaultConfig().(*Config)
		cfg.Extract.Labels = []FieldExtractConfig{{}}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "one of key or key_regex must be set")
	})

	t.Run("negative grace period", func(t *testing.T) {
		cfg := createDefaultConfig().(*Config)
		cfg.DeleteGracePeriod = -time.Second
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete_grace_period")
	})
}

func TestResolveExtract(t *testing.T) {
	ec := watcher.ResolveExtract(
		[]watcher.FieldExtractConfig{
			{Key: "com.example.team", TagName: "team"},
			{KeyRegex: `com\.example\.(.*)`, TagName: "$1"},
			{Key: "plain"}, // default tag name
		},
		[]watcher.FieldExtractConfig{
			{Key: "MY_ENV"},
		},
	)

	// exact match, explicit tag.
	name, ok := ec.LabelsMap("com.example.team")
	assert.True(t, ok)
	assert.Equal(t, "team", name)

	// regex match, back reference.
	name, ok = ec.LabelsMap("com.example.version")
	assert.True(t, ok)
	assert.Equal(t, "version", name)

	// exact match, default tag name.
	name, ok = ec.LabelsMap("plain")
	assert.True(t, ok)
	assert.Equal(t, "container.label.plain", name)

	// unselected label.
	_, ok = ec.LabelsMap("other")
	assert.False(t, ok)

	// env var: selected and default tag name.
	name, ok = ec.EnvVarsMap("MY_ENV")
	assert.True(t, ok)
	assert.Equal(t, "container.env.MY_ENV", name)

	// unselected env var.
	_, ok = ec.EnvVarsMap("OTHER_ENV")
	assert.False(t, ok)
}

// A regex rule must match the WHOLE key: alternation must bind inside the
// anchors (^(?:foo|bar)$), not produce ^foo|bar$ which would match foo-secret.
func TestResolveExtract_RegexAlternationFullMatch(t *testing.T) {
	ec := watcher.ResolveExtract(
		[]watcher.FieldExtractConfig{
			{KeyRegex: `foo|bar`, TagName: "matched"},
		},
		nil,
	)

	// Exact whole-key matches.
	_, ok := ec.LabelsMap("foo")
	assert.True(t, ok, "whole-key 'foo' must match")
	_, ok = ec.LabelsMap("bar")
	assert.True(t, ok, "whole-key 'bar' must match")

	// Partial matches must NOT match (the bug: ^foo|bar$ matches these).
	_, ok = ec.LabelsMap("foo-secret")
	assert.False(t, ok, "'foo-secret' must not match foo|bar (alternation must be anchored)")
	_, ok = ec.LabelsMap("prefix-bar")
	assert.False(t, ok, "'prefix-bar' must not match foo|bar")
}

// A regex rule with no tag_name must derive the attribute name with the default
// prefix, consistent with exact-key rules — not return the raw key.
func TestResolveExtract_RegexDefaultPrefix(t *testing.T) {
	ec := watcher.ResolveExtract(
		[]watcher.FieldExtractConfig{
			{KeyRegex: `com\.example\..*`}, // no TagName
		},
		[]watcher.FieldExtractConfig{
			{KeyRegex: `MY_.*`}, // no TagName
		},
	)

	name, ok := ec.LabelsMap("com.example.team")
	assert.True(t, ok)
	assert.Equal(t, "container.label.com.example.team", name,
		"regex rule without tag_name must apply the default label prefix")

	name, ok = ec.EnvVarsMap("MY_ENV")
	assert.True(t, ok)
	assert.Equal(t, "container.env.MY_ENV", name,
		"regex rule without tag_name must apply the default env prefix")
}
