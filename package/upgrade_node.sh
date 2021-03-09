#!/bin/bash

PROG=$0
usage()
{
    echo "Usage: $PROG [--prepare] [--debug]"
    exit 1
}

upgrade()
{
  if [ -d /host/var/lib/rancher/k3s/server/static/charts ];then
    cp /harvester*.tgz /host/var/lib/rancher/k3s/server/static/charts/
    echo "Copied the Harvester chart"
  fi

  if [ -f /host/var/lib/rancher/k3s/server/manifests/harvester.yaml ];then
    rm /host/var/lib/rancher/k3s/server/manifests/harvester.yaml
    echo "Removed manifests of the old version"
  fi

  # remove the node taint
  NODE=$(kubectl get po -o=custom-columns=NAME:.metadata.name,NODE:.spec.nodeName|grep $HOSTNAME|awk '{print $2}')
  kubectl taint node $NODE kubevirt.io/drain-

  echo "upgrading k3os"
  k3os --debug upgrade --kernel --rootfs --remount --sync --reboot \
  --lock-file=/host/run/k3os/upgrade.lock --source=/k3os/system --destination=/host/k3os/system
}

# prepare is run before node draining
prepare()
{
  echo "Running prepare scripts"
  NODE=$(kubectl get po -o=custom-columns=NAME:.metadata.name,NODE:.spec.nodeName|grep $HOSTNAME|awk '{print $2}')
  # taint the node to trigger VM live migrations
  kubectl taint node $NODE kubevirt.io/drain=draining:NoSchedule
  echo "Finished prepare scripts"
}


while [ "$#" -gt 0 ]; do
    case $1 in
        --debug)
            set -x
            ;;
        --prepare)
            HARVESTER_UPGRADE_PREPARE=true
            ;;
        *)
            break
            ;;
    esac
    shift 1
done

if [ "$HARVESTER_UPGRADE_PREPARE" = "true" ]; then
  prepare
else
  upgrade
fi