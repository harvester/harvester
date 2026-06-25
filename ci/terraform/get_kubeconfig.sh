#!/bin/bash -ex

if [[ $# != 1 ]]
then
        echo "We need the settings.yaml from ipxe repo"
        exit 1
fi

SETTINGS=$1

VIP=$(yq e ".harvester_network_config.vip.ip" ${SETTINGS})
NODE0_IP=$(yq e ".harvester_network_config.cluster[0].ip" ${SETTINGS})

echo "Get kubeconfig from ${NODE0_IP}, VIP: ${VIP}"

# cleanup
ssh-keygen -R ${NODE0_IP} || true
rm -rf kubeconf || true

# get kubeconfig
ssh -o "StrictHostKeyChecking no" -i tmp-ssh-key rancher@$NODE0_IP "sudo cat /etc/rancher/rke2/rke2.yaml" > kubeconfig
sed -i "s,127.0.0.1:6443,$VIP:6443," kubeconfig

# move kubeconfig to related folder
mkdir kubeconf/
mv kubeconfig kubeconf/
