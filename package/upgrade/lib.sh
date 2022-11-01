UPGRADE_NAMESPACE="harvester-system"
UPGRADE_REPO_URL=http://upgrade-repo-$HARVESTER_UPGRADE_NAME.$UPGRADE_NAMESPACE/harvester-iso
UPGRADE_REPO_VM_NAME="upgrade-repo-$HARVESTER_UPGRADE_NAME"
UPGRADE_REPO_RELEASE_FILE="$UPGRADE_REPO_URL/harvester-release.yaml"
UPGRADE_REPO_SQUASHFS_IMAGE="$UPGRADE_REPO_URL/rootfs.squashfs"
UPGRADE_REPO_BUNDLE_ROOT="$UPGRADE_REPO_URL/bundle"
UPGRADE_REPO_BUNDLE_METADATA="$UPGRADE_REPO_URL/bundle/metadata.yaml"
CACHED_BUNDLE_METADATA=""
HOST_DIR="${HOST_DIR:-/host}"

download_file()
{
  local url=$1
  local output=$2

  echo "Downloading the file from \"$url\" to \"$output\"..."
  local i=0
  while [[ "$i" -lt 100 ]]; do
    curl -sSfL "$url" -o "$output" && break
    echo "Failed to download the requested file from \"$url\" to \"$output\" with error code: $?, retrying ($i)..."
    sleep 10
    i=$((i + 1))
  done
}

detect_repo()
{
  release_file=$(mktemp --suffix=.yaml)
  download_file "$UPGRADE_REPO_RELEASE_FILE" "$release_file"

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
  download_file "$UPGRADE_REPO_BUNDLE_METADATA" "$CACHED_BUNDLE_METADATA"
}

check_version()
{
  local current_version="${UPGRADE_PREVIOUS_VERSION#v}"
  local min_upgradable_version="${REPO_HARVESTER_MIN_UPGRADABLE_VERSION#v}"

  local is_rc=0
  [[ "$current_version" =~ "-rc" ]] && is_rc=1

  echo "Current version: $current_version"
  echo "Minimum upgradable version: $min_upgradable_version"

  if [ "$is_rc" = "1" ]; then
    local current_version_rc_stripped="${current_version%-rc*}"
    if [ "$current_version_rc_stripped" = "$min_upgradable_version" ]; then
      echo "Current version is not supported. Abort the upgrade."
      exit 1
    fi
  fi

  if [ -z "$min_upgradable_version" ]; then
    echo "No restriction."
  else
    local versions_to_compare="$min_upgradable_version
$current_version"

    # Current Harvester version should be newer or equal to minimum upgradable version
    if [ "$versions_to_compare" = "$(sort -V <<< "$versions_to_compare")" ]; then
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
        download_file "$image_list_url" "$image_list_file"
      fi

      if [ ! -e $archive_file ]; then
        download_file "$archive_url" "$archive_file"
      fi
    done
}

detect_upgrade()
{
  upgrade_obj=$(kubectl get upgrades.harvesterhci.io $HARVESTER_UPGRADE_NAME -n $UPGRADE_NAMESPACE -o yaml)

  UPGRADE_PREVIOUS_VERSION=$(echo "$upgrade_obj" | yq e .status.previousVersion -)
}

# refer https://github.com/harvester/harvester/issues/3098
detect_node_current_harvester_version()
{
  NODE_CURRENT_HARVESTER_VERSION=""
  local harvester_release_file=/host/etc/harvester-release.yaml

  if [ -f "$harvester_release_file" ]; then
    NODE_CURRENT_HARVESTER_VERSION=$(yq e '.harvester' $harvester_release_file)
    echo "NODE_CURRENT_HARVESTER_VERSION is: $NODE_CURRENT_HARVESTER_VERSION"
  else
    echo "$harvester_release_file is not existing, NODE_CURRENT_HARVESTER_VERSION is set as empty"
  fi
}

upgrade_addon()
{
  local name=$1
  local namespace=$2

  patch=$(cat /usr/local/share/addons/${name}.yaml | yq '{"spec": .spec | pick(["version", "valuesContent"])}')

  cat > addon-patch.yaml <<EOF
$patch
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
