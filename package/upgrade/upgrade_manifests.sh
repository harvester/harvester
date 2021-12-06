#!/bin/bash -ex

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
UPGRADE_TMP_DIR="/tmp/upgrade"

source $SCRIPT_DIR/lib.sh

get_running_rancher_version()
{
  kubectl get settings.management.cattle.io server-version -o yaml | yq -e e '.value' -
}

upgrade_rancher()
{
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

  REPO_RANCHER_VERSION=$REPO_RANCHER_VERSION yq -e e '.rancherImageTag = strenv(REPO_RANCHER_VERSION)' values.yaml -i
  ./helm upgrade rancher ./*.tgz --namespace cattle-system -f values.yaml

  # Wait until new version ready
  until [ "$(get_running_rancher_version)" = "$REPO_RANCHER_VERSION" ]
  do
    echo "Wait for Rancher to be upgraded..."
    sleep 5
  done
}

upgrade_harvester_cluster_repo()
{
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
}

upgrade_harvester()
{
  mkdir -p $UPGRADE_TMP_DIR/harvester
  cd $UPGRADE_TMP_DIR/harvester

  cat > harvester-crd.yaml <<EOF
spec:
  version: $REPO_HARVESTER_CHART_VERSION
EOF
  kubectl patch managedcharts.management.cattle.io harvester-crd -n fleet-local --patch-file ./harvester-crd.yaml --type merge

  cat > harvester.yaml <<EOF
spec:
  version: $REPO_HARVESTER_CHART_VERSION
EOF
  kubectl patch managedcharts.management.cattle.io harvester -n fleet-local --patch-file ./harvester.yaml --type merge
  # TODO: Is there a way to wait for Harvester and all components are fully upgraded (and maybe operational)?
}

upgrade_monitoring() {
  mkdir -p $UPGRADE_TMP_DIR/monitoring
  cd $UPGRADE_TMP_DIR/monitoring

  cat > rancher-monitoring-crd.yaml <<EOF
spec:
  version: $REPO_MONITORING_CHART_VERSION
EOF
  kubectl patch managedcharts.management.cattle.io rancher-monitoring-crd -n fleet-local --patch-file ./rancher-monitoring-crd.yaml --type merge

  cat > rancher-monitoring.yaml <<EOF
spec:
  version: $REPO_MONITORING_CHART_VERSION
EOF
  kubectl patch managedcharts.management.cattle.io rancher-monitoring -n fleet-local --patch-file ./rancher-monitoring.yaml --type merge
}

wait_repo
detect_repo
upgrade_rancher
upgrade_harvester_cluster_repo
upgrade_harvester
upgrade_monitoring
