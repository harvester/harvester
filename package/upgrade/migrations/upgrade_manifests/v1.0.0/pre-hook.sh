#!/bin/bash -e

prepare_plan_manifest=$(mktemp --suffix=.yaml)
plan_name=${HARVESTER_UPGRADE_NAME}-prepare-again
plan_version=${HARVESTER_UPGRADE_NAME}

cat > $prepare_plan_manifest <<EOF
apiVersion: upgrade.cattle.io/v1
kind: Plan
metadata:
  name: ${plan_name}
  namespace: cattle-system
spec:
  concurrency: 2
  nodeSelector:
    matchLabels:
      harvesterhci.io/managed: "true"
  serviceAccountName: system-upgrade-controller
  tolerations:
  - key: CriticalAddonsOnly
    operator: Exists
  - effect: NoSchedule
    key: kubevirt.io/drain
    operator: Exists
  - effect: NoExecute
    key: node-role.kubernetes.io/control-plane
    operator: Exists
  - effect: NoSchedule
    key: kubernetes.io/arch
    operator: Equal
    value: amd64
  - effect: NoSchedule
    key: kubernetes.io/arch
    operator: Equal
    value: arm64
  - effect: NoSchedule
    key: kubernetes.io/arch
    operator: Equal
    value: arm
  upgrade:
    args:
    - prepare
    command:
    - upgrade_node.sh
    envs:
    - name: HARVESTER_UPGRADE_NAME
      value: ${HARVESTER_UPGRADE_NAME}
    image: rancher/harvester-upgrade:${REPO_HARVESTER_VERSION}
  version: ${plan_version}
EOF

echo "Creating plan $plan_name to preload images again..."
kubectl create -f $prepare_plan_manifest

# Wait for all nodes complete

while [ true ]; do
  plan_label="plan.upgrade.cattle.io/${plan_name}"
  plan_latest_version=$(kubectl get plans.upgrade.cattle.io "$plan_name" -n cattle-system -ojsonpath="{.status.latestVersion}")

  if [ "$plan_latest_version" = "$plan_version" ]; then
    plan_latest_hash=$(kubectl get plans.upgrade.cattle.io "$plan_name" -n cattle-system -ojsonpath="{.status.latestHash}")
    total_nodes_count=$(kubectl get nodes -o json | jq '.items | length')
    complete_nodes_count=$(kubectl get nodes --selector="plan.upgrade.cattle.io/${plan_name}=$plan_latest_hash" -o json | jq '.items | length')

    if [ "$total_nodes_count" = "$complete_nodes_count" ]; then
      echo "Plan ${plan_name} completes."
      break
    fi
  fi

  echo "Waiting for plan ${plan_name} to complete..."
  sleep 10
done

echo "Deleting plan $plan_name..."
kubectl delete plans.upgrade.cattle.io "$plan_name" -n cattle-system
rm -f $prepare_plan_manifest
