// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/clientutil"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/hostmetadata"
)

func Test_NotifyConfig(t *testing.T) {
	// Create a simple confmap.Conf
	configData := map[string]any{
		"service": map[string]any{
			"pipelines": map[string]any{
				"traces": map[string]any{
					"receivers": []any{"otlp"},
					"exporters": []any{"debug"},
				},
			},
		},
	}
	conf := confmap.NewFromStringMap(configData)

	// Create a background context
	ctx := context.Background()

	// Create a logger for testing
	logger := zaptest.NewLogger(t)

	set := extension.Settings{}
	// Create telemetry settings with the test logger
	telemetry := componenttest.NewNopTelemetrySettings()
	telemetry.Logger = logger
	set.TelemetrySettings = telemetry
	set.BuildInfo = component.BuildInfo{
		Command:     "otelcol",
		Description: "OpenTelemetry Collector",
		Version:     "1.0.0",
	}
	set.ID = component.MustNewID(metadata.Type.String())

	faExt, err := newExtension(ctx, &Config{}, set, clientutil.ValidateAPIKey, hostmetadata.GetSourceProvider, newForwarder)
	assert.NoError(t, err)
	err = faExt.NotifyConfig(ctx, conf)
	assert.NoError(t, err)

	// Verify that the configuration is correctly set
	assert.Equal(t, conf, faExt.collectorConfig)
}

func TestPrepareAgentMetadataPayload(t *testing.T) {
	site := "datadoghq.com"
	tool := "otelcol"
	toolVersion := "1.0.0"
	installerVersion := "1.0.0"
	hostname := "test-hostname"

	expectedPayload := AgentMetadata{
		AgentVersion:                      "7.64.0-collector",
		AgentStartupTimeMs:                1234567890123,
		AgentFlavor:                       "agent",
		ConfigSite:                        site,
		ConfigEKSFargate:                  false,
		InstallMethodTool:                 tool,
		InstallMethodToolVersion:          toolVersion,
		InstallMethodInstallerVersion:     installerVersion,
		FeatureRemoteConfigurationEnabled: true,
		FeatureOTLPEnabled:                true,
		Hostname:                          hostname,
	}

	actualPayload := prepareAgentMetadataPayload(site, tool, toolVersion, installerVersion, hostname)

	assert.Equal(t, expectedPayload, actualPayload)
}

func TestPrepareOtelMetadataPayload(t *testing.T) {
	version := "1.0.0"
	extensionVersion := "1.0.0"
	command := "otelcol"
	fullConfig := "{\"service\":{\"pipelines\":{\"traces\":{\"receivers\":[\"otlp\"],\"exporters\":[\"debug\"]}}}}"

	expectedPayload := OtelMetadata{
		Enabled:                          true,
		Version:                          version,
		ExtensionVersion:                 extensionVersion,
		Command:                          command,
		Description:                      "OSS Collector with Datadog Fleet Automation Extension",
		ProvidedConfiguration:            "",
		EnvironmentVariableConfiguration: "",
		FullConfiguration:                fullConfig,
	}

	actualPayload := prepareOtelMetadataPayload(version, extensionVersion, command, fullConfig)

	assert.Equal(t, expectedPayload, actualPayload)
}

func TestNewExtension(t *testing.T) {
	tests := []struct {
		name                 string
		config               *Config
		apiKeyValidator      *mockAPIKeyValidator
		sourceProviderGetter *mockSourceProviderGetter
		forwarderGetter      ForwarderGetter
		expectedError        string
	}{
		{
			name: "Valid configuration",
			config: &Config{
				API: APIConfig{
					Site: "datadoghq.com",
					Key:  "valid-api-key",
				},
				Hostname: "test-hostname",
			},
			apiKeyValidator: &mockAPIKeyValidator{
				err: nil,
			},
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
				err:      nil,
			},
			expectedError: "",
		},
		{
			name: "Invalid API key",
			config: &Config{
				API: APIConfig{
					Site:             "datadoghq.com",
					Key:              "invalid-api-key",
					FailOnInvalidKey: true,
				},
				Hostname: "test-hostname",
			},
			apiKeyValidator: &mockAPIKeyValidator{
				err: fmt.Errorf("invalid API key"),
			},
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
				err:      nil,
			},
			expectedError: "invalid API key",
		},
		{
			name: "Hostname detection failed",
			config: &Config{
				API: APIConfig{
					Site: "datadoghq.com",
					Key:  "valid-api-key",
				},
				Hostname: "",
			},
			apiKeyValidator: &mockAPIKeyValidator{
				err: nil,
			},
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: nil,
				err:      fmt.Errorf("hostname detection failed"),
			},
			expectedError: "hostname detection failed",
		},
		{
			name: "Failed to create forwarder",
			config: &Config{
				API: APIConfig{
					Site: "datadoghq.com",
					Key:  "valid-api-key",
				},
				Hostname: "test-hostname",
			},
			apiKeyValidator: &mockAPIKeyValidator{
				err: nil,
			},
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
				err:      nil,
			},
			forwarderGetter: newNilForwarder,
			expectedError:   "failed to create forwarder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctx := context.Background()
			logger := zaptest.NewLogger(t)
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger

			settings := extension.Settings{
				TelemetrySettings: telemetry,
				BuildInfo: component.BuildInfo{
					Command:     "otelcol",
					Description: "OpenTelemetry Collector",
					Version:     "1.0.0",
				},
				ID: component.MustNewID(metadata.Type.String()),
			}

			if tt.forwarderGetter == nil {
				tt.forwarderGetter = newForwarder
			}

			ext, err := newExtension(
				ctx,
				tt.config,
				settings,
				tt.apiKeyValidator.ValidateAPIKey,
				tt.sourceProviderGetter.GetSourceProvider,
				tt.forwarderGetter,
			)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ext)
			}
		})
	}
}

func TestGetHostname(t *testing.T) {
	tests := []struct {
		name                 string
		providedHostname     string
		sourceProviderGetter *mockSourceProviderGetter
		expectedHostname     string
		expectedSource       string
		expectedError        string
	}{
		{
			name:             "Provided hostname is set",
			providedHostname: "test-hostname",
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
				err:      nil,
			},
			expectedHostname: "test-hostname",
			expectedSource:   "config",
			expectedError:    "",
		},
		{
			name:             "Provided hostname is empty, source provider infers hostname",
			providedHostname: "",
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "inferred-hostname"},
				err:      nil,
			},
			expectedHostname: "inferred-hostname",
			expectedSource:   "inferred",
			expectedError:    "",
		},
		{
			name:             "Provided hostname is empty, source provider fails to infer hostname",
			providedHostname: "",
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "", err: fmt.Errorf("hostname detection failed")},
				err:      nil,
			},
			expectedHostname: "",
			expectedSource:   "unset",
			expectedError:    "hostname detection failed, please set hostname manually in config: hostname detection failed",
		},
		{
			name:             "Source provider getter fails",
			providedHostname: "",
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: nil,
				err:      fmt.Errorf("source provider getter failed"),
			},
			expectedHostname: "",
			expectedSource:   "unset",
			expectedError:    "hostname detection failed to start, hostname must be set manually in config: source provider getter failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t)
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger

			hostname, hostnameSource, _, err := getHostname(ctx, telemetry, tt.providedHostname, tt.sourceProviderGetter.GetSourceProvider)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedHostname, hostname)
			assert.Equal(t, tt.expectedSource, hostnameSource)
		})
	}
}

func TestFleetAutomationExtension_Start(t *testing.T) {
	tests := []struct {
		name          string
		forwarder     defaultForwarderInterface
		expectedError string
	}{
		{
			name:          "Forwarder starts successfully",
			forwarder:     mockForwarder{},
			expectedError: "",
		},
		{
			name:          "Forwarder start error",
			forwarder:     mockForwarder{startError: fmt.Errorf("forwarder start error")},
			expectedError: "forwarder start error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t)
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger

			ext := &fleetAutomationExtension{
				telemetry: telemetry,
				forwarder: tt.forwarder,
				done:      make(chan bool),
			}

			// listen on ext.done to avoid panic
			go func() {
				<-ext.done
			}()

			err := ext.Start(ctx, nil)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
			if ext.httpServer != nil {
				ext.httpServer.Shutdown(ctx)
			}
			ext.done <- true
			defer close(ext.done)
		})
	}
}

func TestFleetAutomationExtension_Shutdown(t *testing.T) {
	tests := []struct {
		name          string
		forwarder     defaultForwarderInterface
		expectedError string
	}{
		{
			name:          "Forwarder stops successfully",
			forwarder:     mockForwarder{},
			expectedError: "",
		},
		{
			name:          "Forwarder stop error",
			forwarder:     mockForwarder{stopError: fmt.Errorf("forwarder stop error")},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t)
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger

			ext := &fleetAutomationExtension{
				telemetry: telemetry,
				forwarder: tt.forwarder,
				done:      make(chan bool),
			}

			// listen on ext.done to avoid panic
			go func() {
				<-ext.done
			}()

			err := ext.Shutdown(ctx)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func newNilForwarder(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
	return nil
}

type mockSourceProvider struct {
	hostname string
	err      error
}

func (m *mockSourceProvider) Source(ctx context.Context) (source.Source, error) {
	if m.err != nil {
		return source.Source{}, m.err
	}
	return source.Source{Identifier: m.hostname}, nil
}

type mockSourceProviderGetter struct {
	provider source.Provider
	err      error
}

func (m *mockSourceProviderGetter) GetSourceProvider(telemetry component.TelemetrySettings, providedHostname string, timeout time.Duration) (source.Provider, error) {
	return m.provider, m.err
}

type mockAPIKeyValidator struct {
	err error
}

func (m *mockAPIKeyValidator) ValidateAPIKey(ctx context.Context, apiKey string, logger *zap.Logger, apiClient *datadog.APIClient) error {
	return m.err
}

type mockForwarder struct {
	startError error
	stopError  error
	state      uint32
}

func (m mockForwarder) Start() error {
	if m.startError != nil {
		return m.startError
	}
	atomic.StoreUint32(&m.state, 1)
	return nil
}

func (m mockForwarder) Stop() {
	if m.stopError != nil {
		return
	}
	atomic.StoreUint32(&m.state, 0)
}

func (m mockForwarder) State() uint32 {
	return atomic.LoadUint32(&m.state)
}

func (m mockForwarder) SubmitV1Series(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

func (m mockForwarder) SubmitV1Intake(payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error {
	return nil
}

func (m mockForwarder) SubmitV1CheckRuns(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

func (m mockForwarder) SubmitSeries(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

func (m mockForwarder) SubmitSketchSeries(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

func (m mockForwarder) SubmitHostMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

func (m mockForwarder) SubmitAgentChecksMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

func (m mockForwarder) SubmitMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

func (m mockForwarder) SubmitProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitProcessDiscoveryChecks(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitProcessEventChecks(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitRTProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitRTContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitOrchestratorChecks(payload transaction.BytesPayloads, extra http.Header, payloadType int) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitOrchestratorManifests(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}
