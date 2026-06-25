#!/bin/bash -ex

if [[ $# != 1 ]]
then
        echo "We need the settings.yaml from ipxe repo"
        exit 1
fi

SETTINGS=$1

DISABLED_SERVICES="avahi-daemon"
NODE0_IP=$(yq e ".harvester_network_config.cluster[0].ip" ${SETTINGS})

echo "Check services that should be disabled."

SSHKEY=./tmp-ssh-key
for service in ${DISABLED_SERVICES}; do
  echo "Prepare to check ${service} service..."
  if ! ssh -o "StrictHostKeyChecking no" -i tmp-ssh-key rancher@$NODE0_IP "sudo systemctl status ${service}" | grep -q inactive; then
    echo "${service} should not enable, exit!"
    exit 1
  fi
done