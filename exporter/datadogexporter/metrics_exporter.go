// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datadogexporter

import (
	"context"

	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/config"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/metadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/utils"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/version"
)

type metricsExporter struct {
	logger *zap.Logger
	cfg    *config.Config
	client *datadog.Client
}

func newMetricsExporter(logger *zap.Logger, cfg *config.Config) (*metricsExporter, error) {
	client := datadog.NewClient(cfg.API.Key, "")
	client.ExtraHeader["User-Agent"] = version.UserAgent
	client.SetBaseUrl(cfg.Metrics.TCPAddr.Endpoint)
	client.HttpClient = utils.NewHTTPClient()

	return &metricsExporter{logger, cfg, client}, nil
}

func (exp *metricsExporter) processMetrics(metrics []datadog.Metric) {
	addNamespace := exp.cfg.Metrics.Namespace != ""
	overrideHostname := exp.cfg.Hostname != ""

	for i := range metrics {
		if addNamespace {
			newName := exp.cfg.Metrics.Namespace + *metrics[i].Metric
			metrics[i].Metric = &newName
		}

		if overrideHostname {
			metrics[i].Host = &exp.cfg.TagsConfig.Hostname
		} else if metrics[i].GetHost() == "" {
			metrics[i].Host = metadata.GetHost(exp.logger, exp.cfg)
		}
	}
}

func (exp *metricsExporter) PushMetricsData(ctx context.Context, md pdata.Metrics) (int, error) {
	metrics, droppedTimeSeries := MapMetrics(exp.logger, exp.cfg.Metrics, md)
	exp.processMetrics(metrics)

	err := exp.client.PostMetrics(metrics)
	return droppedTimeSeries, err
}
