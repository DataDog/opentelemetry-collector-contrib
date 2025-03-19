// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension/internal/payload"
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

var kindsToKind = map[string]string{
	receiversKind:  receiverKind,
	processorsKind: processorKind,
	exportersKind:  exporterKind,
	extensionsKind: extensionKind,
	connectorsKind: connectorKind,
	providersKind:  providerKind,
	convertersKind: converterKind,
}

type ComponentChecker interface {
	isComponentConfigured(string, string) (bool, *component.ID)
	isModuleAvailable(string, string) bool
	isHealthCheckV2Enabled() (bool, error)
}

type defaultComponentChecker struct {
	extension *fleetAutomationExtension
}

func (d *defaultComponentChecker) isComponentConfigured(typ string, componentsKind string) (bool, *component.ID) {
	return d.extension.isComponentConfigured(typ, componentsKind)
}

func (d *defaultComponentChecker) isModuleAvailable(componentType string, componentKind string) bool {
	return d.extension.isModuleAvailable(componentType, componentKind)
}

func (d *defaultComponentChecker) isHealthCheckV2Enabled() (bool, error) {
	return d.extension.isHealthCheckV2Enabled()
}

// isComponentConfigured checks if a given component is included in the collector config
func (e *fleetAutomationExtension) isComponentConfigured(typ string, componentsKind string) (bool, *component.ID) {
	// NOTE: this will only match the first instance of a component type in the config
	if components, ok := e.collectorConfigStringMap[componentsKind]; ok {
		if componentMap, ok := components.(map[string]any); ok {
			// iterate through componentMap, checking either if key == typ or key split by "/" has keySplit[0] as typ
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

// isModuleAvailable checks if a given gomod type is included in the collector ModuleInfos struct
func (e *fleetAutomationExtension) isModuleAvailable(componentType string, componentKind string) bool {
	for _, field := range []struct {
		kind string
		data map[component.Type]service.ModuleInfo
	}{
		{receiverKind, e.moduleInfo.Receiver},
		{processorKind, e.moduleInfo.Processor},
		{exporterKind, e.moduleInfo.Exporter},
		{extensionKind, e.moduleInfo.Extension},
		{connectorKind, e.moduleInfo.Connector},
		// TODO: add Providers and Converters after upstream change accepted to add these to moduleinfos
	} {
		if componentKind == field.kind {
			if _, ok := field.data[component.MustNewType(componentType)]; ok {
				return true
			}
		}
	}
	return false
}

// isHealthCheckV2Enabled checks if healthcheckv2 is properly configured
// should be ran after e.isComponentConfigured for healthcheckv2 and e.getComponentConfig for healthcheckv2
func (e *fleetAutomationExtension) isHealthCheckV2Enabled() (bool, error) {
	if useV2, ok := e.healthCheckV2Config["use_v2"].(bool); ok && useV2 {
		if httpConfig, ok := e.healthCheckV2Config["http"].(map[string]any); ok {
			if statusConfig, ok := httpConfig["status"].(map[string]any); ok {
				if enabled, ok := statusConfig["enabled"].(bool); ok && enabled {
					return true, nil
				}
				return false, errors.New("healthcheckv2 extension is enabled but http status check is not enabled; component status will not be available")
			}
			return false, errors.New("healthcheckv2 extension is enabled but http status is not configured; component status will not be available")
		}
		return false, errors.New("healthcheckv2 extension is enabled but http endpoint is not configured; component status will not be available")
	}
	return false, errors.New("healthcheckv2 extension is enabled but is set to legacy mode; component status will not be available")
}

// getComponentHealthStatus is subject to change/removal as healthcheckv2 is in "development status"
// unclear if we should wait until this is more stabilized to add individual component status health
// to the payloads we send
//
// requires e.componentStatus to be recently updated with e.getHealthCheckStatus() prior to function call
func (e *fleetAutomationExtension) getComponentHealthStatus(id string, componentsKind string) map[string]any {
	componentKind, ok := kindsToKind[componentsKind]
	if !ok {
		return nil
	}
	result := map[string]any{}
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

// populateFullComponentsJSON creates a ModuleInfoJSON struct with all components from ModuleInfos
func (e *fleetAutomationExtension) populateFullComponentsJSON() *payload.ModuleInfoJSON {
	modInfo := payload.NewModuleInfoJSON()
	for _, field := range []struct {
		kinds string
		data  map[component.Type]service.ModuleInfo
		kind  string
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
			enabled, _ := e.isComponentConfigured(comp.String(), field.kinds)
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

// populateActiveComponentsJSON creates an activeComponentsJSON struct with all active components from the collector service pipelines
func (e *fleetAutomationExtension) populateActiveComponentsJSON() (*payload.ActiveComponentsJSON, error) {
	var serviceComponents []payload.ServiceComponent
	serviceMap, ok := e.collectorConfigStringMap["service"].(map[string]any)
	if !ok {
		e.telemetry.Logger.Error("Failed to get service map from collector config, cannot populate active components table")
		return &payload.ActiveComponentsJSON{
			Components: serviceComponents,
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
				newServiceComponent := e.getServiceComponent(extensionString, extensionsKind)
				if newServiceComponent != nil {
					serviceComponents = append(serviceComponents, *newServiceComponent)
				}
			}
		} else if key == pipelinesKind {
			pipelineMap, ok := value.(map[string]any) // e.g. "traces", "logs", "metrics"
			if !ok {
				e.telemetry.Logger.Info("Failed to get pipeline map from service map config")
				continue
			}
			for _, components := range pipelineMap {
				// pipelineTelemetryKind will be "traces", "logs", etc.
				// components will be a list of map[string]any with component kinds and ids mapped
				componentKindsInPipeline, ok := components.(map[string]any)
				if !ok {
					e.telemetry.Logger.Info("Failed to get components map from pipeline map in service map config")
					continue
				}
				for componentsKind, componentsInterface := range componentKindsInPipeline {
					componentsList, ok := componentsInterface.([]any)
					if !ok {
						e.telemetry.Logger.Info("Failed to get components list from pipeline map in service map config")
						continue
					}
					for _, component := range componentsList {
						componentString, ok := component.(string)
						if !ok {
							e.telemetry.Logger.Info("Components list in pipeline map in service map config contains non-string value", zap.Any("component", component))
							continue
						}
						newServiceComponent := e.getServiceComponent(componentString, componentsKind)
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

// TODO: Work to support instancing of components (non-trivial problem)
// https://github.com/open-telemetry/opentelemetry-collector/issues/10534#issue-2389523504
func (e *fleetAutomationExtension) getServiceComponent(componentString, componentsKind string) *payload.ServiceComponent {
	var id, name, typ, kind, gomod, version, status string
	componentKind, ok := kindsToKind[componentsKind]
	if !ok {
		e.telemetry.Logger.Info("Invalid component kind", zap.String("kind", componentsKind))
		return nil
	}
	fullID := strings.Split(componentString, "/")
	switch len(fullID) {
	case 1:
		// component does not have a name
		id = fullID[0]
		name = ""
	case 2:
		id = componentString
		name = fullID[1]
	default:
		e.telemetry.Logger.Info("Invalid component ID", zap.String("component", componentString))
		return nil
	}
	typ = fullID[0]
	kind = componentKind
	comp, ok := e.ModuleInfoJSON.GetComponent(typ, kind)
	if !ok {
		e.telemetry.Logger.Info("service component not found in module info", zap.String("component", componentString))
		gomod = "unknown"
		version = "unknown"
	} else {
		gomod = comp.Gomod
		version = comp.Version
	}
	status = ""
	if e.healthCheckV2Enabled {
		statusMap := e.getComponentHealthStatus(id, componentsKind)
		if len(statusMap) > 0 {
			status = dataToFlattenedJSONString(statusMap, true, true)
		}
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

func dataToFlattenedJSONString(data any, removeNewLines bool, removeQuotes bool) string {
	var jsonData []byte
	var err error
	if removeNewLines && removeQuotes {
		// no sense adding all the extra spaces if we are removing newlines and quotes
		jsonData, err = json.Marshal(data)
	} else {
		jsonData, err = json.MarshalIndent(data, "", "  ")
	}
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
