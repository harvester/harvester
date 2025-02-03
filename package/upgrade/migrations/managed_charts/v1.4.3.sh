#!/bin/bash -ex

CHART_NAME=$1
CHART_MANIFEST=$2

patch_snapshot_validation_webhook_tls()
{
  local manifest=$1

  result=$(yq '.spec.diff.comparePatches[] |
                 select(
                   .apiVersion == "v1" and
                   .kind == "Secret" and
                   .name == "snapshot-validation-webhook-tls")' $manifest 2>/dev/null)

  if [ -z "$result" ]; then
    yq e '.spec.diff.comparePatches += [{
      "apiVersion": "v1",
      "kind": "Secret",
      "name": "snapshot-validation-webhook-tls",
      "jsonPointers":["/data"]}]' $manifest -i
  fi
}

patch_harvester_snapshot_validation_webhook()
{
  local manifest=$1

  result=$(yq '.spec.diff.comparePatches[] |
                 select(
                   .apiVersion == "admissionregistration.k8s.io/v1" and
                   .kind == "ValidatingWebhookConfiguration" and
                   .name == "harvester-snapshot-validation-webhook")' $manifest 2>/dev/null)

  if [ -z "$result" ]; then
    yq e '.spec.diff.comparePatches += [{
      "apiVersion": "admissionregistration.k8s.io/v1",
      "kind": "ValidatingWebhookConfiguration",
      "name": "harvester-snapshot-validation-webhook",
      "jsonPointers":["/webhooks"]}]' $manifest -i
  fi
}

patch_ignoring_resources()
{
  # add ignoring resources
  patch_snapshot_validation_webhook_tls $CHART_MANIFEST
  patch_harvester_snapshot_validation_webhook $CHART_MANIFEST
}

case $CHART_NAME in
  harvester)
    patch_ignoring_resources
    ;;
esac
