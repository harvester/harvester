#!/bin/bash -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
UPGRADE_TMP_DIR="/tmp/upgrade"

source $SCRIPT_DIR/lib.sh

pre_upgrade_manifest() {
  if [ -e "/usr/local/share/migrations/upgrade_manifests/${UPGRADE_PREVIOUS_VERSION}/pre-hook.sh" ]; then
    echo "Executing ${UPGRADE_PREVIOUS_VERSION} pre-hook..."
    # Use source to pass current shell's variables to target script
    source "/usr/local/share/migrations/upgrade_manifests/${UPGRADE_PREVIOUS_VERSION}/pre-hook.sh"
  fi
}

wait_managed_chart() {
  namespace=$1
  name=$2
  version=$3
  generation=$4
  state=$5

  echo "Waiting for ManagedChart $namespace/$name from generation $generation"
  echo "Target version: $version, Target state: $state"

  while [ true ]; do
    current_chart=$(kubectl get managedcharts.management.cattle.io $name -n $namespace -o yaml)
    current_version=$(echo "$current_chart" | yq e '.spec.version' -)
    current_observed_generation=$(echo "$current_chart" | yq e '.status.observedGeneration' -)
    current_state=$(echo "$current_chart" | yq e '.status.display.state' -)
    echo "Current version: $current_version, Current state: $current_state, Current generation: $current_observed_generation"

    if [ "$current_version" = "$version" ]; then
      if [ "$current_observed_generation" -gt "$generation" ]; then
        summary_state=$(echo "$current_chart" | yq e ".status.summary.$state" -)
        if [ "$summary_state" = "1" ]; then
          break
        fi
      fi
    fi

    sleep 5
    echo "Sleep for 5 seconds to retry"
  done
}

wait_helm_release() {
  # Wait for helm release to be deployed to a specified version
  namespace=$1
  release_name=$2
  chart=$3
  app_version=$4
  status=$5

  while [ true ]; do
    last_history=$(helm history $release_name -n $namespace -o yaml | yq e '.[-1]' -)

    current_chart=$(echo "$last_history" | yq e '.chart' -)
    current_app_version=$(echo "$last_history" | yq e '.app_version' -)
    current_status=$(echo "$last_history" | yq e '.status' -)

    if [ "$current_chart" != "$chart" ]; then
      sleep 5
      continue
    fi

    if [ "$current_app_version" != "$app_version" ]; then
      sleep 5
      continue
    fi

    if [ "$current_status" != "$status" ]; then
      sleep 5
      continue
    fi

    break
  done
}

wait_rollout() {
  namespace=$1
  resource_type=$2
  name=$3

  kubectl rollout status --watch=true -n $namespace $resource_type $name
}

wait_capi_cluster() {
  # Wait for cluster to settle down
  namespace=$1
  name=$2
  generation=$3

  while [ true ]; do
    cluster=$(kubectl get clusters.cluster.x-k8s.io $name -n $namespace -o yaml)

    current_generation=$(echo "$cluster" | yq e '.status.observedGeneration' -)
    current_phase=$(echo "$cluster" | yq e '.status.phase' -)

    if [ "$current_generation" -gt "$generation" ]; then
      if [ "$current_phase" = "Provisioned" ]; then
        echo "CAPI cluster $namespace/$name is provisioned (current generation: $current_generation)."
        break
      fi
    fi

    echo "Waiting for CAPI cluster $namespace/$name to be provisioned (current phase: $current_phase, current generation: $current_generation)..."
    sleep 5
  done
}

wait_kubevirt() {
  # Wait for kubevirt to be upgraded
  namespace=$1
  name=$2
  version=$3

  echo "Waiting for KubeVirt to upgraded to $version..."
  while [ true ]; do
    kubevirt=$(kubectl get kubevirts.kubevirt.io $name -n $namespace -o yaml)

    current_phase=$(echo "$kubevirt" | yq e '.status.phase' -)
    current_target_version=$(echo "$kubevirt" | yq e '.status.observedKubeVirtVersion' -)

    if [ "$current_target_version" = "$version" ]; then
      if [ "$current_phase" = "Deployed" ]; then
        break
      fi
    fi

    echo "KubeVirt current version: $current_target_version, target version: $version"
    sleep 5
  done
}

wait_longhorn_manager() {
  echo "Waiting for longhorn-manager to be upgraded..."

  lm_repo=$(kubectl get apps.catalog.cattle.io/harvester -n harvester-system -o json | jq -r .spec.chart.values.longhorn.image.longhorn.manager.repository)
  lm_tag=$(kubectl get apps.catalog.cattle.io/harvester -n harvester-system -o json | jq -r .spec.chart.values.longhorn.image.longhorn.manager.tag)
  lm_image="${lm_repo}:${lm_tag}"
  node_count=$(kubectl get nodes --selector=harvesterhci.io/managed=true -o json | jq -r '.items | length')

  while [ true ]; do
    lm_ds_ready=0
    lm_ds_image=$(kubectl get daemonset longhorn-manager -n longhorn-system -o jsonpath='{.spec.template.spec.containers[0].image}')

    if [ "$lm_ds_image" = "$lm_image" ]; then
      lm_ds_ready=$(kubectl get daemonset longhorn-manager -n longhorn-system -o jsonpath='{.status.numberReady}')
      if [ $lm_ds_ready -eq $node_count ]; then
        break
      fi
    fi

    echo "Waiting for longhorn-manager to be upgraded ($lm_ds_ready/$node_count)..."
    sleep 10
  done
}

wait_longhorn_instance_manager_r() {
  im_repo=$(kubectl get apps.catalog.cattle.io/harvester -n harvester-system -o json | jq -r .spec.chart.values.longhorn.image.longhorn.instanceManager.repository)
  im_tag=$(kubectl get apps.catalog.cattle.io/harvester -n harvester-system -o json | jq -r .spec.chart.values.longhorn.image.longhorn.instanceManager.tag)
  im_image="${im_repo}:${im_tag}"

  node_count=$(kubectl get nodes --selector=harvesterhci.io/managed=true -o json | jq -r '.items | length')
  if [ $node_count -le 2 ]; then
    echo "Skip waiting instance-manager-r, node count: $node_count"
    return
  fi

  # Wait for instance-manager-r pods upgraded to new version first.
  kubectl get nodes -o json | jq -r '.items[].metadata.name' | while read -r node; do
    echo "Checking instance-manager-r pod on node $node..."
    while [ true ]; do
      pod_count=$(kubectl get pod --selector=longhorn.io/node=$node,longhorn.io/instance-manager-type=replica -n longhorn-system -o json | jq -r '.items | length')
      if [ "$pod_count" != "1" ]; then
        echo "instance-manager-r pod count is not 1 on node $node, will retry..."
        sleep 5
        continue
      fi

      container_image=$(kubectl get pod --selector=longhorn.io/node=$node,longhorn.io/instance-manager-type=replica -n longhorn-system -o json | jq -r '.items[0].spec.containers[0].image')
      if [ "$container_image" != "$im_image" ]; then
        echo "instance-manager-r pod image is not $im_image, will retry..."
        sleep 5
        continue
      fi

      echo "Checking instance-manager-r pod on node $node OK."
      break
    done
  done
}

wait_longhorn_upgrade() {
  echo "Waiting for LH settling down..."
  wait_longhorn_manager
  wait_longhorn_instance_manager_r
}

get_running_rancher_version() {
  kubectl get settings.management.cattle.io server-version -o yaml | yq -e e '.value' -
}

get_cluster_repo_index_download_time() {
  local output_type=$1
  local iso_time=$(kubectl get clusterrepos.catalog.cattle.io harvester-charts -ojsonpath='{.status.downloadTime}')

  if [ "$output_type" = "epoch" ]; then
    date -d"${iso_time}" +%s
  else
    echo $iso_time
  fi
}

upgrade_rancher() {
  echo "Upgrading Rancher"

  mkdir -p $UPGRADE_TMP_DIR/images
  mkdir -p $UPGRADE_TMP_DIR/rancher

  # Download rancher system agent install image from upgrade repo
  download_image_archives_from_repo "agent" $UPGRADE_TMP_DIR/images

  # Extract the Rancher chart and helm binary
  wharfie --images-dir $UPGRADE_TMP_DIR/images rancher/system-agent-installer-rancher:$REPO_RANCHER_VERSION $UPGRADE_TMP_DIR/rancher

  cd $UPGRADE_TMP_DIR/rancher

  ./helm get values rancher -n cattle-system -o yaml >values.yaml
  echo "Rancher values:"
  cat values.yaml

  RANCHER_CURRENT_VERSION=$(yq -e e '.rancherImageTag' values.yaml)
  if [ -z "$RANCHER_CURRENT_VERSION" ]; then
    echo "[ERROR] Fail to get current Rancher version."
    exit 1
  fi

  if [ "$RANCHER_CURRENT_VERSION" = "$REPO_RANCHER_VERSION" ]; then
    echo "Skip update Rancher. The version is already $RANCHER_CURRENT_VERSION"
    return
  fi

  # Wait for Rancher to settle down before start upgrading, just in case
  wait_capi_cluster fleet-local local 0
  pre_generation=$(kubectl get clusters.cluster.x-k8s.io local -n fleet-local -o=jsonpath="{.status.observedGeneration}")

  # XXX Workaround for https://github.com/rancher/rancher/issues/36914
  # Delete all rancher's clusterrepos so they will be updated by the new version rancher pods
  # Note: The leader pod would create these cluster repos
  kubectl delete clusterrepos.catalog.cattle.io rancher-charts
  kubectl delete clusterrepos.catalog.cattle.io rancher-rke2-charts
  kubectl delete clusterrepos.catalog.cattle.io rancher-partner-charts
  kubectl delete settings.management.cattle.io chart-default-branch

  REPO_RANCHER_VERSION=$REPO_RANCHER_VERSION yq -e e '.rancherImageTag = strenv(REPO_RANCHER_VERSION)' values.yaml -i
  ./helm upgrade rancher ./*.tgz --namespace cattle-system -f values.yaml --wait

  # Wait until new version ready
  until [ "$(get_running_rancher_version)" = "$REPO_RANCHER_VERSION" ]; do
    echo "Wait for Rancher to be upgraded..."
    sleep 5
  done

  echo "Wait for Rancher dependencies helm releases..."
  wait_helm_release cattle-fleet-system fleet fleet-$REPO_FLEET_CHART_VERSION $REPO_FLEET_APP_VERSION deployed
  wait_helm_release cattle-fleet-system fleet-crd fleet-crd-$REPO_FLEET_CRD_CHART_VERSION $REPO_FLEET_CRD_APP_VERSION deployed
  wait_helm_release cattle-system rancher-webhook rancher-webhook-$REPO_RANCHER_WEBHOOK_CHART_VERSION $REPO_RANCHER_WEBHOOK_APP_VERSION deployed
  echo "Wait for Rancher dependencies rollout..."
  wait_rollout cattle-fleet-local-system deployment fleet-agent
  wait_rollout cattle-fleet-system deployment fleet-controller
  wait_rollout cattle-system deployment rancher-webhook
  echo "Wait for cluster settling down..."
  wait_capi_cluster fleet-local local $pre_generation
}

update_local_rke_state_secret() {
  # Starting from Rancher v2.7, the local-rke-state Secret needs to be in type of "rke.cattle.io/current-state"
  # Need to convert it from the "Opaque" type; otherwise, the following RKE2 upgrades won't start
  # Ref: https://github.com/rancher/rancher/pull/41088
  readonly secret_name="local-rke-state"
  readonly new_secret_type="rke.cattle.io/cluster-state"

  echo "Check current local-rke-state secret type..."
  local secret_file
  secret_file=$(mktemp -p "$UPGRADE_TMP_DIR")
  kubectl -n fleet-local get secrets "$secret_name" -o yaml > "$secret_file"
  local current_secret_type
  current_secret_type=$(yq '.type' "$secret_file")
  if [ "$current_secret_type" = "Opaque" ]; then
    secret_type="$new_secret_type" yq -e '.type = strenv(secret_type)' "$secret_file" -i
  else
    echo "Current secret type is: $current_secret_type, skip update"
    return
  fi

  echo "Scale down Rancher deployment to 0"
  local rancher_deployment_replica_count
  rancher_deployment_replica_count=$(kubectl -n cattle-system get deployment rancher -o jsonpath='{.spec.replicas}')
  kubectl -n cattle-system scale deployment rancher --replicas=0

  echo "Remove old secret and apply new one"
  kubectl -n fleet-local delete secret local-rke-state
  kubectl -n fleet-local apply -f "$secret_file"

  echo "Scale up Rancher deployment to original replica count"
  kubectl -n cattle-system scale deployment rancher --replicas="$rancher_deployment_replica_count"
  echo "Wait for Rancher deployment becoming ready"
  wait_rollout cattle-system deployment rancher
}

upgrade_harvester_cluster_repo() {
  echo "Upgrading Harvester Cluster Repo"

  mkdir -p $UPGRADE_TMP_DIR/harvester_cluster_repo
  cd $UPGRADE_TMP_DIR/harvester_cluster_repo

  cat >cluster_repo.yaml <<EOF
spec:
  template:
    spec:
      containers:
        - name: httpd
          image: rancher/harvester-cluster-repo:$REPO_OS_VERSION
EOF
  kubectl patch deployment harvester-cluster-repo -n cattle-system --patch-file ./cluster_repo.yaml --type merge

  until kubectl -n cattle-system rollout status -w deployment/harvester-cluster-repo; do
    echo "Waiting for harvester-cluster-repo deployment ready..."
    sleep 5
  done

  # Force update cluster repo catalog index
  last_repo_download_time=$(get_cluster_repo_index_download_time)
  # See https://github.com/rancher/rancher/blob/47c22388c5451c74f55e162d1e60b4e6dcfd0800/pkg/controllers/dashboard/helm/repo.go#L290-L294
  # for why this would trigger a force upgrade
  force_update_time=$(date -d"${last_repo_download_time} + 1 seconds" --iso-8601=seconds)
  # Sleep 1 sec to ensure force_update_time is always in the past
  sleep 1

  cat >catalog_cluster_repo.yaml <<EOF
spec:
  forceUpdate: "$force_update_time"
EOF
  kubectl patch clusterrepos.catalog.cattle.io harvester-charts --patch-file ./catalog_cluster_repo.yaml --type merge

  until [ $(get_cluster_repo_index_download_time epoch) -ge $(date -d"${force_update_time}" +%s) ]; do
    echo "Waiting for cluster repo catalog index update..."
    sleep 5
  done
}

upgrade_network(){
  [[ $UPGRADE_PREVIOUS_VERSION != "v1.0.3" ]] && return

  shutdown_all_vms
  wait_all_vms_shutdown
  modify_nad_bridge
  delete_canal_flannel_iface
}

wait_all_vms_shutdown() {
    local vm_count="$(get_all_running_vm_count)"

    until [ "$vm_count" = "0" ]
    do
      echo "Waiting for VM shutdown...($vm_count left)"
      sleep 5
      vm_count="$(get_all_running_vm_count)"
    done
}

get_all_running_vm_count() {
  local count

  count=$(kubectl get vmi -A -ojson | jq '.items | length' || true)
  echo $count
}

delete_canal_flannel_iface() {
  kubectl delete helmchartconfig rke2-canal -n kube-system || true
  kubectl patch configmap rke2-canal-config -n kube-system -p '{"data":{"canal_iface": ""}}' --type merge
}

modify_nad_bridge() {
  [[ $(kubectl get clusternetwork vlan -o yaml | yq '.enable') == "false" ]] && echo "VLAN is disabled" && return

  local bridge="vlan-br"
  [[ $(kubectl get nodenetwork -o yaml | yq '.items[].spec.nic | select(. == "harvester-mgmt")') ]] && bridge="mgmt-br"

  kubectl get net-attach-def -A -o json |
  jq -r '.items[] | [.metadata.name, .metadata.namespace] | @tsv' |
      while IFS=$'\t' read -r name namespace; do
        if [ -z "$name" ]; then
          break
        fi
        local nad=$(kubectl get net-attach-def -n "$namespace" "$name" -o yaml)
        local config=$(echo "$nad" | yq '.spec.config')
        export new_config=$(echo "$config" | jq -c --arg v "$bridge" '.bridge = $v')
        echo "$nad" | yq '.spec.config = strenv(new_config)' | kubectl apply -f -
      done
}

upgrade_harvester() {
  echo "Upgrading Harvester"

  pre_generation_harvester=$(kubectl get managedcharts.management.cattle.io harvester -n fleet-local -o=jsonpath='{.status.observedGeneration}')
  pre_generation_harvester_crd=$(kubectl get managedcharts.management.cattle.io harvester-crd -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  mkdir -p $UPGRADE_TMP_DIR/harvester
  cd $UPGRADE_TMP_DIR/harvester

  cat >harvester-crd.yaml <<EOF
spec:
  version: $REPO_HARVESTER_CHART_VERSION
EOF
  kubectl patch managedcharts.management.cattle.io harvester-crd -n fleet-local --patch-file ./harvester-crd.yaml --type merge

  cat >harvester.yaml <<EOF
apiVersion: management.cattle.io/v3
kind: ManagedChart
metadata:
  name: harvester
  namespace: fleet-local
EOF
  kubectl get managedcharts.management.cattle.io -n fleet-local harvester -o yaml | yq e '{"spec": .spec}' - >>harvester.yaml

  upgrade_managed_chart_from_version $UPGRADE_PREVIOUS_VERSION harvester harvester.yaml
  NEW_VERSION=$REPO_HARVESTER_CHART_VERSION yq e '.spec.version = strenv(NEW_VERSION)' harvester.yaml -i

  local sc=$(kubectl get sc -o json | jq '.items[] | select(.metadata.annotations."storageclass.kubernetes.io/is-default-class" == "true" and .metadata.name != "harvester-longhorn")')
  if [ -n "$sc" ] && [ "$UPGRADE_PREVIOUS_VERSION" != "v1.0.3" ]; then
      yq e '.spec.values.storageClass.defaultStorageClass = false' -i harvester.yaml
  fi

  kubectl apply -f ./harvester.yaml

  pause_managed_chart harvester "false"
  pause_managed_chart harvester-crd "false"

  wait_managed_chart fleet-local harvester $REPO_HARVESTER_CHART_VERSION $pre_generation_harvester ready
  wait_managed_chart fleet-local harvester-crd $REPO_HARVESTER_CHART_VERSION $pre_generation_harvester_crd ready

  wait_kubevirt harvester-system kubevirt $REPO_KUBEVIRT_VERSION
}

#upgrade both monitoring and monitoring_crd
upgrade_managedchart_monitoring_and_crd() {
  echo "Upgrading Monitoring"

  pre_generation_monitoring=$(kubectl get managedcharts.management.cattle.io rancher-monitoring -n fleet-local -o=jsonpath='{.status.observedGeneration}')
  pre_generation_monitoring_crd=$(kubectl get managedcharts.management.cattle.io rancher-monitoring-crd -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  mkdir -p $UPGRADE_TMP_DIR/monitoring
  cd $UPGRADE_TMP_DIR/monitoring

  cat >rancher-monitoring-crd.yaml <<EOF
spec:
  version: $REPO_MONITORING_CHART_VERSION
EOF
  kubectl patch managedcharts.management.cattle.io rancher-monitoring-crd -n fleet-local --patch-file ./rancher-monitoring-crd.yaml --type merge

  cat >rancher-monitoring.yaml <<EOF
apiVersion: management.cattle.io/v3
kind: ManagedChart
metadata:
  name: rancher-monitoring
  namespace: fleet-local
EOF
  kubectl get managedcharts.management.cattle.io -n fleet-local rancher-monitoring -o yaml | yq e '{"spec": .spec}' - >>rancher-monitoring.yaml

  upgrade_managed_chart_from_version $UPGRADE_PREVIOUS_VERSION rancher-monitoring rancher-monitoring.yaml
  NEW_VERSION=$REPO_MONITORING_CHART_VERSION yq e '.spec.version = strenv(NEW_VERSION)' rancher-monitoring.yaml -i
  kubectl apply -f ./rancher-monitoring.yaml

  pause_managed_chart rancher-monitoring "false"
  pause_managed_chart rancher-monitoring-crd "false"

  wait_managed_chart fleet-local rancher-monitoring $REPO_MONITORING_CHART_VERSION $pre_generation_monitoring ready
  wait_managed_chart fleet-local rancher-monitoring-crd $REPO_MONITORING_CHART_VERSION $pre_generation_monitoring_crd ready

  wait_rollout cattle-monitoring-system daemonset rancher-monitoring-prometheus-node-exporter
  wait_rollout cattle-monitoring-system deployment rancher-monitoring-operator
}

#upgrade only monitoring_crd
upgrade_managedchart_monitoring_crd() {
  local nm=rancher-monitoring-crd
  echo "Upgrading Managedchart $nm to $REPO_MONITORING_CHART_VERSION"

  local pre_version=$(kubectl get managedcharts.management.cattle.io "$nm" -n fleet-local -o=jsonpath='{.spec.version}')
  if [ "$pre_version" = "$REPO_MONITORING_CHART_VERSION" ]; then
    echo "the $nm has already been target version $REPO_MONITORING_CHART_VERSION, nothing to upgrade"
    pause_managed_chart "$nm" "false"
    return 0
  fi

  local pre_generation=$(kubectl get managedcharts.management.cattle.io "$nm" -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  mkdir -p $UPGRADE_TMP_DIR/monitoring
  cd $UPGRADE_TMP_DIR/monitoring

  cat >"$nm".yaml <<EOF
spec:
  version: $REPO_MONITORING_CHART_VERSION
EOF

  kubectl patch managedcharts.management.cattle.io "$nm" -n fleet-local --patch-file ./"$nm".yaml --type merge

  pause_managed_chart "$nm" "false"

  wait_managed_chart fleet-local "$nm" $REPO_MONITORING_CHART_VERSION $pre_generation ready
}

upgrade_monitoring() {
  if [[ $(is_formal_release $UPGRADE_PREVIOUS_VERSION) = "true" ]]; then
    if [[ "$UPGRADE_PREVIOUS_VERSION" > "v1.1.1" ]] && [[ "$UPGRADE_PREVIOUS_VERSION" < "v1.2.0" ]]; then
      upgrade_managedchart_monitoring_crd
      convert_monitoring_to_addon
    elif [[ "$UPGRADE_PREVIOUS_VERSION" = "v1.2.0" ]] || [[ "$UPGRADE_PREVIOUS_VERSION" > "v1.2.0" ]]; then
      # from v1.2.0, only crd here, rancher-monitoring is upgraded in addons
      upgrade_managedchart_monitoring_crd
    else
      # legacy, upgrade both
      upgrade_managedchart_monitoring_and_crd
    fi
  else
   # for version like v1.2.0/v1.1.2 master-head to master-head/rc/formal release, try convert anyway
    upgrade_managedchart_monitoring_crd
    convert_monitoring_to_addon
  fi
}

upgrade_managedchart_logging_crd() {
  nm=rancher-logging-crd
  echo "Upgrading Managedchart $nm to $REPO_LOGGING_CHART_VERSION"

  pre_version=$(kubectl get managedcharts.management.cattle.io "$nm" -n fleet-local -o=jsonpath='{.spec.version}')
  if [ "$pre_version" = "$REPO_LOGGING_CHART_VERSION" ]; then
    echo "the $nm has already been target version $REPO_LOGGING_CHART_VERSION, nothing to upgrade"
    pause_managed_chart "$nm" "false"
    return 0
  fi

  pre_generation_logging_crd=$(kubectl get managedcharts.management.cattle.io "$nm" -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  mkdir -p $UPGRADE_TMP_DIR/logging
  cd $UPGRADE_TMP_DIR/logging

  cat >"$nm".yaml <<EOF
spec:
  version: $REPO_LOGGING_CHART_VERSION
EOF

  kubectl patch managedcharts.management.cattle.io "$nm" -n fleet-local --patch-file ./"$nm".yaml --type merge

  pause_managed_chart "$nm" "false"

  wait_managed_chart fleet-local "$nm" $REPO_LOGGING_CHART_VERSION $pre_generation_logging_crd ready
}

loop_wait_rollout_logging_audit() {
  local NS=cattle-logging-system

  for i in $(seq 1 $1)
  do
    local EXIT_CODE=0 # reset each loop
    sleep 10

    # logging operator
    wait_rollout $NS deployment rancher-logging || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout deployment rancher-logging, $i"
      continue
    fi

    # agent to grab log
    wait_rollout $NS daemonset rancher-logging-root-fluentbit || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout daemonset rancher-logging-root-fluentbit, $i"
      continue
    fi

    wait_rollout $NS daemonset rancher-logging-rke2-journald-aggregator || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout daemonset rancher-logging-rke2-journald-aggregator, $i"
      continue
    fi

    wait_rollout $NS daemonset rancher-logging-kube-audit-fluentbit || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout daemonset rancher-logging-kube-audit-fluentbit, $i"
      continue
    fi

    # fluentd, a known issue: https://github.com/harvester/harvester/issues/2787
    # wait_rollout cattle-logging-system statefulset rancher-logging-root-fluentd
    # wait_rollout cattle-logging-system statefulset rancher-logging-kube-audit-fluentd

    break
  done

  if [ $EXIT_CODE != 0 ]; then
    echo "fail to wait rollout logging audit"
    return $EXIT_CODE
  fi

  echo "success to wait rollout logging audit"
  return 0
}

loop_wait_rollout_event() {
  local NS=cattle-logging-system
  local NAME=harvester-default-event-tailer

  for i in $(seq 1 $1)
  do
    local EXIT_CODE=0 # reset each loop
    sleep 10

    wait_rollout $NS statefulset $NAME || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout statefulset $NAME, $i"
      continue
    fi

    break
  done

  if [ $EXIT_CODE != 0 ]; then
    echo "fail to wait rollout event"
    return $EXIT_CODE
  fi

  echo "success to wait rollout event"
  return 0
}

loop_wait_rollout_monitoring() {
  local NS=cattle-monitoring-system

  for i in $(seq 1 $1)
  do
    local EXIT_CODE=0 # reset each loop
    sleep 10

    local nm=rancher-monitoring-operator
    wait_rollout $NS deployment $nm || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout deployment $nm, $i"
      continue
    fi

    local nm=rancher-monitoring-prometheus-adapter
    wait_rollout $NS deployment $nm || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout deployment $nm, $i"
      continue
    fi

    local nm=rancher-monitoring-kube-state-metrics
    wait_rollout $NS deployment $nm || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout deployment $nm, $i"
      continue
    fi

    local nm=rancher-monitoring-grafana
    wait_rollout $NS deployment $nm || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout deployment $nm, $i"
      continue
    fi

    local nm=prometheus-rancher-monitoring-prometheus
    wait_rollout $NS statefulset $nm || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout statefulset $nm, $i"
      continue
    fi

    # alertmanager may be disabled, do not wait
    #local nm=alertmanager-rancher-monitoring-alertmanager
    #wait_rollout $NS statefulset $nm || EXIT_CODE=$?
    #if [ $EXIT_CODE != 0 ]; then
    #  echo "continue waiting rollout statefulset $nm, $i"
    #  continue
    #fi

    local nm=rancher-monitoring-prometheus-node-exporter
    wait_rollout $NS daemonset $nm || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "continue waiting rollout daemonset $nm, $i"
      continue
    fi

    break
  done

  if [ $EXIT_CODE != 0 ]; then
    echo "fail to wait rollout monitoring"
    return $EXIT_CODE
  fi

  echo "success to wait rollout monitoring"
  return 0
}

convert_logging_audit_to_addon() {
  echo "Logging Event Audit: start to convert logging from managedchart to addon"
  local chart_name="rancher-logging"
  local namespace="cattle-logging-system"
  local cnt=0

  # check if logging addon is there
  cnt=$(kubectl get addon.harvesterhci.io -n "$namespace" "$chart_name" --no-headers | wc -l)
  if [ "$cnt" -gt 0 ]; then
    echo "logging has already been addon"
    return 0
  fi

  local src_file=/usr/local/share/addons/logging_addon.yaml
  if [ ! -f $src_file ]; then
    echo "there is no $src_file, cannot convert"
    return 0
  fi

  # check if logging managedchart is there
  cnt=$(kubectl get managedchart -n fleet-local $chart_name --no-headers | wc -l)

  mkdir -p $UPGRADE_TMP_DIR/logging
  local saved_config_file=$UPGRADE_TMP_DIR/logging/saved_config.yaml

  # save config
  local enabled=false
  local pod_cnt=0

  if [ "$cnt" -gt 0 ]; then
    enabled=true
    kubectl get managedchart -n fleet-local "$chart_name" -o yaml | yq -e ".spec.values" > "$saved_config_file"
    echo "saved managedchart config:"
    cat "$saved_config_file"

    # eventTailer needs to be deleted separately, as it is not part of the managedchart in v1.1.2
    local et_cnt=$(kubectl get eventtailers.logging-extensions.banzaicloud.io harvester-default --no-headers | wc -l)
    if [ "$et_cnt" -gt 0 ]; then
      local et_spec=$(kubectl get eventtailers.logging-extensions.banzaicloud.io harvester-default -oyaml | yq -e ".spec")
      echo "saved eventTailer config:"
      echo "$et_spec"
      # merge eventTailer config into saved_config_file
      values="$et_spec" yq -i e '.eventTailer=env(values)' "$saved_config_file"
      echo "delete eventtailer harvester-default"
      kubectl delete eventtailers.logging-extensions.banzaicloud.io harvester-default
    else
      echo "eventTailer harvester-default is not found"
    fi

    pod_cnt=$(kubectl get pods -n "$namespace" --no-headers | wc -l)
    echo "there are $pod_cnt pods in $namespace will be replaced"
    echo "delete managedchart $chart_name"
    kubectl delete managedchart -n fleet-local "$chart_name"
    sleep 20

    # check until all pods are deleted
    while [ true ]; do
      pod_cnt=$(kubectl get pods -n "$namespace" --no-headers | wc -l)
      if [ "$pod_cnt" -gt 0 ]; then
        date; echo "there are still $pod_cnt pods in $namespace to be deleted"
        sleep 5
      else
        echo "all pods in $namespace are deleted"
        break
      fi
    done
  fi

  # create tmp addon yaml file
  local target_file="$UPGRADE_TMP_DIR/logging/logging_addon_tmp.yaml"
  rm -f $target_file
  cp $src_file $target_file

  # replace with saved&merged config
  if [ -f "$saved_config_file" ]; then
    echo "replace addon valuesContent with saved config"
    values=$(cat "$saved_config_file") yq -i e '.spec.valuesContent=strenv(values)' $target_file
  fi

  if [ $enabled = true ]; then
    echo "enable logging addon"
    yq -i e '.spec.enabled = true' $target_file
  else
    echo "disable logging addon"
    yq -i e '.spec.enabled = false' $target_file
  fi

  # make sure the version is aligned with harvester-installer
  if [ ! -z $REPO_LOGGING_CHART_VERSION ]; then
    echo "replace addon chart version to target: $REPO_LOGGING_CHART_VERSION"
    values="$REPO_LOGGING_CHART_VERSION" yq -i e '.spec.version=strenv(values)' $target_file
  fi

  echo "final logging addon yaml file"
  cat $target_file

  kubectl apply -f $target_file
  rm -f $target_file
  rm -f $saved_config_file
  kubectl get addon.harvesterhci.io -A || echo "fail to get addon"

  if [ $enabled = true ]; then
    # wait for addon to be applied
    echo "wait for rollout of logging and audit"
    sleep 30
    # loop wait for at most 6 minutes (36 * 10s)
    loop_wait_rollout_logging_audit 36

    pod_cnt=$(kubectl get pods -n "$namespace" --no-headers | wc -l) || echo "fail to get pods in $namespace"
    echo "there are $pod_cnt pods in $namespace after upgrade"
  fi

  echo "Logging Event Audit: finish converting"
}

convert_monitoring_to_addon() {
  echo "Monitoring: start to convert monitoring from managedchart to addon"
  local chart_name="rancher-monitoring"
  local namespace="cattle-monitoring-system"
  local cnt=0

  # check if monitoring addon is there
  cnt=$(kubectl get addon.harvesterhci.io -n "$namespace" "$chart_name" --no-headers | wc -l)
  if [ "$cnt" -gt 0 ]; then
    echo "monitoring has already been addon"
    return 0
  fi

  local src_file=/usr/local/share/addons/monitoring_addon.yaml
  if [ ! -f $src_file ]; then
    echo "there is no $src_file, cannot convert"
    return 0
  fi

  # check if monitoring managedchart is there
  cnt=$(kubectl get managedchart -n fleet-local "$chart_name" --no-headers | wc -l)

  # save config
  local saved_config="null"
  local enabled=false
  local pod_cnt=0

  local pvname=""
  local removeClaimRef=false

  if [ "$cnt" -gt 0 ]; then
    # in v1.1.2, the monitoring is enabled
    enabled=true
    saved_config=$(kubectl get managedchart -n fleet-local "$chart_name" -o yaml | yq -e ".spec.values")
    echo "saved managedchart config:"
    echo "$saved_config"

    # try to retain grafana PV
    # when grafana pvc is not enabled, pvc is not existing, kubectl will return 1
    # pv name is like pvc-e2983b67-035a-485e-bb44-fbf6309389cb
    pvname=$(kubectl get pvc -n "$namespace" rancher-monitoring-grafana -o jsonpath={".spec.volumeName"}) || true
    removeClaimRef=false
    if [ ! -z $pvname ]; then
      local rp=$(kubectl get pv $pvname -o jsonpath={".spec.persistentVolumeReclaimPolicy"}) || true
      if [ ! -z $rp ]; then
        if [ ! $rp = "Retain" ]; then
          echo "patch ReclaimPolicy of grafana pv $pvname from $rp to Retain"
          kubectl patch pv $pvname -p "{\"spec\":{\"persistentVolumeReclaimPolicy\":\"Retain\"}}"
        fi
        removeClaimRef=true
      else
        echo "did not find grafana pv, or spec.persistentVolumeReclaimPolicy is empty"
      fi
    else
      echo "did not find grafana pvc"
    fi

    pod_cnt=$(kubectl get pods -n "$namespace" --no-headers | wc -l)
    echo "there are $pod_cnt pods in $namespace will be replaced"
    echo "delete managedchart $chart_name"
    kubectl delete managedchart -n fleet-local "$chart_name"

    # check until all pods are deleted
    while [ true ]; do
      pod_cnt=$(kubectl get pods -n "$namespace" --no-headers | wc -l)
      if [ "$pod_cnt" -gt 0 ]; then
        date; echo "there are still $pod_cnt pods in $namespace to be deleted"
        sleep 5
      else
        echo "all pods in $namespace are deleted"
        break
      fi
    done

    # remove pv CliamRef when it is status 'Released', and after that, it becomes 'Available'
    if [ $removeClaimRef = true ]; then
      local phase=$(kubectl get pv $pvname -o jsonpath="{.status.phase}") || true
      if [ $phase = "Released" ]; then
        echo "patch ClaimRef of grafana pv, current status is Released"
        echo "current ClaimRef is"
        kubectl get pv $pvname -o jsonpath={".spec.claimRef"} && echo ""
        # remove uid is enough
        kubectl patch pv $pvname --type json -p '[{"op": "remove", "path": "/spec/claimRef/uid"}]'
        sleep 2
        echo "after patch, ClaimRef is"
        kubectl get pv $pvname -o jsonpath={".spec.claimRef"} && echo ""
        echo "the new status is"
        kubectl get pv $pvname -o jsonpath="{.status.phase}" && echo ""
      elif [ $phase = "Available" ]; then
        echo "grafana pv status has already been Available, do not patch ClaimRef"
        echo "current ClaimRef is"
        kubectl get pv $pvname -o jsonpath={".spec.claimRef"} && echo ""
      else
        # CAN NOT simply remove the ClaimRef
        echo "the status.phase of grafana pv is $phase, CAN NOT patch ClaimRef"
        removeClaimRef=false
      fi
    fi
  fi

  # create tmp addon yaml file
  mkdir -p $UPGRADE_TMP_DIR/monitoring
  target_file="$UPGRADE_TMP_DIR/monitoring/monitoring_addon_tmp.yaml"
  rm -f $target_file
  cp $src_file $target_file

  # replace with saved config
  if [ "$saved_config" != "null" ]; then
    echo "replace addon valuesContent with saved config"
    values="$saved_config" yq -i e '.spec.valuesContent=strenv(values)' $target_file
  else
    # replace the replace_with_vip field in add yaml file
    # use sed to replace sub-string in the valuesContent, it is not easily done in yq
    detect_harvester_vip
    if [ -n "$HARVESTER_VIP" ]; then
      echo "replace the 'replace_with_vip' with $HARVESTER_VIP in $target_file"
      sed -i "s/replace_with_vip/$HARVESTER_VIP/" $target_file
    else
      echo "detect vip fail, do not replace the 'replace_with_vip' in $target_file"
    fi
  fi

  if [ $enabled = true ]; then
    echo "enable monitoring addon"
    yq -i e '.spec.enabled = true' $target_file
  else
    echo "disable monitoring addon"
    yq -i e '.spec.enabled = false' $target_file
  fi

  # make sure the version is aligned with harvester-installer
  if [ ! -z $REPO_MONITORING_CHART_VERSION ]; then
    echo "replace addon chart version to target: $REPO_MONITORING_CHART_VERSION"
    values="$REPO_MONITORING_CHART_VERSION" yq -i e '.spec.version=strenv(values)' $target_file
  fi

  values="$UPGRADE_PREVIOUS_VERSION" yq -i e '.metadata.annotations["harvesterhci.io/upgrade-previous-version"]=strenv(values)' $target_file

  if [ $removeClaimRef = true ]; then
    # annotation pv name for future usage on controller side, when addon is disabled, will also try to keep grafana pv
    echo "annotate addon with grafana pv name"
    values="$pvname" yq -i e '.metadata.annotations["harvesterhci.io/grafana-pv-name"]=strenv(values)' $target_file
  fi

  echo "final monitoring addon yaml file"
  cat $target_file

  kubectl apply -f $target_file
  rm -f $target_file
  kubectl get addon.harvesterhci.io -A || echo "fail to get addon"

  if [ $enabled = true ]; then
    echo "wait for rollout of monitoring"
    sleep 30
    # loop wait for at most 6 minutes (36 * 10s)
    loop_wait_rollout_monitoring 36

    pod_cnt=$(kubectl get pods -n "$namespace" --no-headers | wc -l)
    echo "there are $pod_cnt pods in $namespace after upgrade"
  fi

  echo "Monitoring: finish converting"
}

upgrade_logging_event_audit_new_feature() {
  # from v1.0.3 to v1.1.0, logging, event, audit are enabled by default
  echo "Logging Event Audit: start to upgrade manifest"

  # prepare a malformed yaml file, make sure it is effectively replaced
  echo "to-be-replaced-file" > rancher-logging.yaml

  # reuse framework to generate yaml file
  upgrade_managed_chart_from_version $UPGRADE_PREVIOUS_VERSION rancher-logging rancher-logging.yaml

  echo "Apply resource file of logging and audit"

  kubectl apply -f ./rancher-logging.yaml

  # wait for managedchart to be applied
  sleep 50

  echo "Wait for rollout of logging and audit"
  # loop wait for at most 6 minutes (36 * 10s)
  loop_wait_rollout_logging_audit 36

  # due to error: unable to recognize "./rancher-logging.yaml": no matches for kind "EventTailer" in version "logging-extensions.banzaicloud.io/v1alpha1"
  # the eventtailer needs to be deployed after the managedcharts are deployed

  # prepare a malformed yaml file, make sure it is effectively replaced
  echo "to-be-replaced-file" > rancher-logging.yaml

  # reuse frame work to generate yaml file
  # rancher-logging_event-extension is reusing chart rancher-logging, but as an extension for event
  upgrade_managed_chart_from_version $UPGRADE_PREVIOUS_VERSION rancher-logging_event-extension rancher-logging.yaml

  echo "Apply resource file of event"

  kubectl apply -f ./rancher-logging.yaml

  # wait few seconds
  sleep 20

  echo "Wait for rollout of event"
  # loop wait for at most 3 minutes (18 * 10s)
  loop_wait_rollout_event 18

  echo "Logging Event Audit: finish upgrading manifest"
}

upgrade_logging_event_audit() {
  # from v1.0.3, logging, event, audit are enabled by default
  echo "The current version is $UPGRADE_PREVIOUS_VERSION, will check Logging Event Audit upgrade manifest option"

  if [[ $(is_formal_release $UPGRADE_PREVIOUS_VERSION) = "true" ]]; then
    if [[ "$UPGRADE_PREVIOUS_VERSION" = "v1.0.3" ]]; then
      upgrade_logging_event_audit_new_feature
    elif [[ "$UPGRADE_PREVIOUS_VERSION" > "v1.1.1" ]] && [[ "$UPGRADE_PREVIOUS_VERSION" < "v1.2.0" ]]; then
      # from v1.1.2 to potential v1.1.3,v1.1.4 ...
      upgrade_managedchart_logging_crd
      convert_logging_audit_to_addon
    elif [[ "$UPGRADE_PREVIOUS_VERSION" = "v1.2.0" ]] || [[ "$UPGRADE_PREVIOUS_VERSION" > "v1.2.0" ]]; then
      # from v1.2.0, only crd here, rancher-logging is upgraded in addons
      upgrade_managedchart_logging_crd
    else
      echo "Logging Event Audit: nothing to do in $UPGRADE_PREVIOUS_VERSION"
    fi
  else
    # for version like v1.2.0/v1.1.2 master-head to master-head/rc/formal release, try convert anyway
    upgrade_managedchart_logging_crd
    convert_logging_audit_to_addon
  fi
}

apply_extra_manifests()
{
  echo "Applying extra manifests"

  shopt -s nullglob

  # from v1.1.2, extra manifests are controlled by version
  # related files should be put under specific version path, e.g. extra_manifests/v1.1.3/some_manifest.yaml
  if [[ $(is_formal_release $UPGRADE_PREVIOUS_VERSION) = "true" ]] && [[ "$UPGRADE_PREVIOUS_VERSION" < "v1.1.2" ]]; then
    local rootpath="/usr/local/share/extra_manifests/untilv1.1.1"
  else
    local rootpath="/usr/local/share/extra_manifests/$UPGRADE_PREVIOUS_VERSION"
  fi

  if [ -d "$rootpath" ]; then
    for manifest in $rootpath/*.yaml; do
      echo "Apply $manifest"
      kubectl apply -f $manifest
    done
  else
    echo "No extra manifests in $rootpath to apply"
  fi

  shopt -u nullglob
}

upgrade_managed_chart_from_version() {
  version=$1
  chart_name=$2
  chart_manifest=$3

  if [ -e "/usr/local/share/migrations/managed_charts/${version}.sh" ]; then
    /usr/local/share/migrations/managed_charts/${version}.sh $chart_name $chart_manifest
  fi
}

pause_managed_chart() {
  chart=$1
  do_pause=$2

  mkdir -p $UPGRADE_TMP_DIR/pause
  cd $UPGRADE_TMP_DIR/pause
  cat >${chart}.yaml <<EOF
spec:
  paused: $do_pause
EOF
  kubectl patch managedcharts.management.cattle.io $chart -n fleet-local --patch-file ./${chart}.yaml --type merge
}

pause_all_charts() {
  local charts="harvester harvester-crd rancher-monitoring-crd rancher-logging-crd"
  for chart in $charts; do
    pause_managed_chart $chart "true"
  done

  # those charts may have been converted to addon, check if they are there first
  charts="rancher-monitoring rancher-logging"
  for chart in $charts; do
    local cnt=$(kubectl get managedchart -n fleet-local "$chart" --no-headers | wc -l)
    if [ "$cnt" -gt 0 ]; then
      pause_managed_chart $chart "true"
    fi
  done
}

skip_restart_rancher_system_agent() {
  # to prevent rke2-server/agent from restarting during the rancher upgrade.
  # by adding an env var to temporarily make rancher-system-agent on each node skip restarting rke2-server/agent.
  # issue link: https://github.com/rancher/rancher/issues/41965

  # only versions before v1.2.0 that upgrading to v1.2.0 need this workaround
  if [[ ! "${UPGRADE_PREVIOUS_VERSION%%-rc*}" < "v1.2.0" ]]; then
    echo "Only versions before v1.2.0 need this patch."
    return
  fi

  plan_manifest="$(mktemp --suffix=.yaml)"
  plan_name="$HARVESTER_UPGRADE_NAME"-skip-restart-rancher-system-agent
  plan_version="$(openssl rand -hex 4)"

  cat > "$plan_manifest" <<EOF
apiVersion: upgrade.cattle.io/v1
kind: Plan
metadata:
  name: $plan_name
  namespace: cattle-system
spec:
  concurrency: 10
  nodeSelector:
    matchLabels:
      harvesterhci.io/managed: "true"
  serviceAccountName: system-upgrade-controller
  tolerations:
  - operator: "Exists"
  upgrade:
    image: registry.suse.com/bci/bci-base:15.4
    command:
    - chroot
    - /host
    args:
    - sh
    - -c
    - set -x && mkdir -p /run/systemd/system/rancher-system-agent.service.d && echo -e '[Service]\nEnvironmentFile=-/run/systemd/system/rancher-system-agent.service.d/10-harvester-upgrade.env' | tee /run/systemd/system/rancher-system-agent.service.d/override.conf && echo 'INSTALL_RKE2_SKIP_ENABLE=true' | tee /run/systemd/system/rancher-system-agent.service.d/10-harvester-upgrade.env && systemctl daemon-reload && systemctl restart rancher-system-agent.service
  version: $plan_version
EOF

  echo "Creating plan $plan_name to make rancher-system-agent temporarily skip restarting RKE2 server..."
  kubectl create -f "$plan_manifest"

  # Wait for all nodes complete
  while [ true ]; do
    plan_label="plan.upgrade.cattle.io/$plan_name"
    plan_latest_version=$(kubectl get plans.upgrade.cattle.io "$plan_name" -n cattle-system -ojsonpath="{.status.latestVersion}")

    if [ "$plan_latest_version" = "$plan_version" ]; then
      plan_latest_hash=$(kubectl get plans.upgrade.cattle.io "$plan_name" -n cattle-system -ojsonpath="{.status.latestHash}")
      total_nodes_count=$(kubectl get nodes -o json | jq '.items | length')
      complete_nodes_count=$(kubectl get nodes --selector="plan.upgrade.cattle.io/$plan_name=$plan_latest_hash" -o json | jq '.items | length')

      if [ "$total_nodes_count" = "$complete_nodes_count" ]; then
        echo "Plan $plan_name completes."
        break
      fi
    fi

    echo "Waiting for plan $plan_name to complete..."
    sleep 10
  done

  echo "Deleting plan $plan_name..."
  kubectl delete plans.upgrade.cattle.io "$plan_name" -n cattle-system
  rm -f "$plan_manifest"
}

# NOTE: review in each release, add corresponding process
upgrade_addon_rancher_monitoring()
{
  echo "upgrade addon rancher_monitoring"
  if [[ $(is_formal_release $UPGRADE_PREVIOUS_VERSION) = "true" ]]; then
    if [[ "$UPGRADE_PREVIOUS_VERSION" = "v1.2.0" ]] || [[ "$UPGRADE_PREVIOUS_VERSION" > "v1.2.0" ]]; then
      echo ".spec.valuesContent has dynamic fields, cannot merge simply, review in each release"
    fi
  else
    # the addon may be existing in v1.2.0 master-head release and the chart version is bumped, then the addon is upgraded to new version
    upgrade_addon_try_patch_version_only "rancher-monitoring" "cattle-monitoring-system" $REPO_MONITORING_CHART_VERSION
  fi
}

# NOTE: review in each release, add corresponding process
upgrade_addon_rancher_logging()
{
  echo "upgrade addon rancher_logging"
  if [[ $(is_formal_release $UPGRADE_PREVIOUS_VERSION) = "true" ]]; then
    if [[ "$UPGRADE_PREVIOUS_VERSION" = "v1.2.0" ]] || [[ "$UPGRADE_PREVIOUS_VERSION" > "v1.2.0" ]]; then
      echo ".spec.valuesContent has dynamic fields, cannot merge simply, review in each release"
    fi
  else
    # the addon may be existing in v1.2.0 master-head release and the chart version is bumped, then the addon is upgraded to new version
    upgrade_addon_try_patch_version_only "rancher-logging" "cattle-logging-system" $REPO_LOGGING_CHART_VERSION
  fi
}

upgrade_addons()
{
  wait_for_addons_crd
  addons="vm-import-controller pcidevices-controller harvester-seeder"
  for addon in $addons; do
    upgrade_addon $addon "harvester-system"
  done

  # those 2 addons are not simply installed, they are converted from managedchart, above code is no reused
  # from v1.2.0, they are upgraded per following
  upgrade_addon_rancher_monitoring
  upgrade_addon_rancher_logging
}

reuse_vlan_cn() {
  [[ $UPGRADE_PREVIOUS_VERSION != "v1.0.3" ]] && return

  # delete finalizer
  kubectl get clusternetwork vlan -o yaml | yq '.metadata.finalizers = []' | kubectl apply -f -
}

wait_repo
detect_repo
detect_upgrade
check_version
pre_upgrade_manifest
pause_all_charts
skip_restart_rancher_system_agent
upgrade_rancher
update_local_rke_state_secret
upgrade_harvester_cluster_repo
upgrade_network
upgrade_harvester
wait_longhorn_upgrade
reuse_vlan_cn
upgrade_monitoring
upgrade_logging_event_audit
apply_extra_manifests
upgrade_addons
# wait fleet bundles upto 90 seconds
wait_for_fleet_bundles 9
