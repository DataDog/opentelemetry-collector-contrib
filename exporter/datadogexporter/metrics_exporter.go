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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

type metricsExporter struct {
	logger *zap.Logger
	cfg    *Config
	client *datadog.Client
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				// Disable RFC 6555 Fast Fallback ("Happy Eyeballs")
				FallbackDelay: -1 * time.Nanosecond,
			}).DialContext,
			MaxIdleConns: 100,
			// Not supported by intake
			ForceAttemptHTTP2: false,
		},
	}
}

func newMetricsExporter(logger *zap.Logger, cfg *Config) (*metricsExporter, error) {
	client := datadog.NewClient(cfg.API.Key, "")
	client.ExtraHeader["User-Agent"] = userAgent
	client.SetBaseUrl(cfg.Metrics.TCPAddr.Endpoint)
	client.HttpClient = newHTTPClient()

	return &metricsExporter{logger, cfg, client}, nil
}

// pushHostMetadata sends a host metadata payload to the "/intake" endpoint
func (exp *metricsExporter) pushHostMetadata(metadata hostMetadata) error {
	path := exp.cfg.Metrics.TCPAddr.Endpoint + "/intake"
	buf, _ := json.Marshal(metadata)
	req, _ := http.NewRequest(http.MethodPost, path, bytes.NewBuffer(buf))
	req.Header.Set("DD-API-KEY", exp.cfg.API.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	client := newHTTPClient()
	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf(
			"'%d - %s' error when sending metadata payload to %s",
			resp.StatusCode,
			resp.Status,
			path,
		)
	}

	return nil
}

func (exp *metricsExporter) processMetrics(metrics []datadog.Metric) {
	addNamespace := exp.cfg.Metrics.Namespace != ""
	overrideHostname := exp.cfg.Hostname != ""

	for i := range metrics {
		if addNamespace {
			newName := exp.cfg.Metrics.Namespace + *metrics[i].Metric
			metrics[i].Metric = &newName
		}

		if overrideHostname || metrics[i].GetHost() == "" {
			metrics[i].Host = GetHost(exp.cfg)
		}
	}
}

func (exp *metricsExporter) PushMetricsData(ctx context.Context, md pdata.Metrics) (int, error) {
	metrics, droppedTimeSeries := MapMetrics(exp.logger, exp.cfg.Metrics, md)
	exp.processMetrics(metrics)

	err := exp.client.PostMetrics(metrics)
	return droppedTimeSeries, err
}
