#!/bin/bash -ex

CHART_NAME=$1
CHART_MANIFEST=$2

patch_grafana_resources()
{
  # Increase grafana pod limit and request (https://github.com/harvester/harvester-installer/pull/287)
  yq e '.spec.values.grafana.resources = {"limits": {"cpu": "200m", "memory": "500Mi"}, "requests": {"cpu": "100m", "memory": "200Mi"}}' $CHART_MANIFEST -i
}

patch_alertmanager_enable()
{
  # enable alertmanager by default (https://github.com/harvester/harvester-installer/pull/322)
  yq e '.spec.values.alertmanager.enabled = true' $CHART_MANIFEST -i
  yq e '.spec.values.alertmanager.config.global.resolve_timeout = "5m"' $CHART_MANIFEST -i
  yq e '.spec.values.alertmanager.alertmanagerSpec.retention = "120h"' $CHART_MANIFEST -i
  yq e '.spec.values.alertmanager.alertmanagerSpec.resources = {"limits": {"cpu": "1000m", "memory": "600Mi"}, "requests": {"cpu": "100m", "memory": "100Mi"}}' $CHART_MANIFEST -i
  yq e '.spec.values.alertmanager.alertmanagerSpec.storage.volumeClaimTemplate.spec.storageClassName = "harvester-longhorn"' $CHART_MANIFEST -i
  yq e '.spec.values.alertmanager.alertmanagerSpec.storage.volumeClaimTemplate.spec.accessModes = ["ReadWriteOnce"]' $CHART_MANIFEST -i
  yq e '.spec.values.alertmanager.alertmanagerSpec.storage.volumeClaimTemplate.spec.resources.requests.storage = "5Gi"' $CHART_MANIFEST -i
}

patch_alertmanager_externalurl()
{
if [ -n "$HARVESTER_VIP" ]; then
  # enable alertmanager by default (https://github.com/harvester/harvester-installer/pull/322)
  PORT=9093
  yq e '.spec.values.alertmanager.service.port = '$PORT $CHART_MANIFEST -i
  yq e '.spec.values.alertmanager.alertmanagerSpec.externalUrl = "https://'$HARVESTER_VIP'/api/v1/namespaces/cattle-monitoring-system/services/http:rancher-monitoring-alertmanager:'$PORT'/proxy/"' $CHART_MANIFEST -i
fi
}

patch_prometheus_externalurl()
{
if [ -n "$HARVESTER_VIP" ]; then
  # enable alertmanager by default (https://github.com/harvester/harvester-installer/pull/322)
  PORT=9090
  yq e '.spec.values.prometheus.service.port = '$PORT $CHART_MANIFEST -i
  yq e '.spec.values.prometheus.prometheusSpec.externalUrl = "https://'$HARVESTER_VIP'/api/v1/namespaces/cattle-monitoring-system/services/http:rancher-monitoring-prometheus:'$PORT'/proxy/"' $CHART_MANIFEST -i
fi
}

patch_ignoring_resource()
{
	# add ignoring resources when upgrading to match this pr (https://github.com/harvester/harvester-installer/pull/345)
	yq e '.spec.diff.comparePatches = [{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "name": "engineimages.longhorn.io", "jsonPointers":["/status/acceptedNames", "/status/conditions", "/status/storedVersions"]}]' $CHART_MANIFEST -i
	yq e '.spec.diff.comparePatches += [{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "name": "nodes.longhorn.io", "jsonPointers":["/status/acceptedNames", "/status/conditions", "/status/storedVersions"]}]' $CHART_MANIFEST -i
	yq e '.spec.diff.comparePatches += [{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "name": "volumes.longhorn.io", "jsonPointers":["/status/acceptedNames", "/status/conditions", "/status/storedVersions"]}]' $CHART_MANIFEST -i
}

# get harvester vip from service first, then configmap, skip potential error
get_harvester_vip()
{
  EXIT_CODE=0
  #escape the 'return on error'

  VIP=$(kubectl get service -n kube-system ingress-expose -o "jsonpath={.spec.loadBalancerIP}") || EXIT_CODE=$?
  if test $EXIT_CODE = 0; then
    HARVESTER_VIP=$VIP
    return 0
  else
    echo "kubectl get service -n kube-system ingress-expose failed, will try get from configmap"
  fi

  JSON_DATA=$(kubectl get configmap -n kube-system kubevip -o "jsonpath={.data['kubevip-services']}") || EXIT_CODE=$?
  if test $EXIT_CODE = 0; then
    VIP=$(echo $JSON_DATA | jq -r .services[0].vip) || EXIT_CODE=$?
    if test $EXIT_CODE = 0; then
      HARVESTER_VIP=$VIP
    else
      echo "jq parse kubevip configmap json text failed: $JSON_DATA"
    fi
  else
    echo "kubectl get configmap -n kube-system kubevip failed"
  fi
}

# the logging audit is enabled when upgrading from v1.0.3 to v1.1.0
create_logging_audit()
{
  RESOURCE_FILE=/usr/local/share/migrations/managed_charts/logging-audit-v1.0.3.yaml

  if [ -e "$RESOURCE_FILE" ]; then
    echo "add logging audit resource from $RESOURCE_FILE to manifest $CHART_MANIFEST"
    cat $RESOURCE_FILE > $CHART_MANIFEST
  else
    echo "the logging audit resource file $RESOURCE_FILE is not existing, check!"
  fi
}

# the event is enabled when upgrading from v1.0.3 to v1.1.0
create_event()
{
  RESOURCE_FILE=/usr/local/share/migrations/managed_charts/event-v1.0.3.yaml

  if [ -e "$RESOURCE_FILE" ]; then
    echo "add event resource from $RESOURCE_FILE to manifest $CHART_MANIFEST"
    cat $RESOURCE_FILE > $CHART_MANIFEST
  else
    echo "the event resource file $RESOURCE_FILE is not existing, check!"
  fi
}


case $CHART_NAME in
  harvester)
    patch_ignoring_resource
    ;;
  rancher-monitoring)
    patch_grafana_resources
    patch_alertmanager_enable
    get_harvester_vip
    patch_alertmanager_externalurl
    patch_prometheus_externalurl
    ;;
  rancher-logging)
    create_logging_audit
    ;;
  rancher-logging_event-extension)
    create_event
    ;;
esac
