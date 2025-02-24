// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/uuid"
	"go.uber.org/zap"
)

const (
	serverPort                   = 8088
	defaultHealthCheckV2Endpoint = "localhost:13133"
)

func (e *fleetAutomationExtension) startLocalConfigServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/metadata", e.handleMetadata)
	//TODO: let user specify port in config? Or remove?
	e.httpServer = &http.Server{
		Addr:    ":" + fmt.Sprintf("%d", serverPort),
		Handler: mux,
	}
	go func() {
		if err := e.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.telemetry.Logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Create a ticker that triggers every 20 minutes (FA has 1 hour TTL)
	// Start a goroutine that will send the Datadog fleet automation payload every 20 minutes
	go func(ticker *time.Ticker) {
		for {
			select {
			case <-ticker.C:
				// Call handleMetadata periodically
				e.handleMetadata(nil, nil)
			case <-e.done:
				e.telemetry.Logger.Info("Stopping datadog fleet automation payload sender")
				ticker.Stop()
				return
			}
		}
	}(e.ticker)

	e.telemetry.Logger.Info("HTTP Server started on port " + fmt.Sprintf("%d", serverPort))

	return nil
}

func (e *fleetAutomationExtension) getHealthCheckStatus() (map[string]any, error) {
	endpoint := "localhost:13133"
	path := "/health/status"
	if httpConfig, ok := e.healthCheckV2Config["http"].(map[string]interface{}); ok {
		if statusConfig, ok := httpConfig["status"].(map[string]interface{}); ok {
			if enabled, ok := statusConfig["enabled"].(bool); !enabled || !ok {
				return nil, fmt.Errorf("http health check v2 extension is not enabled")
			} else {
				if ep, ok := httpConfig["endpoint"].(string); ok && ep != "" {
					endpoint = ep
				}
				if p, ok := statusConfig["path"].(string); ok && p != "" {
					path = p
				}
			}
		}
	}

	// Construct the URL
	url := fmt.Sprintf("http://%s%s?verbose", endpoint, path)

	// Make the HTTP request
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to health check endpoint: %w", err)
	}
	defer resp.Body.Close()

	// Parse the JSON response
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return result, nil
}

// handleMetadata writes the metadata payloads to the response writer.
// It also sends these payloads to the Datadog backend
func (e *fleetAutomationExtension) handleMetadata(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.healthCheckV2Enabled {
		componentStatus, err := e.getHealthCheckStatus()
		if err != nil {
			e.telemetry.Logger.Error("Failed to get health check status", zap.Error(err))
		} else {
			e.componentStatus = componentStatus
			e.otelMetadataPayload.EnvironmentVariableConfiguration = dataToFlattenedJSONString(e.componentStatus, false)
		}
	}

	e.moduleInfoJSON = e.populateFullComponentsJSON()
	e.otelMetadataPayload.ProvidedConfiguration = dataToFlattenedJSONString(e.moduleInfoJSON, false)

	var err error
	e.activeComponentsJSON, err = e.populateActiveComponentsJSON()
	if err != nil {
		e.telemetry.Logger.Error("Failed to populate active components JSON", zap.Error(err))
	} else {
		e.otelMetadataPayload.ProvidedConfiguration = dataToFlattenedJSONString(e.activeComponentsJSON, false) + "\n" + e.otelMetadataPayload.ProvidedConfiguration
	}

	ap := agentPayload{
		Hostname:  e.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  e.agentMetadataPayload,
		UUID:      uuid.GetUUID(),
	}
	p := otelAgentPayload{
		Hostname:  e.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  e.otelMetadataPayload,
		UUID:      uuid.GetUUID(),
	}

	if e.hostnameSource == "unset" {
		e.telemetry.Logger.Debug("Skipping fleet automation payloads since the hostname is empty")
		if w != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("Fleet automation payloads not sent since the hostname is empty"))
		}
		return
	}
	e.serializer.SendMetadata(&ap)
	e.serializer.SendMetadata(&p)

	combinedPayload := CombinedPayload{
		AgentPayload: ap,
		OtelPayload:  p,
	}

	// Marshal the combined payload to JSON
	jsonData, err := json.MarshalIndent(combinedPayload, "", "  ")
	if err != nil {
		http.Error(w, "Failed to marshal combined payload", http.StatusInternalServerError)
		return
	}

	if w != nil {
		// Write the JSON response
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	}

}
