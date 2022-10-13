#!/bin/bash -e

echo "Remove rke.cattle.io/init-node-machine-id label in fleet-local/local cluster"
kubectl -n fleet-local label clusters.provisioning.cattle.io local rke.cattle.io/init-node-machine-id-
