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
    current_ready_clusters=$(echo "$current_chart" | yq e '.status.display.readyClusters' -)
    echo "Current version: $current_version, Current ready clusters: $current_ready_clusters, Current state: $current_state, Current generation: $current_observed_generation"

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
  echo "wait helm release $namespace $release_name $chart $app_version $status"
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
  echo "wait rollout -n $namespace $resource_type $name"

  kubectl rollout status --watch=true -n $namespace $resource_type $name
}

wait_rollout_with_loop() {
  local namespace=$1
  local resource_type=$2
  local name=$3
  echo "wait rollout -n $namespace $resource_type $name"

  # if resource is new and not created yet, need to wait, otherwise, it will return right now and no error returned
  while [ true ]; do
    local obj=$(kubectl get -n $namespace $resource_type $name)
    if [ -z "$obj" ]; then
      echo "resource was not created, continue"
      sleep 2
    else
      break
    fi
  done

  kubectl rollout status --watch=true -n $namespace $resource_type $name
}

wait_cluster_local_and_fleet() {
  wait_new_fileds_in_cluster_fleet_crd
  wait_new_fileds_in_fleet_controller_configmap
  restore_fleet_controller_configmap
  wait_cluster_local_is_imported
  wait_fleet_agent_is_redeployed
  wait_cluster_local_is_ready
}

debug_cluster_local_and_fleet() {
  # for better debugging
  echo "cluster.fleet local status"
  kubectl get cluster.fleet -n fleet-local local -ojsonpath="{.status}" || echo "cluster.fleet local is not found"

  echo ""
  echo ""
  echo "fleet-controller pods and creationTimestamp"
  kubectl -n cattle-fleet-system get pods -l "app=fleet-controller" -owide || echo "fleet-controller pods are not found"
  kubectl -n cattle-fleet-system get pods -l "app=fleet-controller" -ojsonpath="{.items[].metadata.creationTimestamp}" || echo "fleet-controller pods are not found"

  echo ""
  echo "fleet-agent pods and creationTimestamp"
  kubectl -n cattle-fleet-local-system get pods -l "app=fleet-agent" -owide || echo "fleet-agent pods are not found"
  kubectl -n cattle-fleet-local-system get pods -l "app=fleet-agent" -ojsonpath="{.items[].metadata.creationTimestamp}" || echo "fleet-agent pods are not found"
  echo ""
}

wait_cluster_local_is_imported() {
  echo "wait until cluster.fleet local is Imported after fleet-controller is upgraded"
  # take a look the possible ready pods
  kubectl -n cattle-fleet-system get pods -l "app=fleet-controller" || echo "fleet-controller pods are not found"
  local oldstamp=$(get_fleet_controller_timestamp)
  while [ true ]; do
    local tempupdatetime=$(kubectl get cluster.fleet -n fleet-local local -oyaml | yq -e '.status.conditions | map(select(.type=="Imported")) | .[0] | .lastUpdateTime')
    local tempstatus=$(kubectl get cluster.fleet -n fleet-local local -oyaml | yq -e '.status.conditions | map(select(.type=="Imported")) | .[0] | .status')
    if [ -z "$tempupdatetime" ]; then
      echo "cluster.fleet -n fleet-local local condition Imported is not found, continue"
      sleep 2
    else
      local tempstamp=$(date -u -d "$tempupdatetime" +'%s')
      if [ "$tempstamp" -ge "$oldstamp" ]; then
        echo "cluster.fleet -n fleet-local local condition Imported is updated, $tempstamp >= $oldstamp, status is $tempstatus"
        break
      else
        echo "cluster.fleet -n fleet-local local condition Imported is not updated, $tempstamp < $oldstamp, status is $tempstatus, continue"
        sleep 2
      fi
    fi
    unset tempupdatetime
    unset tempstatus
    unset tempstamp
  done
  echo "cluster.fleet local is Imported"
  debug_cluster_local_and_fleet
}

# wait at most 60 seconds
wait_cluster_local_is_ready() {
  echo "wait until cluster.fleet local is Ready after fleet-controller is upgraded"
  local i=0
  while [[ "$i" -lt 30 ]]; do
    local tempstatus=$(kubectl get cluster.fleet -n fleet-local local -oyaml | yq -e '.status.conditions | map(select(.type=="Ready")) | .[0] | .status')
    if [ -z "$tempstatus" ]; then
      echo "cluster.fleet -n fleet-local local condition Ready is not found, continue"
      sleep 2
      i=$((i + 1))
    else
      if [ "$tempstatus" = "True" ]; then
        echo "cluster.fleet -n fleet-local local condition Ready is true"
        debug_cluster_local_and_fleet
        return 0
      else
        echo "cluster.fleet -n fleet-local local condition Ready is false, continue"
        sleep 2
        i=$((i + 1))
      fi
    fi
    unset tempstatus
  done
  echo "cluster.fleet local is not Ready, skip waiting"
  debug_cluster_local_and_fleet
}

wait_fleet_agent_is_redeployed() {
  echo "wait until fleet-agent is redeployed after fleet-controller is upgraded"
  # take a look the possible ready pods
  kubectl -n cattle-fleet-system get pods -l "app=fleet-controller" || echo "fleet-controller pods are not found"
  local oldstamp=$(get_fleet_controller_timestamp)
  while [ true ]; do
    local tempcreatetime=$(kubectl -n cattle-fleet-local-system get pods -l "app=fleet-agent" -o jsonpath='{range .items[*]}{.status.containerStatuses[*].ready.true}{.metadata.creationTimestamp}{ "\n"}{end}')
    if [ -z "$tempcreatetime" ]; then
      echo "fleet-agent pod is not found, continue"
      sleep 2
    else
      local tempstamp=$(date -u -d "$tempcreatetime" +'%s')
      if [ "$tempstamp" -ge "$oldstamp" ]; then
        echo "fleet-agent is new, $tempstamp >= $oldstamp"
        break
      else
        echo "fleet-agent is old, $tempstamp < $oldstamp, continue"
        sleep 2
      fi
    fi
    unset tempcreatetime
    unset tempstamp
  done
  echo "fleet-agent is redeployed"
  kubectl -n cattle-fleet-local-system get pods
  debug_cluster_local_and_fleet
  # let new fleet-agent run for some time
  sleep 5
}

# only pick the ready pod
get_fleet_controller_timestamp() {
  while [ true ]; do
    local tempcreatetime=$(kubectl -n cattle-fleet-system get pods -l "app=fleet-controller" -o jsonpath='{range .items[*]}{.status.containerStatuses[*].ready.true}{.metadata.creationTimestamp}{ "\n"}{end}')
    if [ -z "$tempcreatetime" ]; then
      # did not get, continue
      sleep 1
    else
      # when unlucky, there are >1 pods are ready, the return is like `2024-10-24T12:57:54Z\n2024-10-24T12:57:54Z\n2024-10-24T12:57:54Z`
      # take the first one
      local firsttime=${tempcreatetime:0:20}
      local tempstamp=$(date -u -d "$firsttime" +'%s')
      echo "$tempstamp"
      break
    fi
  done
}

wait_managedchart_ready() {
  local chart=$1
  if [[ -z $chart ]]; then
    echo "no target managedchart, skip wait"
  fi;

  # wait at most 60 seconds
  echo "wait managedchart $chart to be ready"
  local i=0
  while [[ "$i" -lt 30 ]]; do
    local ready=$(kubectl get managedchart -n fleet-local $chart -ojsonpath="{.status.summary.ready}")
    if [ -z "$ready" ]; then
      echo "chart is not found, continue"
      sleep 2
      i=$((i + 1))
    else
      if [ "$ready" = "0" ]; then
        echo " ready is 0, continue"
        sleep 2
        i=$((i + 1))
      else
        echo " ready is $ready"
        break
      fi
    fi
    unset ready
  done
}

wait_new_fileds_in_cluster_fleet_crd() {
  while [ true ]; do
    local crd=$(kubectl get crd clusters.fleet.cattle.io -o json)
    local newfield="agentTLSMode"

    if echo "$crd" | jq -e ".spec.versions[].schema.openAPIV3Schema.properties.status.properties | has(\"$newfield\")" > /dev/null; then
      echo "new field agentTLSMode is found in clusters.fleet.cattle.io crd"
      break
    else
      echo "wait for new field agentTLSMode in clusters.fleet.cattle.io crd"
      sleep 2
    fi

    unset crd
    unset newfield
  done
}

wait_new_fileds_in_fleet_controller_configmap() {
  while [ true ]; do
    local configmap=$(kubectl get configmap fleet-controller -n cattle-fleet-system -ojsonpath="{.data.config}")
    local newfield="agentTLSMode"

    if echo "$configmap" | yq -e ". | has(\"$newfield\")" > /dev/null; then
      echo "new field agentTLSMode is found in cattle-fleet-system/fleet-controller configmap"
      break
    else
      echo "wait for new field agentTLSMode in cattle-fleet-system/fleet-controller configmap"
      sleep 2
    fi

    unset configmap
    unset newfield
  done
}

# refer issue https://github.com/harvester/harvester/issues/6851
save_fleet_controller_configmap()
{
  local name=fleet-controller
  local namespace=cattle-fleet-system
  local valuesfile="configmap-values-temp.yaml"
  rm -f $valuesfile
  local EXIT_CODE=0
  kubectl get configmap -n $namespace $name -ojsonpath="{.data.config}" > $valuesfile || EXIT_CODE=1

  if [[ "$EXIT_CODE" != 0 ]]; then
    echo "config field on configmap $name -n $namespace is empty, skip saving"
    # unset var
    FLEET_APISERVERURL=""
    FLEET_APISERVERCA=""
    return 0
  fi

  # local var will escape yq field none-exsting error
  local apiServerURL=$(yq -e '.apiServerURL' $valuesfile)
  local apiServerCA=$(yq -e '.apiServerCA' $valuesfile)
  FLEET_APISERVERURL=$apiServerURL
  FLEET_APISERVERCA=$apiServerCA
  echo "saved apiServerURL $FLEET_APISERVERURL"
  echo "saved apiServerCA $FLEET_APISERVERCA"
  rm -f $valuesfile
}

# refer issue https://github.com/harvester/harvester/issues/6851
restore_fleet_controller_configmap()
{
  if [[ -z "$FLEET_APISERVERURL" && -z "$FLEET_APISERVERCA" ]]; then
    echo "both apiServerURL and apiServerCA saved values are empty, skip restoring"
    return 0
  fi

  local name=fleet-controller
  local namespace=cattle-fleet-system
  local valuesfile="configmap-values-temp.yaml"
  rm -f $valuesfile
  local EXIT_CODE=0
  kubectl get configmap -n $namespace $name -ojsonpath="{.data.config}" > $valuesfile || EXIT_CODE=1

  if [[ "$EXIT_CODE" != 0 ]]; then
    echo "config field on configmap $name -n $namespace is empty or configmap is not existing, skip restoring"
    return 0
  fi

  # local var will escape yq field none-exsting error
  local apiServerURL=$(yq -e '.apiServerURL' $valuesfile)
  local apiServerCA=$(yq -e '.apiServerCA' $valuesfile)
  local patchURL=false
  local patchCA=false
  if [[ ! -z "$FLEET_APISERVERURL" && "$apiServerURL" != "$FLEET_APISERVERURL" ]]; then
    patchURL=true
    echo "restore apiServerURL from $apiServerURL to $FLEET_APISERVERURL"
    PATCH_URL=$FLEET_APISERVERURL yq e '.apiServerURL = strenv(PATCH_URL)' -i $valuesfile
  fi

  if [[ ! -z "$FLEET_APISERVERCA" && "$apiServerCA" != "$FLEET_APISERVERCA" ]]; then
    patchCA=true
    echo "restore apiServerCA from $apiServerCA to $FLEET_APISERVERCA"
    PATCH_CA=$FLEET_APISERVERCA yq e '.apiServerCA = strenv(PATCH_CA)' -i $valuesfile
  fi

  if [[ "$patchURL" == false &&  "$patchCA" == false ]]; then
    echo "apiServerURL $apiServerURL and apiServerCA $apiServerCA have already been same with saved values"
    return 0
  fi

  # add 4 spaces to each line
  sed -i -e 's/^/    /' $valuesfile
  local newvalues=$(<$valuesfile)
  rm -f $valuesfile

  local patchfile="configmap-patch-temp.yaml"
  rm -f $patchfile

cat > $patchfile <<EOF
data:
  config: |
$newvalues
EOF

  echo "the configmap will be restored to"
  cat ./$patchfile

  kubectl patch configmap -n $namespace $name --patch-file ./$patchfile --type merge
  rm -f ./$patchfile
  echo "sleep 20s for fleet-controller to work on new configmap"
  sleep 20
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

  echo "Waiting for KubeVirt to upgrade to $version..."
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

  lm_repo=$(helm get values harvester -n harvester-system -a -o json | jq -r .longhorn.image.longhorn.manager.repository)
  lm_tag=$(helm get values harvester -n harvester-system -a -o json | jq -r .longhorn.image.longhorn.manager.tag)
  lm_image="${lm_repo}:${lm_tag}"
  local node_count=$(kubectl get nodes --selector=harvesterhci.io/managed=true,node-role.harvesterhci.io/witness!=true -o json | jq -r '.items | length')

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

wait_longhorn_instance_manager_aio() {
  local node_count=$(kubectl get nodes --selector=harvesterhci.io/managed=true,node-role.harvesterhci.io/witness!=true -o json | jq -r '.items | length')
  if [ $node_count -lt 2 ]; then
    echo "Skip waiting instance-manager (aio), node count: $node_count"
    return
  fi

  im_repo=$(helm get values harvester -n harvester-system -a -o json | jq -r .longhorn.image.longhorn.instanceManager.repository)
  im_tag=$(helm get values harvester -n harvester-system -a -o json | jq -r .longhorn.image.longhorn.instanceManager.tag)
  im_image="${im_repo}:${im_tag}"

  # Get instance-manager-image chechsum
  # reference: https://github.com/longhorn/longhorn-manager/blob/2ec649c35486d782731982c9dff1db41c9031c99/types/types.go#L429
  im_image_checksum="${im_image/\//-}" # replace / with -
  im_image_checksum="${im_image_checksum/:/-}" # replace : with -
  im_image_checksum=$(echo -n "$im_image_checksum" | openssl dgst -sha512 | awk '{print $2}')
  im_image_checksum="imi-${im_image_checksum:0:8}"

  # Wait for instance-manager (aio) pods upgraded to new version first.
  kubectl get nodes.longhorn.io -n longhorn-system -o json | jq -r '.items[].metadata.name' | while read -r node; do
    echo "Checking instance-manager (aio) pod on node $node..."
    check_instance_manager $node $im_image $im_image_checksum "v1"

    v2EngineEnabled=$(kubectl get settings.harvesterhci.io longhorn-v2-data-engine-enabled -o yaml | yq e '.value' -)
    if [ "$v2EngineEnabled" = "true" ]; then
      # check instance-manager (aio) is running with v2 engine
      check_instance_manager $node $im_image $im_image_checksum "v2"
    fi
  done
}

check_instance_manager() {
  local node="$1"
  local im_image="$2"
  local im_image_checksum="$3"
  local data_engine="$4"

  while [ true ]; do
    im_count=$(kubectl get instancemanager.longhorn.io --selector=longhorn.io/node=$node,longhorn.io/instance-manager-type=aio,longhorn.io/data-engine=$data_engine,longhorn.io/instance-manager-image=$im_image_checksum -n longhorn-system -o json | jq -r '.items | length')
    if [ "$im_count" != "1" ]; then
      echo "instance-manager (aio)($data_engine) (image=$im_image) count is not 1 on node $node, will retry..."
      sleep 5
      continue
    fi

    im_status=$(kubectl get instancemanager.longhorn.io --selector=longhorn.io/node=$node,longhorn.io/instance-manager-type=aio,longhorn.io/data-engine=$data_engine,longhorn.io/instance-manager-image=$im_image_checksum -n longhorn-system -o json | jq -r '.items[0].status.currentState')
    if [ "$im_status" != "running" ]; then
      echo "instance-manager (aio)($data_engine) (image=$im_image) state is not running on node $node, will retry..."
      sleep 5
      continue
    fi

    echo "Checking instance-manager (aio)($data_engine) (image=$im_image) on node $node OK."
    break
  done
}

wait_longhorn_upgrade() {
  echo "Waiting for LH settling down..."
  wait_longhorn_manager
  wait_longhorn_instance_manager_aio
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
  # Remove extraEnv fields from values.yaml. We don't want config like CATTLE_SHELL_IMAGE to overwrite shell-image setting.
  yq -i 'del(.extraEnv)' values.yaml
  echo "Rancher values:"
  cat values.yaml

  RANCHER_CURRENT_VERSION=$(yq -e e '.rancherImageTag' values.yaml)
  if [ -z "$RANCHER_CURRENT_VERSION" ]; then
    echo "[ERROR] Fail to get current Rancher version."
    exit 1
  fi

  # Clusters with witness node should have rancher's replicas set to -2 if the total number of nodes is 3.
  local total_nodes_count=$(kubectl get nodes -o json 2>/dev/null | jq -r '.items | length' || echo 0)
  local witness_nodes_count=$(kubectl get nodes -l "node-role.harvesterhci.io/witness=true" -o json 2>/dev/null | jq -r '.items | length' || echo 0)
  # Here we don't consider the case of multiple witness nodes, as we prohibit it in Harvester.
  if [[ "$witness_nodes_count" -gt 0 && "$total_nodes_count" -eq 3 ]]; then
      echo "3-node cluster with witness node detected, setting Rancher replicas to -2"
      RANCHER_REPLICAS=-2 yq e '.replicas = env(RANCHER_REPLICAS)' values.yaml -i
  fi

  # drop the potential manual patch upon shell-image to v0.1.26 on Harvester v1.3.2
  local shellimage=$(kubectl get settings.management.cattle.io shell-image -ojsonpath='{.value}')
  if [[ "$shellimage" = "rancher/shell:v0.1.26" ]]; then
    echo "rancher shell-image is $shellimage, will be reverted to empty"
    kubectl patch settings.management.cattle.io shell-image --type merge -p '{"value":""}'
    kubectl get settings.management.cattle.io shell-image
  else
    echo "rancher shell-image is $shellimage, patch is not needed"
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

  save_fleet_controller_configmap

  yq -i '.features = "multi-cluster-management=false,multi-cluster-management-agent=false,managed-system-upgrade-controller=false"' values.yaml

  REPO_RANCHER_VERSION=$REPO_RANCHER_VERSION yq -e e '.rancherImageTag = strenv(REPO_RANCHER_VERSION)' values.yaml -i
  echo "Rancher patch file to be run via helm upgrade"
  cat values.yaml
  ./helm upgrade rancher ./*.tgz --namespace cattle-system -f values.yaml --wait

  # Wait until new version ready
  until [ "$(get_running_rancher_version)" = "$REPO_RANCHER_VERSION" ]; do
    echo "Wait for Rancher to be upgraded to $REPO_RANCHER_VERSION..."
    sleep 5
  done

  echo "Wait for Rancher dependencies helm releases..."
  wait_helm_release cattle-fleet-system fleet fleet-$REPO_FLEET_CHART_VERSION $REPO_FLEET_APP_VERSION deployed
  wait_helm_release cattle-fleet-system fleet-crd fleet-crd-$REPO_FLEET_CRD_CHART_VERSION $REPO_FLEET_CRD_APP_VERSION deployed
  wait_helm_release cattle-system rancher-webhook rancher-webhook-$REPO_RANCHER_WEBHOOK_CHART_VERSION $REPO_RANCHER_WEBHOOK_APP_VERSION deployed

  # wait Rancher depoyment is ready
  echo "Wait for Rancher deployment rollout..."
  wait_rollout cattle-system deployment rancher
  echo "Rancher deployment and pods"
  kubectl get -n cattle-system deployment rancher -owide
  kubectl get pods -n cattle-system -l "app=rancher" -owide

  echo "Wait for Rancher dependencies rollout..."
  wait_rollout cattle-fleet-system deployment fleet-controller
  wait_rollout cattle-system deployment rancher-webhook

  # Create cattle-system/stv-aggregation secret to make system-agent-upgrader plan ready
  if ! kubectl get secret -n cattle-system stv-aggregation &>/dev/null; then
    echo "Create cattle-system/stv-aggregation secret"
    kubectl create secret generic -n cattle-system stv-aggregation
  fi

  # fleet-agnet is deployed as statefulset after fleet v0.10.1
  # v0.9.2: https://github.com/rancher/fleet/blob/e75c1fb498e3137ba39c2bdc4d59c9122f5ef9c6/internal/cmd/controller/agent/manifest.go#L136-L145
  # v0.10.1: https://github.com/rancher/fleet/blob/62de718a20e1377d5a8702876077762ed9a37f27/internal/cmd/controller/agentmanagement/agent/manifest.go#L152-L161
  wait_rollout_with_loop cattle-fleet-local-system deployment fleet-agent
  echo "Wait for cluster settling down..."
  wait_capi_cluster fleet-local local $pre_generation

  # Following patch is not enough
  wait_rollout_with_loop cattle-fleet-local-system deployment fleet-agent
  pre_patch_timestamp=$(fleet_agent_timestamp)
  patch_fleet_cluster
  wait_rollout_with_loop cattle-fleet-local-system deployment fleet-agent

  # After fleet-controller POD is restarted, it will check until the local cluster is imported, after that, redeploy the fleet-agent
  # Need to wait until fleet-controller assumes the cluster is ready, avoid fleet-agent is accidentally re-deployed and influence related managedcharts
  wait_cluster_local_and_fleet
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
  kubectl patch deployment harvester-cluster-repo -n cattle-system --patch-file ./cluster_repo.yaml --type strategic

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

ensure_ingress_class_name() {
  echo "Ensuring existing rancher-expose Ingress has ingress class name specified"

  INGRESS_CLASS_NAME=$(kubectl -n cattle-system get ingress rancher-expose -o jsonpath='{.spec.ingressClassName}' || true)
  if [ -n "$INGRESS_CLASS_NAME" ]; then
    echo "The ingress class name of the rancher-expose Ingress has been set: $INGRESS_CLASS_NAME"
    return 0
  fi

  # find out the default ingress class with the annotation "ingressclass.kubernetes.io/is-default-class"
  # if more than one, take the oldest; if no matches, set it to "nginx"
  DEFAULT_INGRESS_CLASS=$(kubectl get ingressclasses --sort-by=.metadata.creationTimestamp -o yaml | yq '.items[] | select(.metadata.annotations | has("ingressclass.kubernetes.io/is-default-class")) | .metadata.name' | head -n 1 || true)
  DEFAULT_INGRESS_CLASS=${DEFAULT_INGRESS_CLASS:-nginx}

  cat > rancher-expose.yaml <<EOF
spec:
  ingressClassName: "$DEFAULT_INGRESS_CLASS"
EOF

  echo "Setting the ingress class name $DEFAULT_INGRESS_CLASS for rancher-expose Ingress"
  kubectl -n cattle-system patch ingress rancher-expose --patch-file ./rancher-expose.yaml --type=merge
}

patch_longhorn_settings() {
  # set Longhorn default settings with Harvester expected values, only when they are not set in managedchart and LH uses the default value
  local target=$1
  local EXIT_CODE=0
  # yq returns 'Error: no matches found' if an item is not found
  yq -e '.spec.values.longhorn.defaultSettings.nodeDrainPolicy' $target || EXIT_CODE=$?
  if [ $EXIT_CODE != 0 ]; then
    local ndp=$(kubectl get setting.longhorn.io -n longhorn-system node-drain-policy -ojsonpath="{.value}")
    if [ $ndp == "block-if-contains-last-replica" ]; then
      echo "patch longhorn nodeDrainPolicy to allow-if-replica-is-stopped"
      yq '.spec.values.longhorn.defaultSettings.nodeDrainPolicy = "allow-if-replica-is-stopped"' -i $target
    else
      # user may set it from LH UI
      echo "longhorn nodeDrainPolicy $ndp is not the default value, do not patch"
    fi
  else
    echo "longhorn nodeDrainPolicy has been set in managedchart, do not patch again"
  fi

  EXIT_CODE=0
  yq -e '.spec.values.longhorn.defaultSettings.detachManuallyAttachedVolumesWhenCordoned' $target || EXIT_CODE=$?
  if [ $EXIT_CODE != 0 ]; then
    local dma=$(kubectl get setting.longhorn.io -n longhorn-system detach-manually-attached-volumes-when-cordoned  -ojsonpath="{.value}")
    if [ $dma == "false" ]; then
      echo "patch longhorn detachManuallyAttachedVolumesWhenCordoned to true"
      yq '.spec.values.longhorn.defaultSettings.detachManuallyAttachedVolumesWhenCordoned = true' -i $target
    else
      # user may set it from LH UI
      echo "longhorn detachManuallyAttachedVolumesWhenCordoned $dma is not the default value, do not patch"
    fi
  else
    echo "longhorn detachManuallyAttachedVolumesWhenCordoned has been set in managedchart, do not patch again"
  fi

  echo "longhorn related config"
  yq -e '.spec.values.longhorn' $target || echo "fail to get info .spec.values.longhorn"
}

upgrade_managedchart_harvester_crd() {
  echo "Upgrading Harvester CRD managedchart fleet-local/harvester-crd"

  local pre_generation_harvester_crd=$(kubectl get managedcharts.management.cattle.io harvester-crd -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  local hcpatch=harvester-crd-patch.yaml
  cat >${hcpatch} <<EOF
spec:
  version: ${REPO_HARVESTER_CHART_VERSION}
EOF

  update_managedchart_patch_file_annotations ${hcpatch} $REPO_HARVESTER_CHART_VERSION
  update_managedchart_patch_file_unpause ${hcpatch}
  # use a new timeoutSeconds to ensure the observedGeneration is updated
  update_managedchart_patch_file_timeoutseconds ${hcpatch} fleet-local harvester-crd
  echo "The final content of harvester-crd patch file"
  cat ${hcpatch}

  # wait until managedchart harvester-crd is ready first, it is used by managedchart harvester
  echo "Upgrading..."
  kubectl patch managedcharts.management.cattle.io harvester-crd -n fleet-local --patch-file ./${hcpatch} --type merge
  #pause_managed_chart harvester-crd "false"  // replaced by above patch file
  wait_managed_chart fleet-local harvester-crd $REPO_HARVESTER_CHART_VERSION $pre_generation_harvester_crd ready
}

upgrade_managedchart_harvester() {
  echo "Upgrading Harvester managedchart fleet-local/harvester"

  local hpatch=harvester.yaml
  cat >${hpatch} <<EOF
apiVersion: management.cattle.io/v3
kind: ManagedChart
metadata:
  name: harvester
  namespace: fleet-local
EOF

  kubectl get managedcharts.management.cattle.io -n fleet-local harvester -o yaml | yq e '{"spec": .spec}' - >>${hpatch}
  pre_generation_harvester=$(kubectl get managedcharts.management.cattle.io harvester -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  upgrade_managed_chart_from_version $UPGRADE_PREVIOUS_VERSION harvester ${hpatch}
  NEW_VERSION=$REPO_HARVESTER_CHART_VERSION yq e '.spec.version = strenv(NEW_VERSION)' ${hpatch} -i

  local sc=$(kubectl get sc -o json | jq '.items[] | select(.metadata.annotations."storageclass.kubernetes.io/is-default-class" == "true" and .metadata.name != "harvester-longhorn")')
  if [ -n "$sc" ] && [ "$UPGRADE_PREVIOUS_VERSION" != "v1.0.3" ]; then
      yq e '.spec.values.storageClass.defaultStorageClass = false' -i ${hpatch}
  fi

  patch_longhorn_settings ${hpatch}

  update_managedchart_patch_file_annotations ${hpatch} $REPO_HARVESTER_CHART_VERSION
  update_managedchart_patch_file_unpause ${hpatch}
  update_managedchart_patch_file_timeoutseconds ${hpatch} fleet-local harvester
  echo "The final content of harvester patch file"
  cat ${hpatch}

  echo "Upgrading..."
  kubectl apply -f ./${hpatch}
  # pause_managed_chart harvester "false"  // replaced by above patch file
  wait_managed_chart fleet-local harvester $REPO_HARVESTER_CHART_VERSION $pre_generation_harvester ready

  wait_kubevirt harvester-system kubevirt $REPO_KUBEVIRT_VERSION
}

upgrade_harvester() {
  mkdir -p $UPGRADE_TMP_DIR/harvester
  cd $UPGRADE_TMP_DIR/harvester

  upgrade_managedchart_harvester_crd

  upgrade_managedchart_harvester
}

upgrade_managedchart_monitoring_crd() {
  local nm=rancher-monitoring-crd
  local mpatch=${nm}-patch.yaml
  echo "Upgrading Managedchart ${nm} to ${REPO_MONITORING_CHART_VERSION}"

  local pre_generation=$(kubectl get managedcharts.management.cattle.io ${nm} -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  mkdir -p $UPGRADE_TMP_DIR/monitoring
  cd $UPGRADE_TMP_DIR/monitoring

  cat >${mpatch} <<EOF
spec:
  version: ${REPO_MONITORING_CHART_VERSION}
EOF

  update_managedchart_patch_file_annotations ${mpatch} ${REPO_MONITORING_CHART_VERSION}
  update_managedchart_patch_file_unpause ${mpatch}
  # use a new timeoutSeconds to ensure the observedGeneration is updated
  update_managedchart_patch_file_timeoutseconds ${mpatch} fleet-local ${nm}
  echo "The final content of ${nm} patch file"
  cat ${mpatch}

  echo "Upgrading..."
  kubectl patch managedcharts.management.cattle.io ${nm} -n fleet-local --patch-file ${mpatch} --type merge
  wait_managed_chart fleet-local ${nm} ${REPO_MONITORING_CHART_VERSION} ${pre_generation} ready
}

upgrade_monitoring() {
  # from v1.2.0, only crd here, rancher-monitoring is upgraded in addons
  upgrade_managedchart_monitoring_crd
}

upgrade_managedchart_logging_crd() {
  local nm=rancher-logging-crd
  local lpatch=${nm}-patch.yaml
  echo "Upgrading Managedchart ${nm} to ${REPO_LOGGING_CHART_VERSION}"

  local pre_generation=$(kubectl get managedcharts.management.cattle.io ${nm} -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  mkdir -p $UPGRADE_TMP_DIR/logging
  cd $UPGRADE_TMP_DIR/logging

  cat >${lpatch} <<EOF
spec:
  version: ${REPO_LOGGING_CHART_VERSION}
EOF

  update_managedchart_patch_file_annotations ${lpatch} ${REPO_LOGGING_CHART_VERSION}
  update_managedchart_patch_file_unpause ${lpatch}
  # use a new timeoutSeconds to ensure the observedGeneration is updated
  update_managedchart_patch_file_timeoutseconds ${lpatch} fleet-local ${nm}
  echo "The final content of ${nm} patch file"
  cat ${lpatch}

  echo "Upgrading..."
  kubectl patch managedcharts.management.cattle.io ${nm} -n fleet-local --patch-file ${lpatch} --type merge
  wait_managed_chart fleet-local ${nm} ${REPO_LOGGING_CHART_VERSION} ${pre_generation} ready
}

upgrade_logging_event_audit() {
  # from v1.2.0, only crd here, rancher-logging is upgraded in addons
  upgrade_managedchart_logging_crd
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
}

skip_restart_rancher_system_agent() {
  # to prevent rke2-server/agent from restarting during the rancher upgrade.
  # by adding an env var to temporarily make rancher-system-agent on each node skip restarting rke2-server/agent.
  # issue link: https://github.com/rancher/rancher/issues/41965

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
    image: registry.suse.com/bci/bci-base:15.6
    command:
    - chroot
    - /host
    args:
    - sh
    - -c
    - set -x && mkdir -p /run/systemd/system/rancher-system-agent.service.d && echo -e '[Service]\nEnvironmentFile=-/run/systemd/system/rancher-system-agent.service.d/10-harvester-upgrade.env' | tee /run/systemd/system/rancher-system-agent.service.d/override.conf && echo 'INSTALL_RKE2_SKIP_ENABLE=true' | tee /run/systemd/system/rancher-system-agent.service.d/10-harvester-upgrade.env && systemctl daemon-reload && systemctl restart rancher-system-agent.service
  version: "$plan_version"
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
  echo "upgrade addon rancher-monitoring"

  # .spec.valuesContent has dynamic fields, cannot merge simply, review in each release
  # in v1.5.0, patch version is OK
  upgrade_addon_try_patch_version_only "rancher-monitoring" "cattle-monitoring-system" $REPO_MONITORING_CHART_VERSION

  # patch configmap and replace grafana pod if necessary
  if [ "$REPO_MONITORING_CHART_VERSION" = "105.1.2+up61.3.2" ]; then
    patch_grafana_nginx_proxy_config_configmap
  fi
}

# NOTE: review in each release, add corresponding process
upgrade_addon_rancher_logging()
{
  echo "upgrade addon rancher-logging"
  # .spec.valuesContent has dynamic fields, cannot merge simply, review in each release
  # the eventrouter image tag is aligned with Harvester tag, e.g. v1.5.1-rc3, v1.6.0
  upgrade_addon_rancher_logging_with_patch_eventrouter_image $REPO_LOGGING_CHART_VERSION $REPO_LOGGING_CHART_HARVESTER_EVENTROUTER_VERSION
}

# NOTE: review in each release, add corresponding process, runs before rancher-logging is bumped
upgrade_harvester_upgradelog_loggingref() {
  echo "upgrade harvester upgradelog loggingref"
  # in v1.5.0, new rancher-logging is bumped, loggingref is required
  if [ "${REPO_LOGGING_CHART_VERSION}" = "105.2.0+up4.10.0" ]; then
    upgrade_harvester_upgradelog_with_patch_loggingref "${REPO_LOGGING_CHART_VERSION}"
  fi
}

# adapt upgradeLog to new logging stack requirements, runs after rancher-logging is bumped
upgrade_harvester_upgradelog_logging_fluentd_fluentbit() {
  echo "upgrade harvester upgradelog logging fluend fluentbit"
  # in v1.5.0, new rancher-logging is bumped, fluentbitagent and others are required
  if [ "${REPO_LOGGING_CHART_VERSION}" = "105.2.0+up4.10.0" ]; then
    upgrade_harvester_upgradelog_with_patch_logging_fluentd_fluentbit "${REPO_LOGGING_CHART_VERSION}"
  fi
}

upgrade_addons()
{
  wait_for_addons_crd
  addons="vm-import-controller pcidevices-controller harvester-seeder"
  for addon in $addons; do
    upgrade_addon $addon "harvester-system"
  done

  # the rancher-monitoring and rancher-logging addon have flexible user-configurable fields
  # from v1.2.0, they are upgraded per following
  upgrade_addon_rancher_monitoring
  # the upgradelog may be affected by the new rancher-logging
  upgrade_harvester_upgradelog_loggingref
  upgrade_addon_rancher_logging
  # after rancher-logging is upgraded, upgrade upgradelog if necessary
  upgrade_harvester_upgradelog_logging_fluentd_fluentbit

  upgrade_nvidia_driver_toolkit_addon

  manage_kubeovn
}

reuse_vlan_cn() {
  [[ $UPGRADE_PREVIOUS_VERSION != "v1.0.3" ]] && return

  # delete finalizer
  kubectl get clusternetwork vlan -o yaml | yq '.metadata.finalizers = []' | kubectl apply -f -
}

sync_containerd_registry_to_rancher() {
  echo "Sync containerd-registry setting to Rancher"

  # Check if .spec.rkeConfig.registries is not null or empty.
  # If the field is not null or empty, then the registries have
  # already previously been synced to Rancher and there's no work
  # to do here.
  local num_registries_keys=$(kubectl get --namespace=fleet-local clusters.provisioning.cattle.io local -o yaml | yq '.spec.rkeConfig.registries | length')
  if [[ $num_registries_keys -gt 0 ]]; then
    echo "Rancher registries already set"
    return
  fi

  # Otherwise, write an annotation to the setting to trigger the
  # controller which will sync the settings up to Rancher.
  kubectl annotate --overwrite=true setting.harvesterhci.io containerd-registry "harvesterhci.io/upgrade-patched=$REPO_HARVESTER_VERSION"
}

# rancher v2.8.1 introduced a new field fleetWorkspaceName in the cluster.provisioning CRD.
# for new installs this is already patched by rancherd during bootstrap of cluster however rancherd logic is not
# re-run in the upgrade path as a result this needs to be handled out of band
patch_local_cluster_details() {
  kubectl label -n fleet-local cluster.provisioning local "provisioning.cattle.io/management-cluster-name=local" --overwrite=true
  kubectl patch -n fleet-local cluster.provisioning local --subresource=status --type=merge --patch '{"status":{"fleetWorkspaceName": "fleet-local"}}'
}

# RedeployAgentGeneration can be used to force redeploying the agent.
# RedeployAgentGeneration int64 `json:"redeployAgentGeneration,omitempty"`
patch_fleet_cluster() {
  local generation=$(kubectl get -n fleet-local cluster.fleet local -o jsonpath='{.status.agentDeployedGeneration}')
  local new_generation=$((generation+1))
  patch_manifest="$(mktemp --suffix=.json)"
  cat > "$patch_manifest" <<EOF
{
  "spec": {
    "redeployAgentGeneration": $new_generation
  }
}
EOF
  echo "patch cluster.fleet local to new generation $new_generation"
  kubectl patch -n fleet-local cluster.fleet local  --type=merge --patch-file $patch_manifest
  rm -f $patch_manifest
}

# wait for statefulset will wait until statefulset exists
wait_for_statefulset() {
  local namespace=$1
  local name=$2
  local found=$(kubectl get statefulset -n $namespace -o json | jq -r --arg DEPLOYMENT $name '.items[].metadata | select (.name == $DEPLOYMENT) | .name')
  while [ -z $found ]
  do
    echo "waiting for statefulset $name to be created in namespace $namespace, sleeping for 10 seconds"
    sleep 10
    found=$(kubectl get statefulset -n $namespace -o json | jq -r --arg DEPLOYMENT $name '.items[].metadata | select (.name == $DEPLOYMENT) | .name')
  done
}

fleet_agent_timestamp(){
  wait_rollout cattle-fleet-local-system deployment fleet-agent &> /dev/null
  local temptime=$(kubectl get deployment -n cattle-fleet-local-system fleet-agent -o json | jq -r .metadata.creationTimestamp)
  if [ -z "$temptime" ]; then
    # if kubectl happens to fail due to deployment is just deleted, echo 0 to continue
    echo "0"
  else
    date -u -d $temptime +'%s'
  fi
}

wait_for_fleet_agent(){
  local timestamp=$1
  local newtimestamp=$(fleet_agent_timestamp)
  echo "wait for fleet-agent, current timestamp $timestamp"
  while [ $timestamp -ge $newtimestamp ]
  do
    echo "waiting for fleet-agent creation timestamp to be updated"
    sleep 10
    newtimestamp=$(fleet_agent_timestamp)
  done
  echo "end with new timestamp $newtimestamp"
}

upgrade_harvester_csi_rbac() {

  # only versions before v1.4.0 that upgrading to v1.4.0 need this patch
  if [[ ! "${UPGRADE_PREVIOUS_VERSION%%-rc*}" < "v1.4.0" ]]; then
    echo "Only versions before v1.4.0 need this patch."
    return
  fi

  if kubectl get clusterrole harvesterhci.io:csi-driver 2> /dev/null; then
    echo "Upgrade ClusterRole harvesterhci.io:csi-driver ..."

    cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/component: apiserver
    app.kubernetes.io/name: harvester
    app.kubernetes.io/part-of: harvester
  name: harvesterhci.io:csi-driver
rules:
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - harvesterhci.io
  resources:
  - networkfilesystems
  - networkfilesystems/status
  verbs:
  - '*'
- apiGroups:
  - longhorn.io
  resources:
  - volumes
  - volumes/status
  verbs:
  - get
  - list
EOF
  else
    echo "ClusterRole harvesterhci.io:csi-driver not found, skip updating."
  fi
}

apply_extra_nonversion_manifests()
{
  shopt -s nullglob

  echo "Applying cdi manifests"

  for manifest in /usr/local/share/extra_manifests/cdi/*.yaml; do
      echo "Applying $manifest"
      kubectl apply -f "$manifest"
  done


  shopt -u nullglob
}

wait_repo
detect_repo
detect_upgrade
pre_upgrade_manifest
pause_all_charts
skip_restart_rancher_system_agent
upgrade_rancher
patch_local_cluster_details
update_local_rke_state_secret
upgrade_harvester_cluster_repo
upgrade_network
ensure_ingress_class_name
apply_extra_nonversion_manifests
upgrade_harvester
sync_containerd_registry_to_rancher
wait_longhorn_upgrade
reuse_vlan_cn
upgrade_monitoring
upgrade_logging_event_audit
apply_extra_manifests
upgrade_addons
upgrade_harvester_csi_rbac
# wait fleet bundles upto 90 seconds
wait_for_fleet_bundles 9
