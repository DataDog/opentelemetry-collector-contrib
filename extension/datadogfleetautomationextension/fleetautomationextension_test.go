// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

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
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/metadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog/clientutil"
)

func Test_NotifyConfig(t *testing.T) {
	tests := []struct {
		name           string
		configData     map[string]any
		expectedConfig map[string]any
		expectedError  string
		expectedLog    string
		forwarder      ForwarderGetter
		provider       SourceProviderGetter
		apikey         APIKeyValidator
	}{
		{
			name: "Forwarder fails to send metadata",
			configData: map[string]any{
				"service": map[string]any{},
			},
			expectedConfig: map[string]any{
				"service": map[string]any{},
			},
			expectedError: "failed to send datadog_agent payload",
			forwarder: func(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
				return mockForwarder{failSendMetadata: true, state: 1}
			},
		},
		{
			name: "Invalid configuration",
			configData: map[string]any{
				"invalid": "config",
			},
			expectedConfig: map[string]any{
				"invalid": "config",
			},
			expectedError: "",
			expectedLog:   "Failed to populate active components JSON",
			forwarder: func(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
				return mockForwarder{state: 1}
			},
		},
		{
			name: "Valid configuration",
			configData: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": []any{"otlp"},
							"exporters": []any{"debug"},
						},
					},
				},
			},
			expectedConfig: map[string]any{
				"service": map[string]any{
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers": []any{"otlp"},
							"exporters": []any{"debug"},
						},
					},
				},
			},
			expectedError: "",
			forwarder: func(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
				return mockForwarder{state: 1}
			},
		},
		{
			name: "Empty configuration",
			configData: map[string]any{
				"service": map[string]any{},
			},
			expectedConfig: map[string]any{
				"service": map[string]any{},
			},
			expectedError: "",
			forwarder: func(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
				return mockForwarder{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a confmap.Conf from the test config data
			conf := confmap.NewFromStringMap(tt.configData)

			// Create a background context
			ctx := context.Background()

			// Create a logger for testing
			core, logs := observer.New(zapcore.InfoLevel)
			logger := zap.New(core)

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

			if tt.forwarder == nil {
				tt.forwarder = newForwarder
			}
			if tt.provider == nil {
				mspg := mockSourceProviderGetter{
					provider: &mockSourceProvider{hostname: "inferred-hostname"},
				}
				tt.provider = mspg.GetSourceProvider
			}
			if tt.apikey == nil {
				api := mockAPIKeyValidator{}
				tt.apikey = api.ValidateAPIKey
			}

			faExt, err := newExtension(ctx, &Config{ReporterPeriod: DefaultReporterPeriod}, set, clientutil.ValidateAPIKey, tt.provider, tt.forwarder)
			assert.NoError(t, err)
			mcc := &mockComponentChecker{
				ccFunc: faExt.isComponentConfigured,
				maFunc: faExt.isModuleAvailable,
				hcFunc: faExt.isHealthCheckV2Enabled,
			}
			faExt.componentChecker = mcc
			err = faExt.forwarder.Start()
			defer faExt.forwarder.Stop()
			assert.NoError(t, err)

			err = faExt.NotifyConfig(ctx, conf)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			// Verify that the configuration is correctly set
			assert.Equal(t, conf, faExt.collectorConfig)

			// Verify that the collectorConfigStringMap contains all the items of expectedConfig
			for key, expectedValue := range tt.expectedConfig {
				actualValue, exists := faExt.collectorConfigStringMap[key]
				assert.True(t, exists, "Expected key %s not found in collectorConfigStringMap", key)
				assert.Equal(t, expectedValue, actualValue, "Value for key %s does not match", key)
			}

			// Check if the expected log message is present
			if tt.expectedLog != "" {
				found := false
				for _, log := range logs.All() {
					if log.Message == tt.expectedLog {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected log message not found")
			}
		})
	}
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
				Hostname:       "test-hostname",
				ReporterPeriod: DefaultReporterPeriod,
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
				Hostname:       "",
				ReporterPeriod: DefaultReporterPeriod,
			},
			apiKeyValidator: &mockAPIKeyValidator{
				err: nil,
			},
			sourceProviderGetter: &mockSourceProviderGetter{
				provider: &mockSourceProvider{hostname: "", err: fmt.Errorf("hostname detection failed")},
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
				Hostname:       "test-hostname",
				ReporterPeriod: DefaultReporterPeriod,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t)
			telemetry := componenttest.NewNopTelemetrySettings()
			telemetry.Logger = logger
			sp, _ := tt.sourceProviderGetter.GetSourceProvider(telemetry, tt.providedHostname, 15*time.Second)
			hostname, hostnameSource, err := getHostname(ctx, tt.providedHostname, sp)

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

func TestUpdateHostname(t *testing.T) {
	tests := []struct {
		name             string
		initialHostname  string
		configHostname   any
		providerHostname string
		providerError    error
		expectedHostname string
		expectedSource   string
		expectedLogs     []string
	}{
		{
			name:             "Hostname provided in config",
			initialHostname:  "",
			configHostname:   "test-hostname",
			providerHostname: "inferred-hostname",
			providerError:    nil,
			expectedHostname: "test-hostname",
			expectedSource:   "config",
			expectedLogs:     []string{},
		},
		{
			name:             "Hostname empty in config, inferred successfully",
			initialHostname:  "",
			configHostname:   "",
			providerHostname: "inferred-hostname",
			providerError:    nil,
			expectedHostname: "inferred-hostname",
			expectedSource:   "inferred",
			expectedLogs:     []string{"Hostname in config is empty, inferring hostname", "Inferred hostname"},
		},
		{
			name:             "Hostname empty in config, inference failed",
			initialHostname:  "",
			configHostname:   "",
			providerHostname: "",
			providerError:    fmt.Errorf("hostname detection failed"),
			expectedHostname: "",
			expectedSource:   "unset",
			expectedLogs:     []string{"Hostname in config is empty, inferring hostname", "Failed to infer hostname, collector will not show in Fleet Automation"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a logger for testing
			core, logs := observer.New(zapcore.InfoLevel)
			logger := zap.New(core)

			// Create a mock source provider
			mockProvider := &mockSourceProvider{
				hostname: tt.providerHostname,
				err:      tt.providerError,
			}

			// Create the extension with the initial hostname
			ext := &fleetAutomationExtension{
				telemetry:        component.TelemetrySettings{Logger: logger},
				hostname:         tt.initialHostname,
				hostnameProvider: mockProvider,
				extensionID:      component.MustNewID(metadata.Type.String()),
				collectorConfig:  confmap.NewFromStringMap(map[string]any{extensionsKind: map[string]any{metadata.Type.String(): map[string]any{"hostname": tt.configHostname}}}),
			}

			// Call updateHostname
			ext.updateHostname(context.Background())

			// Verify the hostname and source
			assert.Equal(t, tt.expectedHostname, ext.hostname)
			assert.Equal(t, tt.expectedSource, ext.hostnameSource)

			// Verify the logs
			for _, expectedLog := range tt.expectedLogs {
				found := false
				for _, log := range logs.All() {
					if log.Message == expectedLog {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected log message not found: %s", expectedLog)
			}
		})
	}
}

type mockHost struct {
	moduleInfos service.ModuleInfos
	extensions  map[component.ID]component.Component
}

type exportModules interface {
	GetModuleInfos() service.ModuleInfos
	GetExtensions() map[component.ID]component.Component
}

func (m mockHost) GetModuleInfos() service.ModuleInfos {
	return m.moduleInfos
}

func (m mockHost) GetExtensions() map[component.ID]component.Component {
	return m.extensions
}

func TestFleetAutomationExtension_Start(t *testing.T) {
	tests := []struct {
		name          string
		forwarder     defaultForwarderInterface
		expectedError string
		host          exportModules
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
		{
			name:          "host implements exportModules interface",
			forwarder:     mockForwarder{},
			expectedError: "",
			host: mockHost{
				moduleInfos: service.ModuleInfos{},
			},
		},
		{
			name:          "host doesn't implement GetModuleInfos()",
			forwarder:     mockForwarder{},
			expectedError: "",
			host:          nil,
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

			err := ext.Start(ctx, tt.host)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
			err = ext.Shutdown(ctx)
			assert.NoError(t, err)
		})
	}
}

func TestFleetAutomationExtension_Shutdown(t *testing.T) {
	tests := []struct {
		name          string
		forwarder     defaultForwarderInterface
		expectedError string
		httpServer    *http.Server
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
		{
			name: "non-nil http server",
			httpServer: &http.Server{
				Addr:         ":8080",
				ReadTimeout:  5 * time.Second,  // Set read timeout to 5 seconds
				WriteTimeout: 10 * time.Second, // Set write timeout to 10 seconds
			},
			forwarder:     mockForwarder{},
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

			if tt.httpServer != nil {
				ext.httpServer = tt.httpServer
			}

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

func TestCheckHealthCheckV2(t *testing.T) {
	tests := []struct {
		name                   string
		collectorConfig        map[string]any
		isComponentConfigured  bool
		isModuleAvailable      bool
		isHealthCheckV2Enabled bool
		expectedLogs           []string
		hcfunc                 mockHealthCheckV2Func
	}{
		{
			name: "HealthCheckV2 configured and enabled",
			collectorConfig: map[string]any{
				"extensions": map[string]any{
					"healthcheckv2": map[string]any{
						"enabled": true,
					},
				},
			},
			isComponentConfigured:  true,
			isModuleAvailable:      true,
			isHealthCheckV2Enabled: true,
			expectedLogs:           []string{},
		},
		{
			name: "HealthCheckV2 configured but not enabled",
			collectorConfig: map[string]any{
				"extensions": map[string]any{
					"healthcheckv2": map[string]any{
						"enabled": false,
					},
				},
			},
			isComponentConfigured:  true,
			isModuleAvailable:      true,
			isHealthCheckV2Enabled: false,
			expectedLogs:           []string{"healthcheckv2 extension is included in your collector config but not properly configured"},
		},
		{
			name: "HealthCheckV2 not configured but module available",
			collectorConfig: map[string]any{
				"extensions": map[string]any{},
			},
			isComponentConfigured:  false,
			isModuleAvailable:      true,
			isHealthCheckV2Enabled: false,
			expectedLogs:           []string{"healthcheckv2 extension is included with your collector but not configured; component status will not be available in Datadog Fleet page"},
		},
		{
			name: "HealthCheckV2 not configured and module not available",
			collectorConfig: map[string]any{
				"extensions": map[string]any{},
			},
			isComponentConfigured:  false,
			isModuleAvailable:      false,
			isHealthCheckV2Enabled: false,
			expectedLogs:           []string{},
		},
		{
			name: "Error retrieving HealthCheckV2 config",
			collectorConfig: map[string]any{
				"extensions": map[string]any{},
			},
			isComponentConfigured:  true,
			isModuleAvailable:      true,
			isHealthCheckV2Enabled: false,
			expectedLogs:           []string{"Failed to get healthcheckv2 config"},
		},
		{
			name: "Error checking HealthCheckV2 status",
			collectorConfig: map[string]any{
				"extensions": map[string]any{
					"healthcheckv2": map[string]any{
						"enabled": true,
					},
				},
			},
			isComponentConfigured:  true,
			isModuleAvailable:      true,
			isHealthCheckV2Enabled: false,
			hcfunc:                 func() (bool, error) { return false, fmt.Errorf("failed to check healthcheckv2 status") },
			expectedLogs:           []string{"failed to check healthcheckv2 status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a logger for testing
			core, logs := observer.New(zapcore.InfoLevel)
			logger := zap.New(core)
			id := component.MustNewID("healthcheckv2")
			// Create the extension with the test configuration
			ext := &fleetAutomationExtension{
				telemetry:            component.TelemetrySettings{Logger: logger},
				collectorConfig:      confmap.NewFromStringMap(tt.collectorConfig),
				healthCheckV2ID:      &id,
				healthCheckV2Enabled: tt.isHealthCheckV2Enabled,
			}

			// Mock the isComponentConfigured and isModuleAvailable methods
			mcc := &mockComponentChecker{
				ccFunc: func(string, string) (bool, *component.ID) {
					return tt.isComponentConfigured, ext.healthCheckV2ID
				},
				maFunc: func(string, string) bool { return tt.isModuleAvailable },
				hcFunc: func() (bool, error) { return tt.isHealthCheckV2Enabled, nil },
			}
			if tt.hcfunc != nil {
				mcc.hcFunc = tt.hcfunc
			}
			ext.componentChecker = mcc

			// Call checkHealthCheckV2
			ext.checkHealthCheckV2()

			// Verify the logs
			for _, expectedLog := range tt.expectedLogs {
				found := false
				for _, log := range logs.All() {
					if log.Message == expectedLog {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected log message not found: %s", expectedLog)
			}

			// Verify the healthCheckV2Enabled flag
			assert.Equal(t, tt.isHealthCheckV2Enabled, ext.healthCheckV2Enabled)
		})
	}
}

type (
	mockComponentConfiguredFunc func(string, string) (bool, *component.ID)
	mockModuleAvailableFunc     func(string, string) bool
	mockHealthCheckV2Func       func() (bool, error)
	mockComponentChecker        struct {
		ccFunc mockComponentConfiguredFunc
		maFunc mockModuleAvailableFunc
		hcFunc mockHealthCheckV2Func
	}
)

func (m *mockComponentChecker) isComponentConfigured(name string, kind string) (bool, *component.ID) {
	return m.ccFunc(name, kind)
}

func (m *mockComponentChecker) isModuleAvailable(name string, kind string) bool {
	return m.maFunc(name, kind)
}

func (m *mockComponentChecker) isHealthCheckV2Enabled() (bool, error) {
	return m.hcFunc()
}

func newNilForwarder(coreconfig.Component, corelog.Component) defaultforwarder.Forwarder {
	return nil
}

type mockSourceProvider struct {
	hostname string
	err      error
}

func (m *mockSourceProvider) Source(context.Context) (source.Source, error) {
	if m.err != nil {
		return source.Source{}, m.err
	}
	return source.Source{Identifier: m.hostname}, nil
}

type mockSourceProviderGetter struct {
	provider source.Provider
	err      error
}

func (m *mockSourceProviderGetter) GetSourceProvider(component.TelemetrySettings, string, time.Duration) (source.Provider, error) {
	return m.provider, m.err
}

type mockAPIKeyValidator struct {
	err error
}

func (m *mockAPIKeyValidator) ValidateAPIKey(context.Context, string, *zap.Logger, *datadog.APIClient) error {
	return m.err
}

type mockForwarder struct {
	startError       error
	stopError        error
	state            uint32
	failSendMetadata bool
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

func (m mockForwarder) SubmitV1Series(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitV1Intake(transaction.BytesPayloads, transaction.Kind, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitV1CheckRuns(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitSeries(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitSketchSeries(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitHostMetadata(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitAgentChecksMetadata(transaction.BytesPayloads, http.Header) error {
	return nil
}

func (m mockForwarder) SubmitMetadata(transaction.BytesPayloads, http.Header) error {
	if m.failSendMetadata {
		return fmt.Errorf("failed to send metadata")
	}
	return nil
}

func (m mockForwarder) SubmitProcessChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitProcessDiscoveryChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitProcessEventChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitRTProcessChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitContainerChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitRTContainerChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitConnectionChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitOrchestratorChecks(transaction.BytesPayloads, http.Header, int) (chan defaultforwarder.Response, error) {
	return nil, nil
}

func (m mockForwarder) SubmitOrchestratorManifests(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return nil, nil
}
