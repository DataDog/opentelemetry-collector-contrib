// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package componentchecker

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/payload"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
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

var kindsToKind = map[string]string{
	receiversKind:  receiverKind,
	processorsKind: processorKind,
	exportersKind:  exporterKind,
	extensionsKind: extensionKind,
	connectorsKind: connectorKind,
	providersKind:  providerKind,
	convertersKind: converterKind,
}

// CheckComponentConfiguration checks if a given component is included in the collector config
func CheckComponentConfiguration(typ string, componentsKind string, configMap map[string]any) (bool, *component.ID) {
	if components, ok := configMap[componentsKind]; ok {
		if componentMap, ok := components.(map[string]any); ok {
			for key := range componentMap {
				if key == typ {
					newID := component.MustNewID(typ)
					return true, &newID
				}
				keySplit := strings.Split(key, "/")
				if len(keySplit) == 2 && keySplit[0] == typ {
					newID := component.MustNewIDWithName(keySplit[0], keySplit[1])
					return true, &newID
				}
			}
		}
	}
	return false, nil
}

// GetComponentHealthStatus retrieves the health status for a component
func GetComponentHealthStatus(id string, componentsKind string, componentStatus map[string]any) map[string]any {
	componentKind, ok := kindsToKind[componentsKind]
	if !ok {
		return nil
	}
	result := map[string]any{}
	if componentsConfig, ok := componentStatus["components"].(map[string]any); ok {
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

// GetServiceComponent creates a ServiceComponent from component information
func GetServiceComponent(componentString string, componentsKind string, moduleInfoJSON *payload.ModuleInfoJSON, componentStatus map[string]any, logger *zap.Logger) *payload.ServiceComponent {
	var id, name, typ, kind, gomod, version, status string
	componentKind, ok := kindsToKind[componentsKind]
	if !ok {
		if logger != nil {
			logger.Info("Invalid component kind", zap.String("kind", componentsKind))
		}
		return nil
	}
	fullID := strings.Split(componentString, "/")
	switch len(fullID) {
	case 1:
		id = fullID[0]
		name = ""
	case 2:
		id = componentString
		name = fullID[1]
	default:
		if logger != nil {
			logger.Info("Invalid component ID", zap.String("component", componentString))
		}
		return nil
	}
	typ = fullID[0]
	kind = componentKind
	comp, ok := moduleInfoJSON.GetComponent(typ, kind)
	if !ok {
		if logger != nil {
			logger.Info("service component not found in module info", zap.String("component", componentString))
		}
		gomod = "unknown"
		version = "unknown"
	} else {
		gomod = comp.Gomod
		version = comp.Version
	}
	status = ""
	statusMap := GetComponentHealthStatus(id, componentsKind, componentStatus)
	if len(statusMap) > 0 {
		status = DataToFlattenedJSONString(statusMap, true, false)
	}
	return &payload.ServiceComponent{
		ID:              id,
		Name:            name,
		Type:            typ,
		Kind:            kind,
		Gomod:           gomod,
		Version:         version,
		ComponentStatus: status,
	}
}

// UpdateComponentStatus updates the status of a component based on an event
func UpdateComponentStatus(source interface {
	Kind() component.Kind
	ComponentID() component.ID
}, event interface {
	Status() componentstatus.Status
	Err() error
}) map[string]any {
	componentKey := fmt.Sprintf("%s:%s", strings.ToLower(source.Kind().String()), source.ComponentID())
	return map[string]any{
		componentKey: map[string]any{
			"status":    event.Status().String(),
			"error":     event.Err(),
			"timestamp": time.Now(),
		},
	}
}

func DataToFlattenedJSONString(data any, removeNewLines bool, removeQuotes bool) string {
	indent := ""
	if !removeNewLines {
		indent = "  "
	}

	jsonData, err := json.MarshalIndent(data, "", indent)
	if err != nil {
		return ""
	}

	res := string(jsonData)
	if removeNewLines {
		res = strings.ReplaceAll(res, "\n", "")
	}
	if removeQuotes {
		res = strings.ReplaceAll(res, "\"", "")
	}
	return res
}

// PopulateFullComponentsJSON creates a ModuleInfoJSON struct with all components from ModuleInfos
func PopulateFullComponentsJSON(moduleInfo service.ModuleInfos, collectorConfigStringMap map[string]any) *payload.ModuleInfoJSON {
	modInfo := payload.NewModuleInfoJSON()
	for _, field := range []struct {
		kinds string
		data  map[component.Type]service.ModuleInfo
		kind  string
	}{
		{receiversKind, moduleInfo.Receiver, receiverKind},
		{processorsKind, moduleInfo.Processor, processorKind},
		{exportersKind, moduleInfo.Exporter, exporterKind},
		{extensionsKind, moduleInfo.Extension, extensionKind},
		{connectorsKind, moduleInfo.Connector, connectorKind},
		// TODO: add Providers and Converters after upstream change accepted to add these to moduleinfos
	} {
		for comp, builderRef := range field.data {
			parts := strings.Split(builderRef.BuilderRef, " ")
			if len(parts) != 2 {
				continue
			}
			enabled, _ := CheckComponentConfiguration(comp.String(), field.kinds, collectorConfigStringMap)
			modInfo.AddComponent(payload.CollectorModule{
				Type:       comp.String(),
				Kind:       field.kind,
				Gomod:      parts[0],
				Version:    parts[1],
				Configured: enabled,
			})
		}
	}
	return modInfo
}

// PopulateActiveComponentsJSON creates an activeComponentsJSON struct with all active components from the collector service pipelines
func PopulateActiveComponentsJSON(collectorConfigStringMap map[string]any, moduleInfoJSON *payload.ModuleInfoJSON, componentStatus map[string]any, logger *zap.Logger) (*payload.ActiveComponentsJSON, error) {
	var serviceComponents []payload.ServiceComponent
	serviceMap, ok := collectorConfigStringMap["service"].(map[string]any)
	if !ok {
		if logger != nil {
			logger.Error("Failed to get service map from collector config, cannot populate active components table")
		}
		return &payload.ActiveComponentsJSON{
			Components: serviceComponents,
		}, errors.New("failed to get service map from collector config, cannot populate active components table")
	}
	for key, value := range serviceMap {
		if key == extensionsKind {
			extensionsList, ok := value.([]any)
			if !ok {
				if logger != nil {
					logger.Info("Failed to get extensions list from service map")
				}
				continue
			}
			for _, extension := range extensionsList {
				extensionString, ok := extension.(string)
				if !ok {
					if logger != nil {
						logger.Info("Extensions list in service map config contains non-string value", zap.Any("extension", extension))
					}
					continue
				}
				newServiceComponent := GetServiceComponent(extensionString, extensionsKind, moduleInfoJSON, componentStatus, logger)
				if newServiceComponent != nil {
					serviceComponents = append(serviceComponents, *newServiceComponent)
				}
			}
		} else if key == pipelinesKind {
			pipelineMap, ok := value.(map[string]any) // e.g. "traces", "logs", "metrics"
			if !ok {
				if logger != nil {
					logger.Info("Failed to get pipeline map from service map config")
				}
				continue
			}
			for _, components := range pipelineMap {
				// pipelineTelemetryKind will be "traces", "logs", etc.
				// components will be a list of map[string]any with component kinds and ids mapped
				componentKindsInPipeline, ok := components.(map[string]any)
				if !ok {
					if logger != nil {
						logger.Info("Failed to get components map from pipeline map in service map config")
					}
					continue
				}
				for componentsKind, componentsInterface := range componentKindsInPipeline {
					componentsList, ok := componentsInterface.([]any)
					if !ok {
						if logger != nil {
							logger.Info("Failed to get components list from pipeline map in service map config")
						}
						continue
					}
					for _, component := range componentsList {
						componentString, ok := component.(string)
						if !ok {
							if logger != nil {
								logger.Info("Components list in pipeline map in service map config contains non-string value", zap.Any("component", component))
							}
							continue
						}
						newServiceComponent := GetServiceComponent(componentString, componentsKind, moduleInfoJSON, componentStatus, logger)
						if newServiceComponent != nil {
							serviceComponents = append(serviceComponents, *newServiceComponent)
						}
					}
				}
			}
		}
	}
	return &payload.ActiveComponentsJSON{Components: serviceComponents}, nil
}
