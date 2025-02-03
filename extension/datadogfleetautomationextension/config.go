// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"errors"

	"go.opentelemetry.io/collector/component"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
)

var (
	errMissingSite = errors.New("\"site\" cannot be blank when using the \"datadogfleetautomation\" extension")
	errMissingKey  = errors.New("\"key\" cannot be blank when using the \"datadogfleetautomation\" extension")
)

var _ component.Config = (*Config)(nil)

const (
	// DefaultSite is the default site for the Datadog API.
	DefaultSite = datadogconfig.DefaultSite
)

// Config contains the information necessary for enabling the Datadog Fleet
// Automation Extension.
type Config struct {
	API APIConfig `mapstructure:"api"`
}

// APIConfig contains the information necessary for configuring the Datadog API.
type APIConfig = datadogconfig.APIConfig

// Validate ensures that the configuration is valid.
func (c *Config) Validate() error {
	if c.API.Site == "" {
		return errMissingSite
	}
	if c.API.Key == "" {
		return errMissingKey
	}
	return nil
}
