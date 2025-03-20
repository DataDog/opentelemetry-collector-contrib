// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/componentchecker"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/payload"
)

const (
	serverPort = 8088
)

var nowFunc = time.Now

func (e *fleetAutomationExtension) stopLocalConfigServer() {
	if e.httpServer != nil {
		if err := e.httpServer.Shutdown(context.Background()); err != nil {
			e.telemetry.Logger.Error("Failed to shutdown HTTP server", zap.Error(err))
		}
	}
	if e.done != nil {
		close(e.done)
	}
}

func (e *fleetAutomationExtension) startLocalConfigServer() {
	// TODO: let user specify port in config? Or remove?
	// TODO: Consider adding non-nil error return from this function
	go func() {
		if err := e.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.telemetry.Logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Create a ticker that triggers every 20 minutes (FA has 1 hour TTL)
	// Start a goroutine that will send the Datadog fleet automation payload every 20 minutes
	go func(ticker *time.Ticker) {
		if ticker == nil {
			return
		}
		for {
			select {
			case <-ticker.C:
				// Send fleet automation payload(s) periodically
				_, err := e.prepareAndSendFleetAutomationPayloads()
				if err != nil {
					e.telemetry.Logger.Error("Failed to prepare and send fleet automation payloads", zap.Error(err))
				}
			case <-e.done:
				e.telemetry.Logger.Info("Stopping datadog fleet automation payload sender")
				ticker.Stop()
				return
			}
		}
	}(e.ticker)

	e.telemetry.Logger.Info("HTTP Server started on port " + fmt.Sprintf("%d", serverPort))
}

func (e *fleetAutomationExtension) prepareAndSendFleetAutomationPayloads() (*payload.CombinedPayload, error) {
	healthStatus := componentchecker.DataToFlattenedJSONString(e.componentStatus, false, false)
	e.otelMetadataPayload.EnvironmentVariableConfiguration = componentchecker.DataToFlattenedJSONString(e.componentStatus, false, false)

	// add full components list to Provided Configuration
	e.ModuleInfoJSON = componentchecker.PopulateFullComponentsJSON(e.moduleInfo, e.collectorConfigStringMap)
	e.otelMetadataPayload.ProvidedConfiguration = componentchecker.DataToFlattenedJSONString(e.ModuleInfoJSON, false, false)

	// add active components list to Provided Configuration, if available
	activeComponentsJSON, err := componentchecker.PopulateActiveComponentsJSON(e.collectorConfigStringMap, e.ModuleInfoJSON, e.componentStatus, e.telemetry.Logger)
	if err != nil {
		e.telemetry.Logger.Error("Failed to populate active components JSON", zap.Error(err))
	} else {
		e.activeComponentsJSON = activeComponentsJSON
		e.otelMetadataPayload.ProvidedConfiguration = componentchecker.DataToFlattenedJSONString(e.activeComponentsJSON, false, false) + "\n" + e.otelMetadataPayload.ProvidedConfiguration
	}

	// add remaining data to otelCollectorPayload
	e.otelCollectorPayload.FullComponents = e.ModuleInfoJSON.GetFullComponentsList()
	if e.activeComponentsJSON != nil {
		e.otelCollectorPayload.ActiveComponents = e.activeComponentsJSON.Components
	}
	e.otelCollectorPayload.HealthStatus = healthStatus

	// Create the combined payload
	ap := payload.AgentPayload{
		Hostname:  e.hostname,
		Timestamp: nowFunc().UnixNano(),
		Metadata:  e.agentMetadataPayload,
		UUID:      e.uuid.String(),
	}

	p := payload.OtelAgentPayload{
		Hostname:  e.hostname,
		Timestamp: nowFunc().UnixNano(),
		Metadata:  e.otelMetadataPayload,
		UUID:      e.uuid.String(),
	}

	oc := payload.OtelCollectorPayload{
		Hostname:  e.hostname,
		Timestamp: nowFunc().UnixNano(),
		Metadata:  e.otelCollectorPayload,
		UUID:      e.uuid.String(),
	}

	// Use datadog-agent serializer to send these payloads
	if e.forwarder.State() == defaultforwarder.Started {
		err = e.serializer.SendMetadata(&ap)
		if err != nil {
			return nil, fmt.Errorf("failed to send datadog_agent payload: %w", err)
		}
		err = e.serializer.SendMetadata(&p)
		if err != nil {
			return nil, fmt.Errorf("failed to send datadog_agent_otel payload: %w", err)
		}
		err = e.serializer.SendMetadata(&oc)
		if err != nil {
			return nil, fmt.Errorf("failed to send otel_collector payload: %w", err)
		}
	} else {
		e.telemetry.Logger.Warn("Forwarder is not started, skipping sending payloads")
	}

	combinedPayload := payload.CombinedPayload{
		CollectorPayload: oc,
		OtelPayload:      p,
		AgentPayload:     ap,
	}
	return &combinedPayload, nil
}

// handleMetadata writes the metadata payloads to the response writer.
// It also sends these payloads to the Datadog backend via prepareAndSendFleetAutomationPayloads
func (e *fleetAutomationExtension) handleMetadata(w http.ResponseWriter, _ *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.hostnameSource == "unset" {
		e.telemetry.Logger.Info("Skipping fleet automation payloads since the hostname is empty")
		if w != nil {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte("Fleet automation payloads not sent since the hostname is empty"))
			if err != nil {
				e.telemetry.Logger.Error("Failed to write response to local metadata request", zap.Error(err))
			}
		}
		return
	}

	// call local helper function to prepare the fleet automation payloads
	// and transmit using serializer component
	combinedPayload, err := e.prepareAndSendFleetAutomationPayloads()
	if err != nil {
		e.telemetry.Logger.Error("Failed to prepare and send fleet automation payloads", zap.Error(err))
		if w != nil {
			http.Error(w, "Failed to prepare and send fleet automation payloads", http.StatusInternalServerError)
		}
		return
	}

	// Marshal the combined payload to JSON
	jsonData, err := json.MarshalIndent(combinedPayload, "", "  ")
	if err != nil {
		// prepareAndSendFleetAutomationPayloads is thoroughly tested, should not happen
		// TODO: come up with test that exercises this case anyway
		e.telemetry.Logger.Error("Failed to marshal combined payload for local http response", zap.Error(err))
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
			e.telemetry.Logger.Error("Failed to write response to local metadata request", zap.Error(err))
		}
	}
}
