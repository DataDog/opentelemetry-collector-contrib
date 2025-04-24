// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// package componentchecker will define the functions and types necessary to parse component status and config components
package componentchecker // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension/internal/componentchecker"

import (
	"encoding/json"
	"errors"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension/internal/payload"
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

func checkComponentConfiguration(typ string, componentsKind string, configMap map[string]any) (bool, *component.ID) {
	components, ok := configMap[componentsKind]
	if !ok {
		return false, nil
	}
	componentMap, ok := components.(map[string]any)
	if !ok {
		return false, nil
	}
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
	return false, nil
}

func getComponentHealthStatus(id string, componentsKind string, componentStatus map[string]any) map[string]any {
	componentKind, ok := kindsToKind[componentsKind]
	if !ok {
		return nil
	}
	componentsConfig, ok := componentStatus["components"].(map[string]any)
	if !ok {
		return nil
	}
	result := map[string]any{}
	if componentsKind == extensionsKind {
		componentsStatus, _ := componentsConfig[componentsKind].(map[string]any)
		receiversStatus, _ := componentsStatus["components"].(map[string]any)
		componentNameInPipeline := componentKind + ":" + id
		componentStatus, ok := receiversStatus[componentNameInPipeline].(map[string]any)
		if !ok {
			return result
		}
		result = componentStatus
	} else {
		for key, value := range componentsConfig {
			if key == extensionsKind {
				continue
			}
			pipelineMap, _ := value.(map[string]any)
			components, ok := pipelineMap["components"].(map[string]any)
			if !ok {
				continue
			}
			for component, status := range components {
				componentParts := strings.Split(component, ":")
				if len(componentParts) != 2 {
					continue
				}
				kind := componentParts[0]
				fullID := componentParts[1]
				componentStatus, ok := status.(map[string]any)
				if kind != componentKind || id != fullID || !ok {
					continue
				}
				result[key] = map[string]any{
					component: componentStatus,
				}
			}
		}
	}
	return result
}

func getServiceComponent(componentString string, componentsKind string, moduleInfoJSON *payload.ModuleInfoJSON, componentStatus map[string]any, logger *zap.Logger) *payload.ServiceComponent {
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
	statusMap := getComponentHealthStatus(id, componentsKind, componentStatus)
	if len(statusMap) > 0 {
		status = DataToFlattenedJSONString(statusMap, true)
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

// DataToFlattenedJSONString is a helper function to ensure payload strings are
// properly formatted for JSON parsing
func DataToFlattenedJSONString(data any, removeNewLines bool) string {
	var jsonData []byte
	var err error
	if !removeNewLines {
		jsonData, err = json.MarshalIndent(data, "", "  ")
	} else {
		jsonData, err = json.Marshal(data)
	}
	if err != nil {
		return ""
	}

	res := string(jsonData)
	if removeNewLines {
		res = strings.ReplaceAll(res, "\n", "")
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
			enabled, _ := checkComponentConfiguration(comp.String(), field.kinds, collectorConfigStringMap)
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

func PopulateActiveComponents(collectorConfigStringMap map[string]any, moduleInfoJSON *payload.ModuleInfoJSON, componentStatus map[string]any, logger *zap.Logger) (*[]payload.ServiceComponent, error) {
	var serviceComponents []payload.ServiceComponent
	serviceMap, ok := collectorConfigStringMap["service"].(map[string]any)
	if !ok {
		if logger != nil {
			logger.Error("Failed to get service map from collector config, cannot populate active components table")
		}
		return &serviceComponents, errors.New("failed to get service map from collector config, cannot populate active components table")
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
				newServiceComponent := getServiceComponent(extensionString, extensionsKind, moduleInfoJSON, componentStatus, logger)
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
						newServiceComponent := getServiceComponent(componentString, componentsKind, moduleInfoJSON, componentStatus, logger)
						if newServiceComponent != nil {
							serviceComponents = append(serviceComponents, *newServiceComponent)
						}
					}
				}
			}
		}
	}
	return &serviceComponents, nil
}
