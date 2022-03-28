UPGRADE_NAMESPACE="harvester-system"
UPGRADE_REPO_URL=http://upgrade-repo-$HARVESTER_UPGRADE_NAME.$UPGRADE_NAMESPACE/harvester-iso
UPGRADE_REPO_VM_NAME="upgrade-repo-$HARVESTER_UPGRADE_NAME"
UPGRADE_REPO_RELEASE_FILE="$UPGRADE_REPO_URL/harvester-release.yaml"
UPGRADE_REPO_SQUASHFS_IMAGE="$UPGRADE_REPO_URL/rootfs.squashfs"
UPGRADE_REPO_BUNDLE_ROOT="$UPGRADE_REPO_URL/bundle"
UPGRADE_REPO_BUNDLE_METADATA="$UPGRADE_REPO_URL/bundle/metadata.yaml"

detect_repo()
{
  release_file=$(mktemp --suffix=.yaml)
  curl -sfL $UPGRADE_REPO_RELEASE_FILE -o $release_file

  REPO_HARVESTER_VERSION=$(yq -e e '.harvester' $release_file)
  REPO_HARVESTER_CHART_VERSION=$(yq -e e '.harvesterChart' $release_file)
  REPO_OS_PRETTY_NAME="$(yq -e e '.os' $release_file)"
  REPO_OS_VERSION="${REPO_OS_PRETTY_NAME#Harvester }"
  REPO_RKE2_VERSION=$(yq -e e '.kubernetes' $release_file)
  REPO_RANCHER_VERSION=$(yq -e e '.rancher' $release_file)
  REPO_MONITORING_CHART_VERSION=$(yq -e e '.monitoringChart' $release_file)
  REPO_FLEET_CHART_VERSION=$(yq -e e '.rancherDependencies.fleet.chart' $release_file)
  REPO_FLEET_APP_VERSION=$(yq -e e '.rancherDependencies.fleet.app' $release_file)
  REPO_FLEET_CRD_CHART_VERSION=$(yq -e e '.rancherDependencies.fleet-crd.chart' $release_file)
  REPO_FLEET_CRD_APP_VERSION=$(yq -e e '.rancherDependencies.fleet-crd.app' $release_file)
  REPO_RANCHER_WEBHOOK_CHART_VERSION=$(yq -e e '.rancherDependencies.rancher-webhook.chart' $release_file)
  REPO_RANCHER_WEBHOOK_APP_VERSION=$(yq -e e '.rancherDependencies.rancher-webhook.app' $release_file)
  REPO_KUBEVIRT_VERSION=$(yq -e e '.kubevirt' $release_file)

  if [ -z "$REPO_HARVESTER_VERSION" ]; then
    echo "[ERROR] Fail to get Harvester version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_HARVESTER_CHART_VERSION" ]; then
    echo "[ERROR] Fail to get Harvester chart version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_OS_VERSION" ]; then
    echo "[ERROR] Fail to get OS version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_RKE2_VERSION" ]; then
    echo "[ERROR] Fail to get RKE2 version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_RANCHER_VERSION" ]; then
    echo "[ERROR] Fail to get Rancher version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_MONITORING_CHART_VERSION" ]; then
    echo "[ERROR] Fail to get monitoring chart version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_FLEET_CHART_VERSION" ]; then
    echo "[ERROR] Fail to get fleet chart version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_FLEET_APP_VERSION" ]; then
    echo "[ERROR] Fail to get fleet app version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_FLEET_CRD_CHART_VERSION" ]; then
    echo "[ERROR] Fail to get fleet-crd chart version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_FLEET_CRD_APP_VERSION" ]; then
    echo "[ERROR] Fail to get fleet-crd app version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_RANCHER_WEBHOOK_CHART_VERSION" ]; then
    echo "[ERROR] Fail to get rancher-webhook chart version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_RANCHER_WEBHOOK_APP_VERSION" ]; then
    echo "[ERROR] Fail to get rancher-webhook app version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_KUBEVIRT_VERSION" ]; then
    echo "[ERROR] Fail to get kubevirt version from upgrade repo."
    exit 1
  fi
}

wait_repo()
{
  local repo_vm_status

  # Start upgrade repo VM in case it's shut down due to migration timeout or job failure
  repo_vm_status=$(kubectl get virtualmachines.kubevirt.io $UPGRADE_REPO_VM_NAME -n $UPGRADE_NAMESPACE -o=jsonpath='{.status.printableStatus}')
  if [ "$repo_vm_status" != "Running" ]; then
    virtctl start $UPGRADE_REPO_VM_NAME -n $UPGRADE_NAMESPACE || true
  fi

  until curl -sfL $UPGRADE_REPO_RELEASE_FILE
  do
    echo "Wait for upgrade repo ready..."
    sleep 5
  done
}

download_image_archives_from_repo() {
  local image_type=$1
  local save_dir=$2

  local metadata
  local image_list_url
  local archive_url
  local image_list_file
  local archive_file

  metadata=$(mktemp --suffix=.yaml)
  curl -fL $UPGRADE_REPO_BUNDLE_METADATA -o $metadata

  yq -e -o=json e ".images.$image_type" $metadata | jq -r '.[] | [.list, .archive] | @tsv' |
    while IFS=$'\t' read -r list archive; do
      image_list_url="$UPGRADE_REPO_BUNDLE_ROOT/$list"
      archive_url="$UPGRADE_REPO_BUNDLE_ROOT/$archive"
      image_list_file="$save_dir/$(basename $list)"
      archive_file="$save_dir/$(basename $archive)"

      if [ ! -e $image_list_file ]; then
        curl -fL $image_list_url -o $image_list_file
      fi

      if [ ! -e $archive_file ]; then
        curl -fL $archive_url -o $archive_file
      fi
    done

  rm -f $metadata
}

detect_upgrade()
{
  upgrade_obj=$(kubectl get upgrades.harvesterhci.io $HARVESTER_UPGRADE_NAME -n $UPGRADE_NAMESPACE -o yaml)

  UPGRADE_PREVIOUS_VERSION=$(echo "$upgrade_obj" | yq e .status.previousVersion -)
}
