#!/bin/bash -ex

if [[ $# != 1 ]]
then
        echo "We need the config.yaml from harvester-dev"
        exit 1
fi

SETTINGS=$1

VIP=$(yq e ".vip" ${SETTINGS})
NODE0_IP=$(yq e ".nodes[0].ip" ${SETTINGS})

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
