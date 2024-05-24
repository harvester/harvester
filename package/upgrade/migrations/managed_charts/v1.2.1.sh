#!/bin/bash -ex

CHART_NAME=$1
CHART_MANIFEST=$2

patch_ignoring_resource()
{
	# add ignoring resources ()
	yq e '.spec.diff.comparePatches += [{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "name": "settings.longhorn.io", "jsonPointers":["/status/acceptedNames", "/status/conditions", "/status/storedVersions"]}]' $CHART_MANIFEST -i
	yq e '.spec.diff.comparePatches += [{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "name": "replicas.longhorn.io", "jsonPointers":["/status/acceptedNames", "/status/conditions", "/status/storedVersions"]}]' $CHART_MANIFEST -i
	yq e '.spec.diff.comparePatches += [{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "name": "instancemanagers.longhorn.io", "jsonPointers":["/status/acceptedNames", "/status/conditions", "/status/storedVersions"]}]' $CHART_MANIFEST -i
	yq e '.spec.diff.comparePatches += [{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "name": "engines.longhorn.io", "jsonPointers":["/status/acceptedNames", "/status/conditions", "/status/storedVersions"]}]' $CHART_MANIFEST -i
}

case $CHART_NAME in
  harvester)
    patch_ignoring_resource
    ;;
esac