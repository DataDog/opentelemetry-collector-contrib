// Code generated by mdatagen. DO NOT EDIT.

package metadata

import "go.opentelemetry.io/collector/confmap"

// MetricConfig provides common config for a particular metric.
type MetricConfig struct {
	Enabled bool `mapstructure:"enabled"`

	enabledSetByUser bool
}

func (ms *MetricConfig) Unmarshal(parser *confmap.Conf) error {
	if parser == nil {
		return nil
	}
	err := parser.Unmarshal(ms, confmap.WithErrorUnused())
	if err != nil {
		return err
	}
	ms.enabledSetByUser = parser.IsSet("enabled")
	return nil
}

// MetricsConfig provides config for apache metrics.
type MetricsConfig struct {
	ApacheCPULoad            MetricConfig `mapstructure:"apache.cpu.load"`
	ApacheCPUTime            MetricConfig `mapstructure:"apache.cpu.time"`
	ApacheCurrentConnections MetricConfig `mapstructure:"apache.current_connections"`
	ApacheLoad1              MetricConfig `mapstructure:"apache.load.1"`
	ApacheLoad15             MetricConfig `mapstructure:"apache.load.15"`
	ApacheLoad5              MetricConfig `mapstructure:"apache.load.5"`
	ApacheRequestTime        MetricConfig `mapstructure:"apache.request.time"`
	ApacheRequests           MetricConfig `mapstructure:"apache.requests"`
	ApacheScoreboard         MetricConfig `mapstructure:"apache.scoreboard"`
	ApacheTraffic            MetricConfig `mapstructure:"apache.traffic"`
	ApacheUptime             MetricConfig `mapstructure:"apache.uptime"`
	ApacheWorkers            MetricConfig `mapstructure:"apache.workers"`
}

func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		ApacheCPULoad: MetricConfig{
			Enabled: true,
		},
		ApacheCPUTime: MetricConfig{
			Enabled: true,
		},
		ApacheCurrentConnections: MetricConfig{
			Enabled: true,
		},
		ApacheLoad1: MetricConfig{
			Enabled: true,
		},
		ApacheLoad15: MetricConfig{
			Enabled: true,
		},
		ApacheLoad5: MetricConfig{
			Enabled: true,
		},
		ApacheRequestTime: MetricConfig{
			Enabled: true,
		},
		ApacheRequests: MetricConfig{
			Enabled: true,
		},
		ApacheScoreboard: MetricConfig{
			Enabled: true,
		},
		ApacheTraffic: MetricConfig{
			Enabled: true,
		},
		ApacheUptime: MetricConfig{
			Enabled: true,
		},
		ApacheWorkers: MetricConfig{
			Enabled: true,
		},
	}
}

// ResourceAttributeConfig provides common config for a particular resource attribute.
type ResourceAttributeConfig struct {
	Enabled bool `mapstructure:"enabled"`

	enabledSetByUser bool
}

func (rac *ResourceAttributeConfig) Unmarshal(parser *confmap.Conf) error {
	if parser == nil {
		return nil
	}
	err := parser.Unmarshal(rac, confmap.WithErrorUnused())
	if err != nil {
		return err
	}
	rac.enabledSetByUser = parser.IsSet("enabled")
	return nil
}

// ResourceAttributesConfig provides config for apache resource attributes.
type ResourceAttributesConfig struct {
	ApacheServerName ResourceAttributeConfig `mapstructure:"apache.server.name"`
	ApacheServerPort ResourceAttributeConfig `mapstructure:"apache.server.port"`
}

func DefaultResourceAttributesConfig() ResourceAttributesConfig {
	return ResourceAttributesConfig{
		ApacheServerName: ResourceAttributeConfig{
			Enabled: true,
		},
		ApacheServerPort: ResourceAttributeConfig{
			Enabled: true,
		},
	}
}

// MetricsBuilderConfig is a configuration for apache metrics builder.
type MetricsBuilderConfig struct {
	Metrics            MetricsConfig            `mapstructure:"metrics"`
	ResourceAttributes ResourceAttributesConfig `mapstructure:"resource_attributes"`
}

func DefaultMetricsBuilderConfig() MetricsBuilderConfig {
	return MetricsBuilderConfig{
		Metrics:            DefaultMetricsConfig(),
		ResourceAttributes: DefaultResourceAttributesConfig(),
	}
}
