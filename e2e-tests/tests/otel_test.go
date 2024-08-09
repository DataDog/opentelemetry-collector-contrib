// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package e2etests

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	otelcollector "github.com/DataDog/opentelemetry-collector-contrib/e2e-tests/otel-collector"
	"github.com/DataDog/opentelemetry-collector-contrib/e2e-tests/otel-collector/otelparams"
)

// TODO: Can be improved. On datadog-agent we use python code to run the e2e tests and pass the docker secret as an env variable
var dockerSecret = flag.String("docker_secret", "", "Docker secret to use for the tests")

//go:embed config.yaml
var otelConfig string

type otelSuite struct {
	e2e.BaseSuite[otelcollector.Kubernetes]
}

// TestVMSuite runs tests for the VM interface to ensure its implementation is correct.
func TestVMSuite(t *testing.T) {
	extraParams := runner.ConfigMap{}
	extraParams.Set("ddagent:imagePullRegistry", "669783387624.dkr.ecr.us-east-1.amazonaws.com", false)
	extraParams.Set("ddagent:imagePullPassword", *dockerSecret, true)
	extraParams.Set("ddagent:imagePullUsername", "AWS", false)

	otelOptions := []otelparams.Option{otelparams.WithOTelConfig(otelConfig)}
	// Use image built in the CI
	pipelineID, ok1 := os.LookupEnv("E2E_PIPELINE_ID")
	commitSHA, ok2 := os.LookupEnv("E2E_COMMIT_SHORT_SHA")
	if ok1 && ok2 {
		values := fmt.Sprintf(`
image:
  tag: %s-%s
`, pipelineID, commitSHA)
		otelOptions = append(otelOptions, otelparams.WithHelmValues(values))
	}

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(otelcollector.Provisioner(otelcollector.WithOTelOptions(otelOptions...), otelcollector.WithExtraParams(extraParams)))}

	e2e.Run(t, &otelSuite{}, suiteParams...)
}

// TODO write a test that actually test something
func (v *otelSuite) TestExecute() {
	res, _ := v.Env().KubernetesCluster.Client().CoreV1().Pods("default").List(context.TODO(), v1.ListOptions{})
	for _, pod := range res.Items {
		v.T().Logf("Pod: %s", pod.Name)
	}
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		metricsName, err := v.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(t, err)
		fmt.Printf("metriiiics: %v", metricsName)
		logs, err := v.Env().FakeIntake.Client().FilterLogs("")
		for _, l := range logs {
			fmt.Printf("logs	: %v", l.Tags)
		}

		assert.NoError(t, err)
		assert.NotEmpty(t, metricsName)
		assert.NotEmpty(t, logs)

	}, 1*time.Minute, 10*time.Second)
}

func (s *otelSuite) TestOTLPTraces() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	service := "telemetrygen-job"
	numTraces := 10

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "traces", []string{"--service", service, "--traces", fmt.Sprint(numTraces)})

	s.T().Log("Waiting for traces")
	s.EventuallyWithT(func(c *assert.CollectT) {
		traces, err := s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err)
		assert.NotEmpty(c, traces)
		trace := traces[0]
		assert.Equal(c, "none", trace.Env)
		assert.NotEmpty(c, trace.TracerPayloads)
		tp := trace.TracerPayloads[0]
		assert.NotEmpty(c, tp.Chunks)
		assert.NotEmpty(c, tp.Chunks[0].Spans)
		spans := tp.Chunks[0].Spans
		for _, sp := range spans {
			assert.Equal(c, service, sp.Service)
			assert.Equal(c, "telemetrygen", sp.Meta["otel.library.name"])
		}
	}, 2*time.Minute, 10*time.Second)
}

func (s *otelSuite) TestOTLPMetrics() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	service := "telemetrygen-job"
	serviceAttribute := fmt.Sprintf("service.name=\"%v\"", service)
	numMetrics := 10

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "metrics", []string{"--metrics", fmt.Sprint(numMetrics), "--otlp-attributes", serviceAttribute})

	s.T().Log("Waiting for metrics")
	s.EventuallyWithT(func(c *assert.CollectT) {
		serviceTag := "service:" + service
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("gen", fakeintake.WithTags[*aggregator.MetricSeries]([]string{serviceTag}))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 2*time.Minute, 10*time.Second)
}

func (s *otelSuite) TestOTLPLogs() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	service := "telemetrygen-job"
	serviceAttribute := fmt.Sprintf("service.name=\"%v\"", service)
	numLogs := 10
	logBody := "telemetrygen log"

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "logs", []string{"--logs", fmt.Sprint(numLogs), "--otlp-attributes", serviceAttribute, "--body", logBody})

	s.T().Log("Waiting for logs")
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(service)
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
		for _, log := range logs {
			assert.Contains(c, log.Message, logBody)
		}
	}, 2*time.Minute, 10*time.Second)
}

func (s *otelSuite) createTelemetrygenJob(ctx context.Context, telemetry string, options []string) {
	var ttlSecondsAfterFinished int32 = 0 //nolint:revive // We want to see this is explicitly set to 0
	var backOffLimit int32 = 4

	otlpEndpoint := fmt.Sprintf("%v:4317", s.Env().OTelCollector.LabelSelectors["app.kubernetes.io/name"])
	s.T().Log("otlpEndpoint", otlpEndpoint)
	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("telemetrygen-job-%v", telemetry),
			Namespace: "default",
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "telemetrygen-job",
							Image:   "ghcr.io/open-telemetry/opentelemetry-collector-contrib/telemetrygen:latest",
							Command: append([]string{"/telemetrygen", telemetry, "--otlp-endpoint", otlpEndpoint, "--otlp-insecure"}, options...),
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err := s.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(ctx, jobSpec, metav1.CreateOptions{})
	assert.NoError(s.T(), err, "Could not properly start job")
}
