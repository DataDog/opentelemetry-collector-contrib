// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package sdk // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/sdk"

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
)

type sdkServer struct {
	config    *Config
	settings  extension.Settings
	telemetry *instrumentationTelemetry
	server    *http.Server
	listener  net.Listener
	cancel    context.CancelFunc
	done      chan struct{}
}

// New serves SDK instrumentation telemetry on a dedicated listener.
func New(set extension.Settings, c *Config) (extension.Extension, error) {
	if err := ValidateConfig(c); err != nil {
		return nil, err
	}
	if !c.InstrumentationTelemetry.Enabled {
		return nil, errors.New("instrumentation telemetry must be enabled")
	}
	config := *c
	config.ServerConfig.MaxRequestBodySize = max(config.ServerConfig.MaxRequestBodySize, c.InstrumentationTelemetry.MaxRequestBytes)
	return &sdkServer{config: &config, settings: set, telemetry: newInstrumentationTelemetry(c.InstrumentationTelemetry, c.API, c.Backend)}, nil
}

func (s *sdkServer) Start(ctx context.Context, host component.Host) error {
	handler := http.TimeoutHandler(http.HandlerFunc(s.serveHTTP), s.config.InstrumentationTelemetry.Timeout, "SDK request timed out")
	config := s.config.ServerConfig
	// Preserve compressed SDK bytes under the configured server TLS and auth.
	config.CompressionAlgorithms = []string{}
	server, err := config.ToServer(ctx, host.GetExtensions(), s.settings.TelemetrySettings, handler)
	if err != nil {
		return err
	}
	listener, err := config.ToListener(ctx)
	if err != nil {
		return err
	}
	lifetime, cancel := context.WithCancel(context.Background())
	if err := s.telemetry.start(lifetime, host, s.settings.TelemetrySettings); err != nil {
		cancel()
		_ = listener.Close()
		return err
	}
	s.server, s.listener, s.cancel, s.done = server, listener, cancel, make(chan struct{})
	go func() {
		defer close(s.done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
		}
	}()
	return nil
}

func (s *sdkServer) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	s.cancel()
	err := s.server.Shutdown(ctx)
	if err != nil {
		_ = s.server.Close()
	}
	<-s.done
	s.telemetry.close()
	return err
}

func (s *sdkServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		http.Error(w, "query parameters are not supported", http.StatusBadRequest)
		return
	}
	var method string
	switch r.URL.Path {
	case "/info":
		method = http.MethodGet
	case telemetryPath:
		method = http.MethodPost
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/info" {
		body, err := json.Marshal(map[string]any{"version": s.config.Version, "endpoints": []string{"/telemetry/proxy/"}})
		if err != nil {
			http.Error(w, "could not encode SDK capabilities", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	s.telemetry.serveHTTP(w, r)
}
