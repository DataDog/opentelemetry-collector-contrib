// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metadata defines the metadata for the Datadog Fleet Automation extension component.
package metadata

import (
	"go.opentelemetry.io/collector/component"
)

var (
	// Type is the component type of the Datadog Fleet Automation extension.
	Type = component.MustNewType("datadogfleetautomation")
	// ScopeName is the long form scope name of the Datadog Fleet Automation extension.
	ScopeName = "github.com/DataDog/datadog-agent/comp/otelcol/fleetautomationextension"
)

const (
	// ExtensionStability is the stability level of the OpenTelemetry Extension.
	// As it is set to Development, the extension is not recommended for production use.
	ExtensionStability = component.StabilityLevelDevelopment
)
