#!/bin/bash -ex

CHART_NAME=$1
CHART_MANIFEST=$2

remove_vip_config()
{
  # VIP interface isn't needed after kube-vip v0.4.1 (https://github.com/harvester/harvester-installer/pull/216)
  yq e 'del(.spec.values.kube-vip.config)' $CHART_MANIFEST -i
}


add_longhorn_settings()
{
  # Add config "DataDisk" to specify the default Longhorn partition (https://github.com/harvester/harvester-installer/pull/217)
  yq e '.spec.values.longhorn.defaultSettings.defaultDataPath = "/var/lib/harvester/defaultdisk"' $CHART_MANIFEST -i
}

adjust_prometheus_resources()
{
  # Increate prometheus pod limit and request (https://github.com/harvester/harvester-installer/pull/240)
  yq e '.spec.values.prometheus.prometheusSpec.resources = {"limits": {"cpu": "1000m", "memory": "2500Mi"}, "requests": {"cpu": "750m", "memory": "1750Mi"}}' $CHART_MANIFEST -i

  # Increase limits.memory (https://github.com/harvester/harvester-installer/pull/250)
  yq e '.spec.values.prometheus-node-exporter.resources = {"limits": {"cpu": "200m", "memory": "180Mi"}, "requests": {"cpu": "100m", "memory": "30Mi"}}' $CHART_MANIFEST -i
}

case $CHART_NAME in
  harvester)
    remove_vip_config
    add_longhorn_settings
    ;;
  rancher-monitoring)
    adjust_prometheus_resources
    ;;
esac
