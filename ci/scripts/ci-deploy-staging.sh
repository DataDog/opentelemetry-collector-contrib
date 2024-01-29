#!/usr/bin/env bash

# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0

# This script is used to deploy collector on demo account cluster

set -euo pipefail
IFS=$'\n\t'
set -x
namespace=$NAMESPACE
nodegroup=$NODE_GROUP
mode=$MODE
replicaCount=$REPLICA_COUNT
clusterRole=$CLUSTER_ROLE
clusterName=$CLUSTER_NAME
clusterArn=$CLUSTER_ARN

install_collector() {
	release_name="opentelemetry-collector"

	# Add open-telemetry helm repo (if repo already exists, helm 3+ will skip)
	helm --debug repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
	helm repo update open-telemetry

	# deploy collector via helm
	helm --debug upgrade "${release_name}" -n "${namespace}" open-telemetry/opentelemetry-collector --install \
		-f ./ci/values.yaml \
		--set-string image.tag="otelcolcontrib-v$CI_COMMIT_SHORT_SHA" \
		--set-string image.repository="601427279990.dkr.ecr.us-east-1.amazonaws.com/otel-collector-contrib" \
		# --set nodeSelector.alpha\\.eksctl\\.io/nodegroup-name="${nodegroup}" \
		# --set mode="${mode}" \
		# --set replicaCount="${replicaCount}" \
		--set clusterRole.name="${clusterRole}" \
		--set clusterRole.clusterRoleBinding.name="${clusterRole}"

	# only deploy otlp col for otel-ds-gateway
	if [ "$namespace" == "otel-ds-gateway" ]; then
		install_ds_otlp
	fi
}

install_ds_otlp() {
	release_name="opentelemetry-collector-ds"

	# daemonset with otlp exporter
	helm --debug upgrade "${release_name}" -n "${namespace}" open-telemetry/opentelemetry-collector --install \
		-f ./ci/values.yaml \
		-f ./ci/values-otlp-col.yaml \
		--set-string image.tag="otelcolcontrib-v$CI_COMMIT_SHORT_SHA" \
		--set-string image.repository="601427279990.dkr.ecr.us-east-1.amazonaws.com/otel-collector-contrib"
}


install_deployment() {
	release_name_deployment="opentelemetry-collector-deployment"

	# deploy collector that fetches jmx metrics via helm. The jmx receiver cannot be used in the daemonset deployment
	# as this would lead to duplicate metrics.
	helm --debug upgrade "${release_name_deployment}" -n "${namespace}" open-telemetry/opentelemetry-collector --install \
		-f ./ci/values-jmx.yaml \
		--set-string image.tag="otelcolcontrib-v$CI_COMMIT_SHORT_SHA" \
		--set-string image.repository="601427279990.dkr.ecr.us-east-1.amazonaws.com/otel-collector-contrib" \
		--set nodeSelector.alpha\\.eksctl\\.io/nodegroup-name="${nodegroup}"

}

###########################################################################################################
clusterName="${clusterName}"
clusterArn="${clusterArn}"

aws eks --region us-east-1 update-kubeconfig --name "${clusterName}"
kubectl config use-context "${clusterArn}"

if [ "$namespace" == "otel" ]; then
	install_collector
fi

