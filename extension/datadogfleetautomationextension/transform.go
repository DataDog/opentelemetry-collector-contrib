// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"encoding/json"
	"errors"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/service"

	"go.uber.org/zap"
)

const (
	receiverType   = "receiver"
	receiversType  = "receivers"
	processorType  = "processor"
	processorsType = "processors"
	exporterType   = "exporter"
	exportersType  = "exporters"
	extensionType  = "extension"
	extensionsType = "extensions"
	connectorType  = "connector"
	connectorsType = "connectors"
	providerType   = "provider"
	providersType  = "providers"
	converterType  = "converter"
	convertersType = "converters"
)

var (
	typesToType map[string]string = map[string]string{
		receiversType:  receiverType,
		processorsType: processorType,
		exportersType:  exporterType,
		extensionsType: extensionType,
		connectorsType: connectorType,
		providersType:  providerType,
		convertersType: converterType,
	}
)

func (e *fleetAutomationExtension) isComponentConfigured(name string, componentsType string) bool {
	if components, ok := e.collectorConfigStringMap[componentsType]; ok {
		if componentMap, ok := components.(map[string]interface{}); ok {
			if _, ok := componentMap[name]; ok {
				return true
			}
		}
	}
	return false
}

func (e *fleetAutomationExtension) isModuleAvailable(componentName string, componentType string) bool {
	if componentType == receiverType {
		if _, ok := e.moduleInfo.Receiver[component.MustNewType(componentName)]; ok {
			return true
		}
	}
	if componentType == processorType {
		if _, ok := e.moduleInfo.Processor[component.MustNewType(componentName)]; ok {
			return true
		}
	}
	if componentType == exporterType {
		if _, ok := e.moduleInfo.Exporter[component.MustNewType(componentName)]; ok {
			return true
		}
	}
	if componentType == extensionType {
		if _, ok := e.moduleInfo.Extension[component.MustNewType(componentName)]; ok {
			return true
		}
	}
	if componentType == connectorType {
		if _, ok := e.moduleInfo.Connector[component.MustNewType(componentName)]; ok {
			return true
		}
	}
	// TODO: add Provider and converter types after upstream change accepted to add these to moduleinfos
	return false
}

func (e *fleetAutomationExtension) isHealthCheckV2Enabled() (bool, error) {
	if useV2, ok := e.healthCheckV2Config["use_v2"].(bool); ok && useV2 {
		if httpConfig, ok := e.healthCheckV2Config["http"].(map[string]interface{}); ok {
			if statusConfig, ok := httpConfig["status"].(map[string]interface{}); ok {
				if enabled, ok := statusConfig["enabled"].(bool); ok && enabled {
					return true, nil
				} else {
					return false, errors.New("healthcheckv2 extension is enabled but http status check is not enabled; component status will not be available")
				}
			} else {
				return false, errors.New("healthcheckv2 extension is enabled but http status is not configured; component status will not be available")
			}
		} else {
			return false, errors.New("healthcheckv2 extension is enabled but http endpoint is not configured; component status will not be available")
		}
	} else {
		return false, errors.New("healthcheckv2 extension is enabled but is set to legacy mode; component status will not be available")
	}
}

func (e *fleetAutomationExtension) getComponentConfig(name string, componentsType string) map[string]any {
	if components, ok := e.collectorConfigStringMap[componentsType]; ok {
		if componentMap, ok := components.(map[string]interface{}); ok {
			if componentConfig, ok := componentMap[name]; ok {
				if configMap, ok := componentConfig.(map[string]any); ok {
					return configMap
				}
			}
		}
	}
	return nil
}

// subject to change/removal as healthcheckv2 is in "development status"
// unclear if we should wait until this is more stabilized to add individual component status health
// to the payloads we send
func (e *fleetAutomationExtension) getComponentHealthStatus(name string, componentsType string) map[string]any {
	componentType, ok := typesToType[componentsType]
	if !ok {
		return nil
	}
	result := make(map[string]any)
	// scrape components list for extensions, pipelines list for receivers, processors, exporters, connectors
	if componentsConfig, ok := e.componentStatus["components"].(map[string]any); ok {
		if componentsType == extensionsType {
			if componentStatus, ok := componentsConfig[componentsType].(map[string]any); ok {
				if receiversStatus, ok := componentStatus["components"].(map[string]any); ok {
					componentName := componentType + ":" + name
					if componentStatus, ok := receiversStatus[componentName].(map[string]any); ok {
						result = componentStatus
					}
				}
			}
		} else {
			// extract component from pipeline list
			for key, value := range componentsConfig {
				if key == extensionsType {
					continue
				}
				if pipelineMap, ok := value.(map[string]any); ok {
					if components, ok := pipelineMap["components"].(map[string]any); ok {
						for id, status := range components {
							idParts := strings.Split(id, ":")
							if len(idParts) != 2 {
								continue
							}
							kind := idParts[0]
							fullID := idParts[1]
							idSplit := strings.Split(fullID, "/")
							compType := idSplit[0]
							if kind == componentType && name == compType {
								if componentStatus, ok := status.(map[string]any); ok {
									result[key] = map[string]any{
										id: componentStatus,
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return result
}

func (e *fleetAutomationExtension) populateModuleInfoJSON() moduleInfoJSON {
	var components []collectorComponent
	for _, field := range []struct {
		names string
		data  map[component.Type]service.ModuleInfo
		name  string
	}{
		{receiversType, e.moduleInfo.Receiver, receiverType},
		{processorsType, e.moduleInfo.Processor, processorType},
		{exportersType, e.moduleInfo.Exporter, exporterType},
		{extensionsType, e.moduleInfo.Extension, extensionType},
		{connectorsType, e.moduleInfo.Connector, connectorType},
		// TODO: add Providers and Converters after upstream change accepted to add these to moduleinfos
	} {
		for comp, builderRef := range field.data {
			parts := strings.Split(builderRef.BuilderRef, " ")
			if len(parts) != 2 {
				e.telemetry.Logger.Warn("Invalid extension info", zap.String("extension", builderRef.BuilderRef))
				continue
			}
			enabled := e.isComponentConfigured(comp.String(), field.names)
			status := ""
			// Remains to be seen if we should wait until healthcheckv2 is marked stable to use this
			if enabled && e.healthCheckV2Enabled {
				statusMap := e.getComponentHealthStatus(comp.String(), field.names)
				if statusMap != nil {
					statusJson, err := json.MarshalIndent(statusMap, "", "  ")
					if err != nil {
						e.telemetry.Logger.Error("Failed to marshal component healthcheck status", zap.Error(err))
					} else {
						status = string(statusJson)
						status = strings.ReplaceAll(status, "\"", "")
						status = strings.ReplaceAll(status, "\n", "")
					}
				}
			}
			components = append(components, collectorComponent{
				Name:            comp.String(),
				Type:            field.name,
				Module:          parts[0],
				Version:         parts[1],
				Enabled:         enabled,
				ComponentStatus: status,
			})
		}
	}

	return moduleInfoJSON{
		Components: components,
	}
}
