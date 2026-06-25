#!/bin/bash -ex

if [[ $# != 1 ]]
then
        echo "We need the settings.yaml from ipxe repo"
        exit 1
fi

SETTINGS=$1

EXISTING_FILES="/oem/grubenv /oem/grubcustom"
NODE0_IP=$(yq e ".harvester_network_config.cluster[0].ip" ${SETTINGS})

echo "Check files that should exist."

SSHKEY=./tmp-ssh-key
for filepath in ${EXISTING_FILES}; do
  echo "Prepare to check file: ${filepath} ..."
  if ! ssh -o "StrictHostKeyChecking no" -i tmp-ssh-key rancher@$NODE0_IP "sudo stat ${filepath}" | grep -q ${filepath}; then
    echo "${filepath} should exist, exit!"
    exit 1
  fi
done