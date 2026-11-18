// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package remoteconfig // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/remoteconfig"

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server adapts the published Agent RC engine to the SDK request protocol.
type Server struct {
	config     *Config
	settings   extension.Settings
	newService func(context.Context, component.Host) (remoteConfigService, error)
	service    remoteConfigService
}

func (e *Server) Start(ctx context.Context, host component.Host) error {
	service, err := e.newService(ctx, host)
	if err != nil {
		return err
	}
	e.service = service
	service.Start()
	return nil
}

func (e *Server) Shutdown(context.Context) error {
	if e.service == nil {
		return nil
	}
	return e.service.Stop()
}

func (e *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		http.Error(w, "query parameters are not supported", http.StatusBadRequest)
		return
	}
	if r.URL.Path != "/v0.7/config" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request pbgo.ClientGetConfigsRequest
	limit := e.config.ServerConfig.MaxRequestBodySize
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	if err := decoder.Decode(&request); err != nil {
		writeDecodeError(w, err)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeDecodeError(w, err)
		return
	}
	if request.Client == nil || !request.Client.IsTracer || request.Client.ClientTracer == nil || request.Client.IsAgent || request.Client.IsUpdater {
		http.Error(w, "a tracer client is required", http.StatusBadRequest)
		return
	}
	tracer := request.Client.ClientTracer
	tracer.Service, _ = normalize.NormalizeService(tracer.Service, tracer.Language)
	tracer.Env = normalize.NormalizeTagValue(tracer.Env)
	ctx, cancel := context.WithTimeout(r.Context(), e.config.RequestTimeout)
	defer cancel()
	response, err := e.service.ClientGetConfigs(ctx, &request)
	if err != nil {
		code := http.StatusInternalServerError
		switch status.Code(err) {
		case codes.InvalidArgument:
			code = http.StatusBadRequest
		case codes.DeadlineExceeded:
			code = http.StatusGatewayTimeout
		case codes.Unavailable:
			code = http.StatusServiceUnavailable
		}
		http.Error(w, http.StatusText(code), code)
		return
	}
	writeJSON(w, response)
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "invalid Remote Configuration request", http.StatusBadRequest)
}

func writeJSON(w http.ResponseWriter, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "could not encode Remote Configuration response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
