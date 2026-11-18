// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package sdk

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
)

type routeComponent struct {
	started, stopped bool
	startErr         error
}

func (c *routeComponent) Start(context.Context, component.Host) error {
	c.started = true
	return c.startErr
}
func (c *routeComponent) Shutdown(context.Context) error { c.stopped = true; return nil }

func routeConfig() *Config {
	c := DefaultConfig()
	c.ServerConfig.NetAddr.Endpoint = "127.0.0.1:0"
	c.API.Key = "collector-secret"
	c.Version = "7.85.0"
	return c
}

func TestAdditionalRouteDiscoveryAndDecoding(t *testing.T) {
	for _, telemetry := range []bool{false, true} {
		t.Run(map[bool]string{false: "route only", true: "with telemetry"}[telemetry], func(t *testing.T) {
			config := routeConfig()
			if telemetry {
				config = telemetryServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }).config
			}
			lifecycle := &routeComponent{}
			payload := []byte(`{"client":"sdk"}`)
			route := Route{Path: "/v0.7/config", Method: http.MethodPost, Timeout: time.Second, Component: lifecycle, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || !bytes.Equal(payload, body) {
					t.Error("route did not receive decoded request")
				}
				w.WriteHeader(http.StatusOK)
			})}
			ext, err := New(extension.Settings{ID: component.NewID(Type), TelemetrySettings: componenttest.NewNopTelemetrySettings()}, config, route)
			require.NoError(t, err)
			s := ext.(*sdkServer)
			address := startTelemetryServer(t, s)
			require.True(t, lifecycle.started)
			response, err := http.Get(address + "/info")
			require.NoError(t, err)
			var info struct{ Endpoints []string }
			require.NoError(t, json.NewDecoder(response.Body).Decode(&info))
			require.NoError(t, response.Body.Close())
			require.Contains(t, info.Endpoints, "/v0.7/config")
			require.Equal(t, telemetry, len(info.Endpoints) == 2)
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			_, err = writer.Write(payload)
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			req, err := http.NewRequest(http.MethodPost, address+route.Path, &compressed)
			require.NoError(t, err)
			req.Header.Set("Content-Encoding", "gzip")
			response, err = http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.NoError(t, s.Shutdown(t.Context()))
			require.True(t, lifecycle.stopped)
		})
	}
}

func TestAdditionalRouteTimeout(t *testing.T) {
	release, finished := make(chan struct{}), make(chan struct{})
	route := Route{Path: "/v0.7/config", Method: http.MethodPost, Timeout: 30 * time.Millisecond, Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { defer close(finished); <-release })}
	ext, err := New(extension.Settings{ID: component.NewID(Type), TelemetrySettings: componenttest.NewNopTelemetrySettings()}, routeConfig(), route)
	require.NoError(t, err)
	address := startTelemetryServer(t, ext.(*sdkServer))
	defer func() { close(release); <-finished }()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Post(address+route.Path, "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
}

func TestAdditionalRouteStartFailureReleasesResources(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	config := routeConfig()
	config.ServerConfig.NetAddr.Endpoint = address
	first, failed := &routeComponent{}, &routeComponent{startErr: errors.New("route unavailable")}
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	ext, err := New(extension.Settings{ID: component.NewID(Type), TelemetrySettings: componenttest.NewNopTelemetrySettings()}, config,
		Route{Path: "/first", Method: http.MethodPost, Timeout: time.Second, Handler: handler, Component: first},
		Route{Path: "/second", Method: http.MethodPost, Timeout: time.Second, Handler: handler, Component: failed})
	require.NoError(t, err)
	require.ErrorContains(t, ext.Start(t.Context(), componenttest.NewNopHost()), "route unavailable")
	require.True(t, first.started)
	require.True(t, first.stopped)
	listener, err = net.Listen("tcp", address)
	require.NoError(t, err)
	require.NoError(t, listener.Close())
}

func TestReservedSDKRoutesAreRejected(t *testing.T) {
	for _, path := range []string{"/info", telemetryPath, "/path?query=x", "not-a-path"} {
		route := Route{Path: path, Method: http.MethodPost, Timeout: time.Second, Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}
		_, err := New(extension.Settings{}, routeConfig(), route)
		require.Error(t, err)
	}
}
