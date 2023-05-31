#!/usr/bin/env bash

# @ckelner: this script closely replicates deploy-prod-us-11287-eks.sh but
# removes some dependencies like aws-vault in lieu of automation that can be
# leveraged via GitLab

# http://redsymbol.net/articles/unofficial-bash-strict-mode/
set -euo pipefail
IFS=$'\n\t'
set -x


install_collector() {

  # Set the namespace and release name
  NAMESPACE="default"
  RELEASE_NAME="otel-collector-deploy"

  # Get the helm list and filter for the release name
  helm_output=$(helm list -n $NAMESPACE | grep $RELEASE_NAME)

  # Check if the helm_output variable is empty
  if [[ -z "$helm_output" ]]; then
    echo "The release $RELEASE_NAME is not installed in the namespace $NAMESPACE."
    helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
    helm install $RELEASE_NAME -f ./ci/values.yaml --set-string image.tag="otelcolcontrib-$CI_COMMIT_SHORT_SHA"
  else
    echo "The release $RELEASE_NAME is installed in the namespace $NAMESPACE."
    echo "$helm_output"
    helm upgrade $RELEASE_NAME -f ./ci/values.yaml --set-string image.tag="otelcolcontrib-$CI_COMMIT_SHORT_SHA"
  fi

}

###########################################################################################################
clusterName="otel-demo"
clusterArn="arn:aws:eks:us-east-1:172597598159:cluster/${clusterName}"

aws sts get-caller-identity
aws eks --region us-east-1 update-kubeconfig --name "${clusterName}"
aws sts get-caller-identity
kubectl config use-context "${clusterArn}"

install_collector
