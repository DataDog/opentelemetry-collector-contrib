#!/usr/bin/env bash

# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0

# This script is used to deploy collector on demo account cluster

set -euo pipefail
IFS=$'\n\t'
set -x

install_collector() {
	release_name="opentelemetry-collector"

	# if repo already exists, helm 3+ will skip
	helm --debug repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
	helm repo update open-telemetry
	# --install will run `helm install` if not already present.
	helm --debug upgrade "${release_name}" -n "kafka-poc" open-telemetry/opentelemetry-collector --install \
		-f ./ci/values.yaml \
		-f "./ci/values-kafka-poc.yaml" \
		--set-string image.tag="otelcolcontrib-v$CI_COMMIT_SHORT_SHA" \
		--set-string image.repository="601427279990.dkr.ecr.us-east-1.amazonaws.com/otel-collector-contrib"
	helm list --all-namespaces

	install_deployment
}

install_deployment() {
	release_name_deployment="opentelemetry-collector-deployment"

	# --install collector that fetches jmx metrics. The jmx receiver cannot be used in the daemonset deployment
	# as this would lead to duplicate metrics.
	helm --debug upgrade "${release_name_deployment}" -n "kafka-poc" open-telemetry/opentelemetry-collector --install \
		-f ./ci/values-jmx.yaml \
		--set-string image.tag="otelcolcontrib-v$CI_COMMIT_SHORT_SHA" \
		--set-string image.repository="601427279990.dkr.ecr.us-east-1.amazonaws.com/otel-collector-contrib" \
		--set nodeSelector.alpha\\.eksctl\\.io/nodegroup-name=kafka-poc-ng

}

###########################################################################################################
clusterName="dd-otel"
clusterArn="arn:aws:eks:us-east-1:601427279990:cluster/${clusterName}"

aws eks --region us-east-1 update-kubeconfig --name "${clusterName}"
kubectl config use-context "${clusterArn}"

install_collector
