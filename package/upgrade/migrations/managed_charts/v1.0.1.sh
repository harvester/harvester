#!/bin/bash -ex

CHART_NAME=$1
CHART_MANIFEST=$2

patch_prometheus_resources()
{
    # Add a diff to monitoring managed chart to ignore the CA certificate change (https://github.com/harvester/harvester-installer/pull/274)
  yq e '.spec.diff.comparePatches = [{"apiVersion": "admissionregistration.k8s.io/v1", "kind": "ValidatingWebhookConfiguration", "name": "rancher-monitoring-admission", "operations":[{"op":"remove","path":"/webhooks"}]},{"apiVersion": "admissionregistration.k8s.io/v1", "kind": "MutatingWebhookConfiguration", "name": "rancher-monitoring-admission", "operations":[{"op":"remove","path":"/webhooks"}]}]' $CHART_MANIFEST -i
}

case $CHART_NAME in
  rancher-monitoring)
    patch_prometheus_resources
    ;;
esac
