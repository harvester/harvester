#!/bin/bash -ex

CHART_NAME=$1
CHART_MANIFEST=$2

patch_grafana_resources()
{
  # Increase grafana pod limit and request (https://github.com/harvester/harvester-installer/pull/287)
  yq e '.spec.values.grafana.resources = {"limits": {"cpu": "200m", "memory": "500Mi"}, "requests": {"cpu": "100m", "memory": "200Mi"}}' $CHART_MANIFEST -i
}

case $CHART_NAME in
  rancher-monitoring)
    patch_grafana_resources
    ;;
esac
