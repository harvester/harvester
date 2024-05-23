#!/bin/bash -ex

CHART_NAME=$1
CHART_MANIFEST=$2

create_crd_patch_if_not_exists()
{
  local manifest=$1
  local crd=$2

  export CRD_NAME="$crd"
  result=$(yq '.spec.diff.comparePatches[] |
                 select(
                   .apiVersion == "apiextensions.k8s.io/v1" and
                   .kind == "CustomResourceDefinition" and
                   .name == strenv(CRD_NAME))' $manifest 2>/dev/null)

  if [ -z "$result" ]; then
    yq e '.spec.diff.comparePatches += [{
      "apiVersion": "apiextensions.k8s.io/v1",
      "kind": "CustomResourceDefinition",
      "name": strenv(CRD_NAME),
      "jsonPointers":["/status/acceptedNames", "/status/conditions", "/status/storedVersions"]}]' $manifest -i
  fi
}

patch_ignoring_resources()
{
  # add ignoring resources
  create_crd_patch_if_not_exists $CHART_MANIFEST "settings.longhorn.io"
  create_crd_patch_if_not_exists $CHART_MANIFEST "replicas.longhorn.io"
  create_crd_patch_if_not_exists $CHART_MANIFEST "instancemanagers.longhorn.io"
  create_crd_patch_if_not_exists $CHART_MANIFEST "engines.longhorn.io"
}

case $CHART_NAME in
  harvester)
    patch_ignoring_resources
    ;;
esac
