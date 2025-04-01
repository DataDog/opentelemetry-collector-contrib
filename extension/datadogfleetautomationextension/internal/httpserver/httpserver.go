// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/componentchecker"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/payload"
)

const (
	ServerPort = 8088
)

var nowFunc = time.Now

// defaultForwarderInterface is wrapper for methods in datadog-agent DefaultForwarder struct
type defaultForwarderInterface interface {
	defaultforwarder.Forwarder
	Start() error
	State() uint32
	Stop()
}

// Server represents an HTTP server for the Fleet Automation extension
type Server struct {
	server     *http.Server
	logger     *zap.Logger
	serializer serializer.MetricSerializer
	forwarder  defaultForwarderInterface
	cancel     context.CancelFunc
}

// NewServer creates a new HTTP server instance
func NewServer(logger *zap.Logger, serializer serializer.MetricSerializer, forwarder defaultforwarder.Forwarder) *Server {
	f, ok := forwarder.(defaultForwarderInterface)
	if !ok {
		return nil
	}
	return &Server{
		logger:     logger,
		serializer: serializer,
		forwarder:  f,
	}
}

// Stop shuts down the HTTP server
func (s *Server) Stop() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(ctx); err != nil {
			s.logger.Error("Failed to shutdown HTTP server", zap.Error(err))
		}
	}
	if s.cancel != nil {
		s.cancel()
	}
}

// Start starts the HTTP server and begins sending payloads periodically
func (s *Server) Start(
	handler func(w http.ResponseWriter, r *http.Request),
) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	s.server = &http.Server{
		Addr:         ":" + fmt.Sprintf("%d", ServerPort),
		Handler:      http.HandlerFunc(handler),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		BaseContext:  func(net.Listener) context.Context { return ctx },
	}

	// Start HTTP server
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Monitor context cancellation
	go func() {
		<-ctx.Done()
		if err := s.server.Shutdown(context.Background()); err != nil {
			s.logger.Error("Error during server shutdown", zap.Error(err))
		}
	}()

	s.logger.Info("HTTP Server started on port " + fmt.Sprintf("%d", ServerPort))
}

// PrepareAndSendFleetAutomationPayloads prepares and sends the fleet automation payloads
func PrepareAndSendFleetAutomationPayloads(
	logger *zap.Logger,
	serializer serializer.MetricSerializer,
	forwarder defaultForwarderInterface,
	hostname string,
	uuid string,
	componentStatus map[string]any,
	moduleInfo interface{},
	collectorConfigStringMap map[string]any,
	agentMetadataPayload payload.AgentMetadata,
	otelMetadataPayload payload.OtelMetadata,
	otelCollectorPayload payload.OtelCollector,
) (*payload.CombinedPayload, error) {
	healthStatus := componentchecker.DataToFlattenedJSONString(componentStatus, false, false)
	otelMetadataPayload.EnvironmentVariableConfiguration = componentchecker.DataToFlattenedJSONString(componentStatus, false, false)

	// add full components list to Provided Configuration
	moduleInfoJSON := componentchecker.PopulateFullComponentsJSON(moduleInfo.(service.ModuleInfos), collectorConfigStringMap)
	otelMetadataPayload.ProvidedConfiguration = componentchecker.DataToFlattenedJSONString(moduleInfoJSON, false, false)

	// add active components list to Provided Configuration, if available
	activeComponentsJSON, err := componentchecker.PopulateActiveComponentsJSON(collectorConfigStringMap, moduleInfoJSON, componentStatus, logger)
	if err != nil {
		logger.Error("Failed to populate active components JSON", zap.Error(err))
	} else {
		otelMetadataPayload.ProvidedConfiguration = componentchecker.DataToFlattenedJSONString(activeComponentsJSON, false, false) + "\n" + otelMetadataPayload.ProvidedConfiguration
	}

	// add remaining data to otelCollectorPayload
	otelCollectorPayload.FullComponents = moduleInfoJSON.GetFullComponentsList()
	if activeComponentsJSON != nil {
		otelCollectorPayload.ActiveComponents = activeComponentsJSON.Components
	}
	otelCollectorPayload.HealthStatus = healthStatus

	// Create the combined payload
	ap := payload.AgentPayload{
		Hostname:  hostname,
		Timestamp: nowFunc().UnixNano(),
		Metadata:  agentMetadataPayload,
		UUID:      uuid,
	}

	p := payload.OtelAgentPayload{
		Hostname:  hostname,
		Timestamp: nowFunc().UnixNano(),
		Metadata:  otelMetadataPayload,
		UUID:      uuid,
	}

	oc := payload.OtelCollectorPayload{
		Hostname:  hostname,
		Timestamp: nowFunc().UnixNano(),
		Metadata:  otelCollectorPayload,
		UUID:      uuid,
	}

	// Use datadog-agent serializer to send these payloads
	if forwarder.State() == defaultforwarder.Started {
		err = serializer.SendMetadata(&ap)
		if err != nil {
			return nil, fmt.Errorf("failed to send datadog_agent payload: %w", err)
		}
		err = serializer.SendMetadata(&p)
		if err != nil {
			return nil, fmt.Errorf("failed to send datadog_agent_otel payload: %w", err)
		}
		err = serializer.SendMetadata(&oc)
		if err != nil {
			return nil, fmt.Errorf("failed to send otel_collector payload: %w", err)
		}
	} else {
		logger.Warn("Forwarder is not started, skipping sending payloads")
	}

	combinedPayload := payload.CombinedPayload{
		CollectorPayload: oc,
		OtelPayload:      p,
		AgentPayload:     ap,
	}
	return &combinedPayload, nil
}

// HandleMetadata writes the metadata payloads to the response writer and sends them to the Datadog backend
func HandleMetadata(
	w http.ResponseWriter,
	logger *zap.Logger,
	hostnameSource string,
	hostname string,
	uuid string,
	componentStatus map[string]any,
	moduleInfo interface{},
	collectorConfigStringMap map[string]any,
	agentMetadataPayload payload.AgentMetadata,
	otelMetadataPayload payload.OtelMetadata,
	otelCollectorPayload payload.OtelCollector,
	serializer serializer.MetricSerializer,
	forwarder defaultForwarderInterface,
) {
	// Prepare and send the fleet automation payloads
	combinedPayload, err := PrepareAndSendFleetAutomationPayloads(
		logger,
		serializer,
		forwarder,
		hostname,
		uuid,
		componentStatus,
		moduleInfo,
		collectorConfigStringMap,
		agentMetadataPayload,
		otelMetadataPayload,
		otelCollectorPayload,
	)
	if err != nil {
		logger.Error("Failed to prepare and send fleet automation payloads", zap.Error(err))
		if w != nil {
			http.Error(w, "Failed to prepare and send fleet automation payloads", http.StatusInternalServerError)
		}
		return
	}

	// Marshal the combined payload to JSON
	jsonData, err := json.MarshalIndent(combinedPayload, "", "  ")
	if err != nil {
		logger.Error("Failed to marshal combined payload for local http response", zap.Error(err))
		if w != nil {
			http.Error(w, "Failed to marshal combined payload", http.StatusInternalServerError)
		}
		return
	}

	if w != nil {
		// Write the JSON response
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(jsonData)
		if err != nil {
			logger.Error("Failed to write response to local metadata request", zap.Error(err))
		}
	}
}
