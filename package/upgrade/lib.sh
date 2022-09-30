UPGRADE_NAMESPACE="harvester-system"
UPGRADE_REPO_URL=http://upgrade-repo-$HARVESTER_UPGRADE_NAME.$UPGRADE_NAMESPACE/harvester-iso
UPGRADE_REPO_VM_NAME="upgrade-repo-$HARVESTER_UPGRADE_NAME"
UPGRADE_REPO_RELEASE_FILE="$UPGRADE_REPO_URL/harvester-release.yaml"
UPGRADE_REPO_SQUASHFS_IMAGE="$UPGRADE_REPO_URL/rootfs.squashfs"
UPGRADE_REPO_BUNDLE_ROOT="$UPGRADE_REPO_URL/bundle"
UPGRADE_REPO_BUNDLE_METADATA="$UPGRADE_REPO_URL/bundle/metadata.yaml"
CACHED_BUNDLE_METADATA=""
HOST_DIR="${HOST_DIR:-/host}"

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
  # Value could be: 1. valid version string; 2. empty string
  REPO_HARVESTER_MIN_UPGRADABLE_VERSION=$(yq e '.minUpgradableVersion' $release_file)

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

  CACHED_BUNDLE_METADATA=$(mktemp --suffix=.yaml)
  curl -sfL "$UPGRADE_REPO_BUNDLE_METADATA" -o "$CACHED_BUNDLE_METADATA"
}

check_version()
{
  CURRENT_VERSION=${UPGRADE_PREVIOUS_VERSION#v}
  MIN_UPGRADABLE_VERSION=${REPO_HARVESTER_MIN_UPGRADABLE_VERSION#v}

  echo "Current version: $CURRENT_VERSION"
  echo "Minimum upgradable version: $MIN_UPGRADABLE_VERSION"

  if [ -z "$MIN_UPGRADABLE_VERSION" ]; then
    echo "No restriction."
  else
    VERSIONS_TO_COMPARE="${MIN_UPGRADABLE_VERSION}
${CURRENT_VERSION}"

  # Current Harvester version should be newer or equal to minimum upgradable version
    if [ "$VERSIONS_TO_COMPARE" = "$(sort -V <<< "$VERSIONS_TO_COMPARE")" ]; then
      echo "Current version is supported."
    else
      echo "Current version is not supported. Abort the upgrade."
      exit 1
    fi
  fi
}

get_repo_vm_status()
{
  kubectl get virtualmachines.kubevirt.io $UPGRADE_REPO_VM_NAME -n $UPGRADE_NAMESPACE -o=jsonpath='{.status.printableStatus}'
}

wait_repo()
{
  # Start upgrade repo VM in case it's shut down due to migration timeout or job failure
  until [[ "$(get_repo_vm_status)" == "Running" ]]
  do
    echo "Try to bring up the upgrade repo VM..."
    virtctl start $UPGRADE_REPO_VM_NAME -n $UPGRADE_NAMESPACE || true
    sleep 10
  done

  until curl -sfL $UPGRADE_REPO_RELEASE_FILE
  do
    echo "Wait for upgrade repo ready..."
    sleep 5
  done
}

import_image_archives_from_repo() {
  local image_type=$1
  local upgrade_tmp_dir=$2
  local tmp_image_archives=$(mktemp -d -p $upgrade_tmp_dir)

  export CONTAINER_RUNTIME_ENDPOINT=unix:///$HOST_DIR/run/k3s/containerd/containerd.sock
  export CONTAINERD_ADDRESS=$HOST_DIR/run/k3s/containerd/containerd.sock

  CTR="$HOST_DIR/$(readlink $HOST_DIR/var/lib/rancher/rke2/bin)/ctr"
  if [ -z "$CTR" ];then
    echo "Fail to get host ctr binary."
    exit 1
  fi

  echo "Importing $image_type images from repo..."
  yq -e -o=json e ".images.$image_type" "$CACHED_BUNDLE_METADATA" | jq -r '.[] | [.list, .archive] | @tsv' |
    while IFS=$'\t' read -r list archive; do
      archive_name=$(basename -s .tar.zst $archive)
      image_list_url="$UPGRADE_REPO_BUNDLE_ROOT/$list"
      archive_url="$UPGRADE_REPO_BUNDLE_ROOT/$archive"
      image_list_file="${tmp_image_archives}/$(basename $list)"
      archive_file="${tmp_image_archives}/${archive_name}.tar"

      # Check if images already exist
      curl -sfL $image_list_url | sort > $image_list_file
      missing=$($CTR -n k8s.io images ls -q | grep -v ^sha256 | sort | comm -23 $image_list_file -)
      if [ -z "$missing" ]; then
        echo "Images in $image_list_file already present in the system. Skip preloading."
        continue
      fi

      curl -sfL $archive_url | zstd -d -f --no-progress -o $archive_file
      $CTR -n k8s.io image import $archive_file
      rm -f $archive_file
    done
  rm -rf $tmp_image_archives
}

download_image_archives_from_repo() {
  local image_type=$1
  local save_dir=$2

  local image_list_url
  local archive_url
  local image_list_file
  local archive_file

  yq -e -o=json e ".images.$image_type" "$CACHED_BUNDLE_METADATA" | jq -r '.[] | [.list, .archive] | @tsv' |
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
}

detect_upgrade()
{
  upgrade_obj=$(kubectl get upgrades.harvesterhci.io $HARVESTER_UPGRADE_NAME -n $UPGRADE_NAMESPACE -o yaml)

  UPGRADE_PREVIOUS_VERSION=$(echo "$upgrade_obj" | yq e .status.previousVersion -)
}

upgrade_addon()
{
  local name=$1
  local namespace=$2
  local version=$3

  version=$(cat /usr/local/share/addons/${name}.yaml | yq e .spec.version)

  cat > addon-patch.yaml <<EOF
spec:
  version: $version
EOF

  item_count=$(kubectl get addons.harvesterhci $name -n $namespace -o  jsonpath='{..name}' || true)
  if [ -z "$item_count" ]; then
    install_addon $name $namespace
  else
    kubectl patch addons.harvesterhci $name -n $namespace --patch-file ./addon-patch.yaml --type merge
  fi
}

install_addon()
{
  local name=$1
  local namespace=$2

  kubectl apply -f /usr/local/share/addons/${name}.yaml -n $namespace
}

wait_for_addons_crd()
{
  item_count=$(kubectl get customresourcedefinitions addons.harvesterhci.io -o  jsonpath='{.metadata.name}' || true)
  while [ -z "$item_count" ]
  do
    echo "wait for addons.harvesterhci.io crd to be created"
    sleep 10
    item_count=$(kubectl get customresourcedefinitions addons.harvesterhci.io -o  jsonpath='{.metadata.name}' || true)
  done
}

lower_version_check()
{
  local v1=$1
  local v2=$2
  first=$(printf "$v1\n$v2\n" | sort -V | head -1)
  if [ $first = $v1 ]; then
    echo "upgrade needed"
    echo 1 && return 0
  fi
}

shutdown_all_vms()
{
  kubectl get vmi -A -o json |
    jq -r '.items[] | [.metadata.name, .metadata.namespace] | @tsv' |
    while IFS=$'\t' read -r name namespace; do
      if [ -z "$name" ]; then
        break
      fi
      echo "Stop ${namespace}/${name}"
      virtctl stop $name -n $namespace
    done
}
