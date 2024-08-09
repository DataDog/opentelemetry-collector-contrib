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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	otelcollector "github.com/DataDog/opentelemetry-collector-contrib/e2e-tests/otel-collector"
	"github.com/DataDog/opentelemetry-collector-contrib/e2e-tests/otel-collector/otelparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO: Can be improved. On datadog-agent we use python code to run the e2e tests and pass the docker secret as an env variable
var dockerSecret *string = flag.String("docker_secret", "", "Docker secret to use for the tests")

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
