#!/bin/bash -e

if [[ $# != 1 ]]
then
        echo "We need the settings.yaml from ipxe repo"
        exit 1
fi

echo "test vm network..."
TEST_VM_IP=$(./terraform show -json |jq '.values.root_module.resources[] |select (.name=="cirros-01")'|jq .values.network_interface[0].ip_address)
SETTINGS=$1

NODE0_IP=$(yq e ".harvester_network_config.cluster[0].ip" ${SETTINGS})

retries=0
while [ true ]; do
    SSHKEY=./tmp-ssh-key
    if ssh -i ${SSHKEY} rancher@$NODE0_IP "ping -c 5 $TEST_VM_IP"; then
      echo "VM is alive."
      break
    fi

  if [ $retries -eq 60 ]; then
    echo "Pinging VM timed out. Exit!"
    exit 1
  fi

  echo "Fail to ping VM, will retry in 5 seconds..."
  sleep 5
  retries=$((retries+1))
done
