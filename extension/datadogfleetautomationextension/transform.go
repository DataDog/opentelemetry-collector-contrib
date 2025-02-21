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
	receiverKind   = "receiver"
	receiversKind  = "receivers"
	processorKind  = "processor"
	processorsKind = "processors"
	exporterKind   = "exporter"
	exportersKind  = "exporters"
	extensionKind  = "extension"
	extensionsKind = "extensions"
	connectorKind  = "connector"
	connectorsKind = "connectors"
	providerKind   = "provider"
	providersKind  = "providers"
	converterKind  = "converter"
	convertersKind = "converters"
	pipelinesKind  = "pipelines"
)

var (
	kindsToKind map[string]string = map[string]string{
		receiversKind:  receiverKind,
		processorsKind: processorKind,
		exportersKind:  exporterKind,
		extensionsKind: extensionKind,
		connectorsKind: connectorKind,
		providersKind:  providerKind,
		convertersKind: converterKind,
	}
)

// isComponentConfigured checks if a given component is included in the collector config
func (e *fleetAutomationExtension) isComponentConfigured(typ string, componentsKind string) (bool, *component.ID) {
	// NOTE: this will only match the first instance of a component type in the config
	if components, ok := e.collectorConfigStringMap[componentsKind]; ok {
		if componentMap, ok := components.(map[string]any); ok {
			// iterate through componentMap, checking either if key == typ or key split by "/" has keySplit[0] as typ
			for key, _ := range componentMap {
				if key == typ {
					newID := component.NewID(component.MustNewType(typ))
					return true, &newID
				}
				keySplit := strings.Split(key, "/")
				if len(keySplit) == 2 && keySplit[0] == typ {
					newID := component.NewIDWithName(component.MustNewType(keySplit[0]), keySplit[1])
					return true, &newID
				}
			}
		}
	}
	return false, nil
}

// isModuleAvailable checks if a given gomod type is included in the collector ModuleInfos struct
func (e *fleetAutomationExtension) isModuleAvailable(componentType string, componentKind string) bool {
	if componentKind == receiverKind {
		if _, ok := e.moduleInfo.Receiver[component.MustNewType(componentType)]; ok {
			return true
		}
	}
	if componentKind == processorKind {
		if _, ok := e.moduleInfo.Processor[component.MustNewType(componentType)]; ok {
			return true
		}
	}
	if componentKind == exporterKind {
		if _, ok := e.moduleInfo.Exporter[component.MustNewType(componentType)]; ok {
			return true
		}
	}
	if componentKind == extensionKind {
		if _, ok := e.moduleInfo.Extension[component.MustNewType(componentType)]; ok {
			return true
		}
	}
	if componentKind == connectorKind {
		if _, ok := e.moduleInfo.Connector[component.MustNewType(componentType)]; ok {
			return true
		}
	}
	// TODO: add Provider and converter types after upstream change accepted to add these to moduleinfos
	return false
}

// isHealthCheckV2Enabled checks if healthcheckv2 is properly configured
// should be ran after e.isComponentConfigured for healthcheckv2 and e.getComponentConfig for healthcheckv2
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

// getComponentConfig looks for the component type in the appropriate config section
// TODO: allow matching of named components (only allows exact type check currently)
func (e *fleetAutomationExtension) getComponentConfig(id string, componentsKind string) map[string]any {
	if components, ok := e.collectorConfigStringMap[componentsKind]; ok {
		if componentMap, ok := components.(map[string]interface{}); ok {
			// TODO: allow matching of named components
			if componentConfig, ok := componentMap[id]; ok {
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
//
// requires e.componentStatus to be recently updated with e.getHealthCheckStatus()
func (e *fleetAutomationExtension) getComponentHealthStatus(id string, componentsKind string) map[string]any {
	componentKind, ok := kindsToKind[componentsKind]
	if !ok {
		return nil
	}
	result := make(map[string]any)
	// scrape components list for extensions, pipelines list for receivers, processors, exporters, connectors
	if componentsConfig, ok := e.componentStatus["components"].(map[string]any); ok {
		if componentsKind == extensionsKind {
			if componentStatus, ok := componentsConfig[componentsKind].(map[string]any); ok {
				if receiversStatus, ok := componentStatus["components"].(map[string]any); ok {
					componentNameInPipeline := componentKind + ":" + id
					if componentStatus, ok := receiversStatus[componentNameInPipeline].(map[string]any); ok {
						result = componentStatus
					}
				}
			}
		} else {
			// extract component from pipeline list
			for key, value := range componentsConfig {
				if key == extensionsKind {
					continue
				}
				if pipelineMap, ok := value.(map[string]any); ok {
					if components, ok := pipelineMap["components"].(map[string]any); ok {
						for component, status := range components {
							componentParts := strings.Split(component, ":")
							if len(componentParts) != 2 {
								continue
							}
							kind := componentParts[0]
							fullID := componentParts[1]
							if kind == componentKind && id == fullID {
								if componentStatus, ok := status.(map[string]any); ok {
									result[key] = map[string]any{
										component: componentStatus,
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

// populateFullComponentsJSON creates a moduleInfoJSON struct with all components from ModuleInfos
func (e *fleetAutomationExtension) populateFullComponentsJSON() *moduleInfoJSON {
	modInfo := newModuleInfoJSON()
	for _, field := range []struct {
		names string
		data  map[component.Type]service.ModuleInfo
		name  string
	}{
		{receiversKind, e.moduleInfo.Receiver, receiverKind},
		{processorsKind, e.moduleInfo.Processor, processorKind},
		{exportersKind, e.moduleInfo.Exporter, exporterKind},
		{extensionsKind, e.moduleInfo.Extension, extensionKind},
		{connectorsKind, e.moduleInfo.Connector, connectorKind},
		// TODO: add Providers and Converters after upstream change accepted to add these to moduleinfos
	} {
		for comp, builderRef := range field.data {
			parts := strings.Split(builderRef.BuilderRef, " ")
			if len(parts) != 2 {
				e.telemetry.Logger.Warn("Invalid extension info", zap.String("extension", builderRef.BuilderRef))
				continue
			}
			enabled, _ := e.isComponentConfigured(comp.String(), field.names)
			modInfo.addComponent(collectorModule{
				Type:              comp.String(),
				Kind:              field.name,
				Gomod:             parts[0],
				Version:           parts[1],
				IncludedInService: enabled,
			})
		}
	}
	return modInfo
}

// populateActiveComponentsJSON creates an activeComponentsJSON struct with all active components from the collector service pipelines
func (e *fleetAutomationExtension) populateActiveComponentsJSON() (*activeComponentsJSON, error) {
	var components []serviceComponent
	serviceMap, ok := e.collectorConfigStringMap["service"].(map[string]any)
	if !ok {
		e.telemetry.Logger.Error("Failed to get service map from collector config, cannot populate active components table")
		return &activeComponentsJSON{
			Components: components,
		}, errors.New("failed to get service map from collector config, cannot populate active components table")
	}
	for key, value := range serviceMap {
		if key == extensionsKind {
			extensionsList, ok := value.([]any)
			if !ok {
				e.telemetry.Logger.Info("Failed to get extensions list from service map")
				continue
			}
			for _, extension := range extensionsList {
				extensionString, ok := extension.(string)
				if !ok {
					e.telemetry.Logger.Info("Extensions list in service map config contains non-string value", zap.Any("extension", extension))
					continue
				}
				var id, name, typ, kind, gomod, version, status string
				fullID := strings.Split(extensionString, "/")
				if len(fullID) == 1 {
					// component does not have a name
					id = fullID[0]
					name = ""
				} else if len(fullID) == 2 {
					id = extensionString
					name = fullID[1]
				} else {
					e.telemetry.Logger.Info("Invalid extension ID", zap.String("extension", extensionString))
					continue
				}
				typ = fullID[0]
				kind = kindsToKind[extensionsKind]
				comp, ok := e.moduleInfoJSON.getComponent(typ, kind)
				if !ok {
					e.telemetry.Logger.Info("service component not found in module info", zap.String("component", extensionString))
					gomod = "unknown"
					version = "unknown"
				} else {
					gomod = comp.Gomod
					version = comp.Version
				}
				status = ""
				if e.healthCheckV2Enabled {
					statusMap := e.getComponentHealthStatus(id, extensionsKind)
					if statusMap != nil {
						status = dataToFlattenedJSONString(statusMap, true)
					}
				}
				components = append(components, serviceComponent{
					ID:              id,
					Name:            name,
					Type:            typ,
					Kind:            kind,
					Gomod:           gomod,
					Version:         version,
					ComponentStatus: status,
				})
			}
		} else {
			if key == pipelinesKind {
				// TODO: ADD PIPELINE COMPONENTS TO ACTIVE COMPONENTS
				e.telemetry.Logger.Info("Skipping pipelines section in service map")
				continue
			}
		}
	}
	return &activeComponentsJSON{Components: components}, nil
}

func dataToFlattenedJSONString(data any, removeNewLines bool) string {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return ""
	}
	res := string(jsonData)
	res = strings.ReplaceAll(res, "\"", "")
	if removeNewLines {
		res = strings.ReplaceAll(res, "\n", "")
	}
	return res
}
