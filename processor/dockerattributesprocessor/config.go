// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package dockerattributesprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor"

import (
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/confmap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/docker"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"
)

// The only association source and name supported in v1.
const (
	associationFromResourceAttribute = "resource_attribute"
	associationNameContainerID       = "container.id"
)

// Config is the configuration for the docker attributes processor.
type Config struct {
	// docker.Config provides Endpoint, Timeout, ExcludedImages, DockerAPIVersion,
	// squashed to the top level.
	docker.Config `mapstructure:",squash"`

	// Extract selects which metadata, labels, and env vars are enriched.
	Extract ExtractConfig `mapstructure:"extract"`

	// Association maps telemetry to a container. v1 supports only
	// {from: resource_attribute, name: container.id}; empty defaults to that.
	Association []AssociationConfig `mapstructure:"association"`

	// DeleteGracePeriod is how long an exited container's metadata is retained so
	// late-arriving telemetry can still be enriched. Default 120s.
	DeleteGracePeriod time.Duration `mapstructure:"delete_grace_period"`
}

// ExtractConfig selects which container labels and env vars are extracted. The
// fixed container.* metadata is always emitted and is not configurable.
type ExtractConfig struct {
	// Labels selects container labels to emit.
	Labels []FieldExtractConfig `mapstructure:"labels"`
	// EnvVars selects container env vars to emit (opt-in per key).
	EnvVars []FieldExtractConfig `mapstructure:"env_vars"`
}

// FieldExtractConfig is a single label/env extraction rule. It is defined in the
// watcher package (which consumes it) and aliased here so the public config can
// name it without an import cycle or a duplicate struct.
type FieldExtractConfig = watcher.FieldExtractConfig

// AssociationConfig is a single container-association rule.
type AssociationConfig struct {
	From string `mapstructure:"from"`
	Name string `mapstructure:"name"`
}

// Unmarshal decodes the whole Config. Without it, the Unmarshal promoted from the
// embedded docker.Config would run instead and drop this processor's own fields.
func (cfg *Config) Unmarshal(conf *confmap.Conf) error {
	return conf.Unmarshal(cfg)
}

// Validate checks the configuration is well-formed.
func (cfg *Config) Validate() error {
	if err := cfg.Config.Validate(); err != nil {
		return err
	}

	if cfg.DeleteGracePeriod < 0 {
		return errors.New("delete_grace_period must not be negative")
	}

	for _, f := range cfg.Extract.Labels {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("invalid extract.labels rule: %w", err)
		}
	}
	for _, f := range cfg.Extract.EnvVars {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("invalid extract.env_vars rule: %w", err)
		}
	}

	for _, a := range cfg.Association {
		if err := a.validate(); err != nil {
			return err
		}
	}

	return nil
}

func (a AssociationConfig) validate() error {
	if a.From != associationFromResourceAttribute {
		return fmt.Errorf(
			"association from %q is not supported in v1: only %q is supported",
			a.From, associationFromResourceAttribute,
		)
	}
	if a.Name != associationNameContainerID {
		return fmt.Errorf(
			"association name %q is not supported in v1: only %q is supported",
			a.Name, associationNameContainerID,
		)
	}
	return nil
}
