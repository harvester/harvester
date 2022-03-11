#!/bin/bash -ex

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
UPGRADE_TMP_DIR="/tmp/upgrade"

source $SCRIPT_DIR/lib.sh

wait_managed_chart()
{
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

wait_helm_release()
{
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

wait_rollout()
{
  namespace=$1
  resource_type=$2
  name=$3

  kubectl rollout status --watch=true -n $namespace $resource_type $name
}

wait_capi_cluster()
{
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
        break
      fi
    fi

    sleep 5
  done
}

wait_kubevirt()
{
  # Wait for kubevirt to be upgraded
  namespace=$1
  name=$2
  version=$3


  while [ true ]; do
    kubevirt=$(kubectl get kubevirts.kubevirt.io $name -n $namespace -o yaml)

    current_phase=$(echo "$kubevirt" | yq e '.status.phase' -)
    current_target_version=$(echo "$kubevirt" | yq e '.status.observedKubeVirtVersion' -)

    if [ "$current_target_version" = "$version" ]; then
      if [ "$current_phase" = "Deployed" ]; then
        break
      fi
    fi

    sleep 5
  done
}

get_running_rancher_version()
{
  kubectl get settings.management.cattle.io server-version -o yaml | yq -e e '.value' -
}

get_cluster_repo_index_download_time()
{
  local output_type=$1
  local iso_time=$(kubectl get clusterrepos.catalog.cattle.io harvester-charts -ojsonpath='{.status.downloadTime}')

  if [ "$output_type" = "epoch" ]; then
    date -d"${iso_time}" +%s
  else
    echo $iso_time
  fi
}

upgrade_rancher()
{
  echo "Upgrading Rancher"

  mkdir -p $UPGRADE_TMP_DIR/images
  mkdir -p $UPGRADE_TMP_DIR/rancher

  # Download rancher system agent install image from upgrade repo
  download_image_archives_from_repo "agent" $UPGRADE_TMP_DIR/images

  # Extract the Rancher chart and helm binary
  wharfie --images-dir $UPGRADE_TMP_DIR/images rancher/system-agent-installer-rancher:$REPO_RANCHER_VERSION $UPGRADE_TMP_DIR/rancher

  cd $UPGRADE_TMP_DIR/rancher

  ./helm get values rancher -n cattle-system -o yaml > values.yaml
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
  until [ "$(get_running_rancher_version)" = "$REPO_RANCHER_VERSION" ]
  do
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

upgrade_harvester_cluster_repo()
{
  echo "Upgrading Harvester Cluster Repo"

  mkdir -p $UPGRADE_TMP_DIR/harvester_cluster_repo
  cd $UPGRADE_TMP_DIR/harvester_cluster_repo

  cat > cluster_repo.yaml << EOF
spec:
  template:
    spec:
      containers:
        - name: httpd
          image: rancher/harvester-cluster-repo:$REPO_OS_VERSION
EOF
  kubectl patch deployment harvester-cluster-repo -n cattle-system --patch-file ./cluster_repo.yaml --type merge

  until kubectl -n cattle-system rollout status -w deployment/harvester-cluster-repo
  do
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

  cat > catalog_cluster_repo.yaml << EOF
spec:
  forceUpdate: "$force_update_time"
EOF
  kubectl patch clusterrepos.catalog.cattle.io harvester-charts --patch-file ./catalog_cluster_repo.yaml --type merge

  until [ $(get_cluster_repo_index_download_time epoch) -ge $(date -d"${force_update_time}" +%s) ]
  do
    echo "Waiting for cluster repo catalog index update..."
    sleep 5
  done
}

upgrade_harvester()
{
  echo "Upgrading Harvester"

  pre_generation_harvester=$(kubectl get managedcharts.management.cattle.io harvester -n fleet-local -o=jsonpath='{.status.observedGeneration}')
  pre_generation_harvester_crd=$(kubectl get managedcharts.management.cattle.io harvester-crd -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  mkdir -p $UPGRADE_TMP_DIR/harvester
  cd $UPGRADE_TMP_DIR/harvester

  cat > harvester-crd.yaml <<EOF
spec:
  version: $REPO_HARVESTER_CHART_VERSION
EOF
  kubectl patch managedcharts.management.cattle.io harvester-crd -n fleet-local --patch-file ./harvester-crd.yaml --type merge

  cat > harvester.yaml <<EOF
apiVersion: management.cattle.io/v3
kind: ManagedChart
metadata:
  name: harvester
  namespace: fleet-local
EOF
  kubectl get managedcharts.management.cattle.io -n fleet-local harvester -o yaml | yq e '{"spec": .spec}' - >> harvester.yaml

  upgrade_managed_chart_from_version $UPGRADE_PREVIOUS_VERSION harvester harvester.yaml
  NEW_VERSION=$REPO_HARVESTER_CHART_VERSION yq e '.spec.version = strenv(NEW_VERSION)' harvester.yaml -i
  kubectl apply -f ./harvester.yaml

  pause_managed_chart harvester "false"
  pause_managed_chart harvester-crd "false"

  # XXX The eventual state of harvester would be "modified", not "ready"!
  wait_managed_chart fleet-local harvester $REPO_HARVESTER_CHART_VERSION $pre_generation_harvester modified
  wait_managed_chart fleet-local harvester-crd $REPO_HARVESTER_CHART_VERSION $pre_generation_harvester_crd ready

  wait_kubevirt harvester-system kubevirt $REPO_KUBEVIRT_VERSION
}

upgrade_monitoring() {
  echo "Upgrading Monitoring"

  pre_generation_monitoring=$(kubectl get managedcharts.management.cattle.io rancher-monitoring -n fleet-local -o=jsonpath='{.status.observedGeneration}')
  pre_generation_monitoring_crd=$(kubectl get managedcharts.management.cattle.io rancher-monitoring-crd -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  mkdir -p $UPGRADE_TMP_DIR/monitoring
  cd $UPGRADE_TMP_DIR/monitoring

  cat > rancher-monitoring-crd.yaml <<EOF
spec:
  version: $REPO_MONITORING_CHART_VERSION
EOF
  kubectl patch managedcharts.management.cattle.io rancher-monitoring-crd -n fleet-local --patch-file ./rancher-monitoring-crd.yaml --type merge

  cat > rancher-monitoring.yaml <<EOF
apiVersion: management.cattle.io/v3
kind: ManagedChart
metadata:
  name: rancher-monitoring
  namespace: fleet-local
EOF
  kubectl get managedcharts.management.cattle.io -n fleet-local rancher-monitoring -o yaml | yq e '{"spec": .spec}' - >> rancher-monitoring.yaml

  upgrade_managed_chart_from_version $UPGRADE_PREVIOUS_VERSION rancher-monitoring rancher-monitoring.yaml
  NEW_VERSION=$REPO_MONITORING_CHART_VERSION yq e '.spec.version = strenv(NEW_VERSION)' rancher-monitoring.yaml -i
  kubectl apply -f ./rancher-monitoring.yaml

  pause_managed_chart rancher-monitoring "false"
  pause_managed_chart rancher-monitoring-crd "false"

  # XXX The eventual state of rancher-monitoring would be "modified", not "ready"!
  wait_managed_chart fleet-local rancher-monitoring $REPO_MONITORING_CHART_VERSION $pre_generation_monitoring modified
  wait_managed_chart fleet-local rancher-monitoring-crd $REPO_MONITORING_CHART_VERSION $pre_generation_monitoring_crd ready

  wait_rollout cattle-monitoring-system daemonset rancher-monitoring-prometheus-node-exporter
  wait_rollout cattle-monitoring-system deployment rancher-monitoring-operator
}

apply_extra_manifests()
{
    echo "Applying extra manifests"

    shopt -s nullglob
    for manifest in /usr/local/share/extra_manifests/*.yaml; do
        kubectl apply -f $manifest
    done
    shopt -u nullglob
}

upgrade_managed_chart_from_version()
{
  version=$1
  chart_name=$2
  chart_manifest=$3

  if [ -e "/usr/local/share/migrations/managed_charts/${version}.sh" ]; then
    /usr/local/share/migrations/managed_charts/${version}.sh $chart_name $chart_manifest
  fi
}

pause_managed_chart()
{
  chart=$1
  do_pause=$2

  mkdir -p $UPGRADE_TMP_DIR/pause
  cd $UPGRADE_TMP_DIR/pause
  cat > ${chart}.yaml <<EOF
spec:
  paused: $do_pause
EOF
  kubectl patch managedcharts.management.cattle.io $chart -n fleet-local --patch-file ./${chart}.yaml --type merge
}

pause_all_charts()
{
  charts="harvester harvester-crd rancher-monitoring rancher-monitoring-crd"
  for chart in $charts; do
    pause_managed_chart $chart "true"
  done
}

wait_repo
detect_repo
detect_upgrade
pause_all_charts
upgrade_rancher
upgrade_harvester_cluster_repo
upgrade_harvester
upgrade_monitoring
apply_extra_manifests
