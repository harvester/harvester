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
    curl -sSfL "$url" -o "$output" --create-dirs && break
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
  REPO_LOGGING_CHART_VERSION=$(yq -e e '.loggingChart' $release_file)
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

  if [ -z "$REPO_LOGGING_CHART_VERSION" ]; then
    echo "[ERROR] Fail to get logging chart version from upgrade repo."
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
  if [ "$SKIP_VERSION_CHECK" = "true" ]; then
    echo "Skip minimum upgradable version check."
    return
  fi

  local ret=0
  upgrade-helper version-guard "$HARVESTER_UPGRADE_NAME" || ret=$?
  if [ $ret -ne 0 ]; then
    echo "Version checking failed. Abort."
    exit $ret
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

      # Check if images already exist
      curl -sfL $image_list_url | sort > $image_list_file
      missing=$($CTR -n k8s.io images ls -q | grep -v ^sha256 | sort | comm -23 $image_list_file -)
      if [ -z "$missing" ]; then
        echo "Images in $image_list_file already present in the system. Skip preloading."
        continue
      fi

      curl -fL $archive_url | zstdcat | $CTR -n k8s.io images import --no-unpack -
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
  SKIP_VERSION_CHECK=$(echo "$upgrade_obj" | yq e '.metadata.annotations."harvesterhci.io/skip-version-check"' -)
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

wait_for_fleet_bundles()
{
  # wait for the changes to be applied to bundle object first
  # fleet-agent takes time to further apply changes to downstream deployments
  sleep 10

  # loop wait upto 3 minutes by default, do not block even when some bundles are not ready
  # that needs manual operation
  if [ -z $1 ]; then
    local loop_cnt=18
  else
    local loop_cnt=$1
  fi

  while [ $loop_cnt -gt 0 ]
  do
    if [ -f "/tmp/skip-wait-for-fleet-bundles" ]; then
      echo "Skip waiting for all fleet bundles to be ready."
      break
    fi

    local EXIT_CODE=0
    local bsr=""
    echo ""
    date

    kubectl get bundle.fleet.cattle.io -A || true
    bsr=$(kubectl get bundle.fleet.cattle.io -A -o jsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}') || EXIT_CODE=$?
    if [ $EXIT_CODE != 0 ]; then
      echo "get bundle status fail, try again"
    else
      if [[ $bsr == *"False"* ]]; then
        echo "some bundles are not ready"
      else
        echo "all bundles are ready"
        return
      fi
    fi

    sleep 10
    loop_cnt=$((loop_cnt-1))
  done

  echo "finish wait fleet bundles"
}

# detect harvester vip from service first, then configmap, skip potential error
detect_harvester_vip()
{
  local EXIT_CODE=0
  #escape the 'return on error'

  local VIP=$(kubectl get service -n kube-system ingress-expose -o "jsonpath={.spec.loadBalancerIP}") || EXIT_CODE=$?
  if test $EXIT_CODE = 0; then
    HARVESTER_VIP=$VIP
    return 0
  else
    echo "kubectl get service -n kube-system ingress-expose failed, will try get from configmap"
  fi

  EXIT_CODE=0
  local JSON_DATA=$(kubectl get configmap -n kube-system kubevip -o "jsonpath={.data['kubevip-services']}") || EXIT_CODE=$?
  if test $EXIT_CODE = 0; then
    VIP=$(echo $JSON_DATA | jq -r .services[0].vip) || EXIT_CODE=$?
    if test $EXIT_CODE = 0; then
      HARVESTER_VIP=$VIP
    else
      echo "jq parse kubevip configmap json text failed: $JSON_DATA"
    fi
  else
    echo "kubectl get configmap -n kube-system kubevip failed"
  fi
}

# formal release like v1.2.0
# the "true" / "false" are the "return" value, DO NOT echo anything else
is_formal_release()
{
  local REGEX='^v[0-9]{1}[\.][0-9]{1}[\.][0-9]{1}$'
  if [[ $1 =~ $REGEX ]]; then
    echo "true"
  else
    echo "false"
  fi
}

# rc release like v1.2.0-rc2
# the "true" / "false" are the "return" value, DO NOT echo anything else
is_rc_release()
{
  local REGEX='^v[0-9]{1}[\.][0-9]{1}[\.][0-9]{1}-rc.*$'
  if [[ $1 =~ $REGEX ]]; then
    echo "true"
  else
    echo "false"
  fi
}

# upgrade addon, the only operation is to upgrade the chart version
upgrade_addon_try_patch_version_only()
{
  local name=$1
  local namespace=$2
  local newversion=$3
  echo "try to patch addon $name in $namespace to $newversion"

  # check if addon is there
  local version=$(kubectl get addons.harvesterhci.io $name -n $namespace -o=jsonpath='{.spec.version}' || true)
  if [[ -z "$version" ]]; then
    echo "addon is not found, nothing to do"
    return 0
  fi

  # check if version is updated
  if [[ "$version" = "$newversion" ]]; then
    echo "addon has already been $newversion, nothing to do"
    return 0
  fi

  # patch version
  local patchfile=addon-patch-temp.yaml
  cat > $patchfile <<EOF
spec:
  version: "$newversion"
EOF
  echo "to be patched file content"
  cat ./$patchfile

  local enabled=""
  local curstatus=""
  enabled=$(kubectl get addons.harvesterhci.io $name -n $namespace -o=jsonpath='{.spec.enabled}' || true)
  if [[ $enabled = "true" ]]; then
    curstatus=$(kubectl get addons.harvesterhci.io $name -n $namespace -o=jsonpath='{.status.status}' || true)
  fi

  kubectl patch addons.harvesterhci.io $name -n $namespace --patch-file ./$patchfile --type merge
  rm -f ./$patchfile

  # wait status only when enabled and already AddonDeploySuccessful
  wait_for_addon_upgrade_deployment $name $namespace $enabled $curstatus
}

wait_for_addon_upgrade_deployment() {
  local name=$1
  local namespace=$2
  local enabled=$3
  local curstatus=$4
  local targetstatus="AddonDeploySuccessful"

  # wait status only when enabled and already AddonDeploySuccessful, max 300 s
  if [[ $enabled = "true" ]]; then
    if [[ "$curstatus" = "$targetstatus" ]]; then
      date
      echo "wait addon status to be $targetstatus"
      for i in $(seq 1 60)
      do
        sleep 5
        local status=""
        status=$(kubectl get addons.harvesterhci.io $name -n $namespace -o=jsonpath='{.status.status}' || true)
        if [[ "$status" != "$targetstatus" ]]; then
          echo "current status is $status, continue wait: $i"
          continue
        else
          date
          echo "addon status is $targetstatus"
          return 0
        fi
      done
      # do not block the upgrade if an addon failed after 300s
      date
      echo "the addon did not become $targetstatus after 300 seconds, recover it manually after the upgrade"
    else
      if [[ -z $curstatus ]]; then
        echo "addon status is failed to fetch, do not wait"
      else
        echo "addon status is $curstatus, do not wait"
      fi
    fi
  else
    if [[ -z $enabled ]]; then
      echo "addon enabled is failed to fetch, do not wait"
    else
      echo "addon enabled is $enabled, do not wait"
    fi
  fi
}

upgrade_addon_rancher_logging_with_patch_eventrouter_image()
{
  local name=rancher-logging
  local namespace=cattle-logging-system
  local newversion=$1
  echo "try to patch addon $name in $namespace to $newversion, with patch of eventrouter image"

  # check if addon is there
  local version=$(kubectl get addons.harvesterhci.io $name -n $namespace -o=jsonpath='{.spec.version}' || true)
  if [[ -z "$version" ]]; then
    echo "addon is not found, nothing to do"
    return 0
  fi

  local valuesfile="logging-values-temp.yaml"
  rm -f $valuesfile
  kubectl get addons.harvesterhci.io $name -n $namespace -ojsonpath="{.spec.valuesContent}" > $valuesfile

  local EXIT_CODE=0

  echo "check eventrouter image tag"
  local fixeventrouter=false
  local tag=""
  tag=$(yq -e '.eventTailer.workloadOverrides.containers[0].image' $valuesfile) || EXIT_CODE=$?

  if [ $EXIT_CODE != 0 ]; then
    echo "eventrouter is not found, need not patch"
  else
    if [[ "rancher/harvester-eventrouter:v0.3.2" > $tag ]]; then
      echo "eventrouter image is $tag, will patch to v0.3.2"
      fixeventrouter=true
    else
      echo "eventrouter image is updated, need not patch"
    fi
  fi

  if [[ $fixeventrouter == false ]]; then
    echo "eventrouter image is updated/not found, fallback to the normal addon $name upgrade"
    rm -f $valuesfile
    upgrade_addon_try_patch_version_only $name $namespace $newversion
    return 0
  fi

  echo "current valuesContent of the addon $name:"
  cat $valuesfile

  if [[ $fixeventrouter == true ]]; then
    yq -e '.eventTailer.workloadOverrides.containers[0].image = "rancher/harvester-eventrouter:v0.3.2"' -i $valuesfile
  fi

  # add 4 spaces to each line
  sed -i -e 's/^/    /' $valuesfile
  local newvalues=$(<$valuesfile)
  rm -f $valuesfile

  local patchfile="addon-patch-temp.yaml"
  rm -f $patchfile

cat > $patchfile <<EOF
spec:
  version: $newversion
  valuesContent: |
$newvalues
EOF

  local enabled=""
  local curstatus=""
  enabled=$(kubectl get addons.harvesterhci.io $name -n $namespace -o=jsonpath='{.spec.enabled}' || true)
  if [[ $enabled = "true" ]]; then
    curstatus=$(kubectl get addons.harvesterhci.io $name -n $namespace -o=jsonpath='{.status.status}' || true)
  fi

  echo "new patch of addon $name:"
  cat $patchfile

  kubectl patch addons.harvesterhci.io $name -n $namespace --patch-file ./$patchfile --type merge
  rm -f ./$patchfile

  # wait status only when enabled and already AddonDeploySuccessful
  wait_for_addon_upgrade_deployment $name $namespace $enabled $curstatus
}


upgrade_nvidia_driver_toolkit_addon()
{
  # patch nvidia-driver-toolkit with existing location before performing upgrade
  CURRENTENDPOINT=$(kubectl get addons.harvester nvidia-driver-toolkit -n harvester-system -o yaml | yq .spec.valuesContent | yq '.driverLocation // "empty"')
  if [ ${CURRENTENDPOINT} != "empty" ]
  then
    sed -i "s|HTTPENDPOINT/NVIDIA-Linux-x86_64-vgpu-kvm.run|${CURRENTENDPOINT}|" /usr/local/share/addons/nvidia-driver-toolkit.yaml
  fi
  upgrade_addon nvidia-driver-toolkit harvester-system
}
