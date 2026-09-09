// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package dockerattributesprocessor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/processor/processortest"
	"go.opentelemetry.io/collector/processor/xprocessor"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/docker"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/sharedcomponent"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/metadata"
)

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig()
	dc, ok := cfg.(*Config)
	require.True(t, ok, "expected *Config")

	assert.Equal(t, 120*time.Second, dc.DeleteGracePeriod)
	assert.Equal(t, *docker.NewDefaultConfig(), dc.Config)
	assert.Empty(t, dc.Association)
	assert.Empty(t, dc.Extract.Labels)
	assert.Empty(t, dc.Extract.EnvVars)
	assert.NoError(t, dc.Validate())
}

func TestFactorySharesProcessorsByID(t *testing.T) {
	f := &factory{processors: sharedcomponent.NewSharedComponents()}
	cfg := createDefaultConfig().(*Config)
	set := processortest.NewNopSettings(metadata.Type)

	firstShared, first := f.get(set, cfg)
	secondShared, second := f.get(set, cfg)
	assert.Same(t, firstShared, secondShared)
	assert.Same(t, first, second)

	set.ID = component.NewIDWithName(metadata.Type, "named")
	namedShared, named := f.get(set, cfg)
	assert.NotSame(t, firstShared, namedShared)
	assert.NotSame(t, first, named)
}

// TestCreateProcessors also asserts create does not connect to a daemon.
func TestCreateProcessors(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	set := processortest.NewNopSettings(factory.Type())
	ctx := t.Context()

	t.Run("traces", func(t *testing.T) {
		p, err := factory.CreateTraces(ctx, set, cfg, consumertest.NewNop())
		require.NoError(t, err)
		require.NotNil(t, p)
	})
	t.Run("metrics", func(t *testing.T) {
		p, err := factory.CreateMetrics(ctx, set, cfg, consumertest.NewNop())
		require.NoError(t, err)
		require.NotNil(t, p)
	})
	t.Run("logs", func(t *testing.T) {
		p, err := factory.CreateLogs(ctx, set, cfg, consumertest.NewNop())
		require.NoError(t, err)
		require.NotNil(t, p)
	})
	t.Run("profiles", func(t *testing.T) {
		xf, ok := factory.(xprocessor.Factory)
		require.True(t, ok, "factory should implement xprocessor.Factory")
		p, err := xf.CreateProfiles(ctx, set, cfg, consumertest.NewNop().(xconsumer.Profiles))
		require.NoError(t, err)
		require.NotNil(t, p)
	})
}
