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
  yq e '.spec.values.defaultDataPath.defaultSettings.defaultDataPath = "/var/lib/harvester/defaultdisk"' $CHART_MANIFEST -i
}

case $CHART_NAME in
  harvester)
    remove_vip_config
    add_longhorn_settings
    ;;
esac
