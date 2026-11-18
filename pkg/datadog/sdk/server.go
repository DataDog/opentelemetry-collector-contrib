// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package sdk // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/sdk"

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
)

// Route adds a capability whose handler and optional component share the SDK listener.
type Route struct {
	Path      string
	Method    string
	Timeout   time.Duration
	Handler   http.Handler
	Component extension.Extension
}

type sdkServer struct {
	config    *Config
	settings  extension.Settings
	telemetry *instrumentationTelemetry
	routes    []Route
	started   []extension.Extension
	server    *http.Server
	listener  net.Listener
	cancel    context.CancelFunc
	done      chan struct{}
}

// New serves independently enabled SDK capabilities on one listener.
func New(set extension.Settings, c *Config, routes ...Route) (extension.Extension, error) {
	if err := ValidateConfig(c); err != nil {
		return nil, err
	}
	if !c.InstrumentationTelemetry.Enabled && len(routes) == 0 {
		return nil, errors.New("at least one SDK capability must be enabled")
	}
	seen := map[string]bool{"/info": true, telemetryPath: true}
	for _, route := range routes {
		if !strings.HasPrefix(route.Path, "/") || strings.ContainsAny(route.Path, "?#") || seen[route.Path] || route.Method == "" || route.Timeout <= 0 || route.Handler == nil {
			return nil, errors.New("SDK routes require a unique path, method, positive timeout and handler")
		}
		seen[route.Path] = true
	}
	config := *c
	s := &sdkServer{config: &config, settings: set, routes: append([]Route(nil), routes...)}
	if c.InstrumentationTelemetry.Enabled {
		config.ServerConfig.MaxRequestBodySize = max(config.ServerConfig.MaxRequestBodySize, c.InstrumentationTelemetry.MaxRequestBytes)
		s.telemetry = newInstrumentationTelemetry(c.InstrumentationTelemetry, c.API, c.Backend)
	}
	return s, nil
}

func (s *sdkServer) timeout(path string) time.Duration {
	if path == telemetryPath && s.telemetry != nil {
		return s.telemetry.config.Timeout
	}
	for _, route := range s.routes {
		if path == route.Path {
			return route.Timeout
		}
	}
	return 5 * time.Second
}

func (s *sdkServer) Start(ctx context.Context, host component.Host) error {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.TimeoutHandler(http.HandlerFunc(s.serveHTTP), s.timeout(r.URL.Path), "SDK request timed out").ServeHTTP(w, r)
	})
	config := s.config.ServerConfig
	server, err := config.ToServer(ctx, host.GetExtensions(), s.settings.TelemetrySettings, handler)
	if err != nil {
		return err
	}
	if s.telemetry != nil {
		opaque := config
		opaque.CompressionAlgorithms = []string{}
		opaqueServer, opaqueErr := opaque.ToServer(ctx, host.GetExtensions(), s.settings.TelemetrySettings, handler)
		if opaqueErr != nil {
			return opaqueErr
		}
		normal := server.Handler
		server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == telemetryPath {
				opaqueServer.Handler.ServeHTTP(w, r)
				return
			}
			normal.ServeHTTP(w, r)
		})
	}
	listener, err := config.ToListener(ctx)
	if err != nil {
		return err
	}
	lifetime, cancel := context.WithCancel(context.Background())
	cleanup := func() {
		cancel()
		for _, started := range slices.Backward(s.started) {
			_ = started.Shutdown(ctx)
		}
		s.started = nil
		if s.telemetry != nil {
			s.telemetry.close()
		}
		_ = listener.Close()
	}
	if s.telemetry != nil {
		if err := s.telemetry.start(lifetime, host, s.settings.TelemetrySettings); err != nil {
			cleanup()
			return err
		}
	}
	for _, route := range s.routes {
		if route.Component != nil {
			if err := route.Component.Start(lifetime, host); err != nil {
				cleanup()
				return err
			}
			s.started = append(s.started, route.Component)
		}
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
	if s.telemetry != nil {
		s.telemetry.close()
	}
	for _, started := range slices.Backward(s.started) {
		err = errors.Join(err, started.Shutdown(ctx))
	}
	s.started = nil
	return err
}

func (s *sdkServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		http.Error(w, "query parameters are not supported", http.StatusBadRequest)
		return
	}
	var method string
	var selected *Route
	switch {
	case r.URL.Path == "/info":
		method = http.MethodGet
	case r.URL.Path == telemetryPath && s.telemetry != nil:
		method = http.MethodPost
	default:
		for i := range s.routes {
			if r.URL.Path == s.routes[i].Path {
				selected = &s.routes[i]
				method = selected.Method
				break
			}
		}
		if selected == nil {
			http.NotFound(w, r)
			return
		}
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/info" {
		endpoints := make([]string, 0, len(s.routes)+1)
		for _, route := range s.routes {
			endpoints = append(endpoints, route.Path)
		}
		if s.telemetry != nil {
			endpoints = append(endpoints, "/telemetry/proxy/")
		}
		body, err := json.Marshal(map[string]any{"version": s.config.Version, "endpoints": endpoints})
		if err != nil {
			http.Error(w, "could not encode SDK capabilities", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	if selected != nil {
		selected.Handler.ServeHTTP(w, r)
		return
	}
	s.telemetry.serveHTTP(w, r)
}
