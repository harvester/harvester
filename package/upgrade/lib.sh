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
  REPO_LOGGING_CHART_HARVESTER_EVENTROUTER_VERSION=$(yq -e e '.loggingChartHarvesterEventRouter' $release_file)
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

  if [ -z "$REPO_LOGGING_CHART_HARVESTER_EVENTROUTER_VERSION" ]; then
    echo "[ERROR] Fail to get logging chart harvester eventrouter version from upgrade repo."
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
  local ernewversion=$2
  echo "try to patch addon $name in $namespace to $newversion, with patch of eventrouter image to $ernewversion"

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
    echo "eventrouter image is $tag, will patch to $ernewversion"
    fixeventrouter=true
  fi

  if [[ $fixeventrouter == false ]]; then
    echo "eventrouter image is not found, fallback to the normal addon $name upgrade"
    rm -f $valuesfile
    upgrade_addon_try_patch_version_only $name $namespace $newversion
    return 0
  fi

  echo "current valuesContent of the addon $name:"
  cat $valuesfile

  if [[ $fixeventrouter == true ]]; then
    NEW_VERSION=$ernewversion yq -e '.eventTailer.workloadOverrides.containers[0].image = strenv(NEW_VERSION)' -i $valuesfile
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

upgrade_harvester_upgradelog_with_patch_loggingref()
{
  local namespace="${UPGRADE_NAMESPACE}"
  local upgradelogname=$(kubectl get upgrades.harvesterhci.io "${HARVESTER_UPGRADE_NAME}" -n "${UPGRADE_NAMESPACE}" -ojsonpath="{.status.upgradeLog}")
  local loggingref="harvester-upgradelog"

  if [[ -z "${upgradelogname}" ]]; then
    echo "upgradelog is not found from upgrade ${HARVESTER_UPGRADE_NAME}, nothing to do"
    return 0
  fi

  # patch_clusteroutput
  echo "patch clusteroutput ${upgradelogname}-clusteroutput"
  local patchfile="patch_clusteroutput.yaml"
  cat > "${patchfile}" <<EOF
spec:
  loggingRef: "${loggingref}"
EOF
  kubectl patch clusteroutput -n "${namespace}" "${upgradelogname}"-clusteroutput --patch-file ./"${patchfile}" --type merge || echo "failed to patch upgradeLog clusteroutput"
  rm -rf ./"${patchfile}"

  # patch_clusterflow
  echo "patch clusterflow ${upgradelogname}-clusterflow"
  patchfile="patch_clusterflow.yaml"
  cat > "${patchfile}" <<EOF
spec:
  loggingRef: "${loggingref}"
EOF
  kubectl patch clusterflow -n "${namespace}" "${upgradelogname}"-clusterflow --patch-file ./"${patchfile}" --type merge || echo "failed to patch upgradeLog clusterflow"
  rm -rf ./"${patchfile}"

  # patch logging
  echo "patch logging ${upgradelogname}-infra"
  patchfile="patch_logging.yaml"
  cat > "${patchfile}" <<EOF
spec:
  loggingRef: "${loggingref}"
EOF
  kubectl patch logging -n "${namespace}" "${upgradelogname}"-infra --patch-file ./"${patchfile}" --type merge || echo "failed to patch upgradeLog logging"
  rm -rf ./"${patchfile}"

  # patch the may be existing logging operator-root
  echo "patch logging ${upgradelogname}-operator-root"
  patchfile="patch_logging.yaml"
  cat > "${patchfile}" <<EOF
spec:
  loggingRef: "harvester-upgradelog-operator-root"
EOF
  kubectl patch logging -n "${namespace}" "${upgradelogname}"-operator-root --patch-file ./"${patchfile}" --type merge || echo "failed to patch upgradeLog logging operator-root or it is not existing"
  rm -rf ./"${patchfile}"
}

# upgrade upgradelog operator managedchart if it is existing
upgrade_managedchart_upgradelog_operator()
{
  local upgradelogname=$1
  local upgradeloguid=$2
  local nm="${upgradelogname}"-operator
  local newver="${REPO_LOGGING_CHART_VERSION}"

  local upgradelogoperator=$(kubectl get managedcharts.management.cattle.io "$nm" -n fleet-local -ojsonpath="{.metadata.name}")
  if [[ -z "${upgradelogoperator}" ]]; then
    echo "the managedchart ${nm} is not found, nothing to patch"
    return 0
  fi

  local pre_version=$(kubectl get managedcharts.management.cattle.io "$nm" -n fleet-local -o=jsonpath='{.spec.version}')
  if [ "${pre_version}" = "${newver}" ]; then
    echo "the managedchart ${nm} has already been target version ${newver}"
    pause_managed_chart "${nm}" "false"
    return 0
  fi

  echo "upgrading managedchart ${nm} to ${newver}"

  local pre_generation=$(kubectl get managedcharts.management.cattle.io "${nm}" -n fleet-local -o=jsonpath='{.status.observedGeneration}')

  cat >./"${nm}".yaml <<EOF
spec:
  version: ${newver}
  timeoutSeconds: 600
EOF

  kubectl patch managedcharts.management.cattle.io "${nm}" -n fleet-local --patch-file ./"${nm}".yaml --type merge
  pause_managed_chart "${nm}" "false"
  wait_managed_chart fleet-local "${nm}" "${newver}" "${pre_generation}" ready
}

upgrade_harvester_upgradelog_logging()
{
  local upgradelogname=$1
  local upgradeloguid=$2

  local loggingimg=$(kubectl get logging.logging.banzaicloud.io "${upgradelogname}"-infra -ojsonpath="{.spec.fluentd.image.repository}")

  # previous image is rancher/mirrored-banzaicloud-fluentd
  if [ "${loggingimg}" = "rancher/mirrored-kube-logging-fluentd" ]; then
    echo "logging ${upgradelogname}-infra has been upgraded, nothing to patch"
    return 0
  fi

  echo "delete the current logging-infra"

  local EXIT_CODE=0
  kubectl delete logging.logging.banzaicloud.io "${upgradelogname}"-infra || EXIT_CODE=1
  if [ "${EXIT_CODE}" -gt 0 ]; then
    echo "failed to delete current logging ${upgradelogname}-infra, skip patch"
    return 0
  fi

  # wait at most 60 seconds
  local loop_cnt=10
  while [ $loop_cnt -gt 0 ]
  do
    unset loggingname
    local loggingname=$(kubectl get logging.logging.banzaicloud.io "${upgradelogname}"-infra -ojsonpath="{.metadata.name}")
    if [ -z "${loggingname}" ]; then
      echo "current logging ${upgradelogname}-infra is gone"
      break
    fi
    sleep 6
    loop_cnt=$((loop_cnt-1))
  done

  # no matter logging infra is deleted or not, re-create it
  # the new logging has no fluentbit, which is replaced by fluentbitagent
  echo "logging ${upgradelogname}-infra will be re-created"
  patchfile="patch_logging.yaml"
  cat > "${patchfile}" <<EOF
apiVersion: logging.banzaicloud.io/v1beta1
kind: Logging
metadata:
  labels:
    harvesterhci.io/upgradeLog: ${upgradelogname}
    harvesterhci.io/upgradeLogComponent: infra
  name: ${upgradelogname}-infra
  ownerReferences:
  - apiVersion: harvesterhci.io/v1beta1
    kind: UpgradeLog
    name: ${upgradelogname}
    uid: ${upgradeloguid}
spec:
  configCheck: {}
  controlNamespace: harvester-system
  flowConfigCheckDisabled: true
  fluentd:
    configReloaderImage:
      repository: rancher/mirrored-kube-logging-config-reloader
      tag: v0.0.6
    configReloaderResources: {}
    disablePvc: true
    extraVolumes:
    - containerName: fluentd
      path: /archive
      volume:
        pvc:
          source:
            claimName: log-archive
          spec:
            accessModes:
            - ReadWriteOnce
            resources:
              requests:
                storage: 1Gi
            volumeMode: Filesystem
      volumeName: log-archive
    fluentOutLogrotate:
      age: "10"
      enabled: true
      path: /fluentd/log/out
      size: "10485760"
    image:
      repository: rancher/mirrored-kube-logging-fluentd
      tag: v1.16-4.10-full
    labels:
      harvesterhci.io/upgradeLog: ${upgradelogname}
      harvesterhci.io/upgradeLogComponent: aggregator
    readinessDefaultCheck: {}
    resources: {}
    scaling:
      drain:
        enabled: true
        image: {}
        pauseImage: {}
  loggingRef: harvester-upgradelog
EOF

  kubectl create -f ./"${patchfile}" || echo "failed to create"
  rm -rf ./"${patchfile}"
}

upgrade_harvester_upgradelog_fluentbit_agent()
{
  local upgradelogname=$1
  local upgradeloguid=$2

  local fbagent=$(kubectl get fluentbitagent.logging.banzaicloud.io "${upgradelogname}"-agent -ojsonpath="{.metadata.name}")
  if [[ ! -z "${fbagent}" ]]; then
    echo "fluentbitagent ${upgradelogname}-agent is existing, skip"
    return 0
  fi

  echo "fluentbitagent ${upgradelogname}-agent will be created"
  local patchfile="patch_fluentbit_agent.yaml"
  cat > "${patchfile}" <<EOF
apiVersion: logging.banzaicloud.io/v1beta1
kind: FluentbitAgent
metadata:
  labels:
    harvesterhci.io/upgradeLog: ${upgradelogname}
    harvesterhci.io/upgradeLogComponent: infra
  name: ${upgradelogname}-agent
  ownerReferences:
  - apiVersion: harvesterhci.io/v1beta1
    kind: UpgradeLog
    name: ${upgradelogname}
    uid: ${upgradeloguid}
spec:
  image:
    repository: rancher/mirrored-fluent-fluent-bit
    tag: 3.1.8
  inputTail: {}
  labels:
    harvesterhci.io/upgradeLog: ${upgradelogname}
    harvesterhci.io/upgradeLogComponent: shipper
  loggingRef: harvester-upgradelog
EOF
  kubectl create -f ./"${patchfile}" || echo "failed to create"
  rm -rf ./"${patchfile}"
}

upgrade_harvester_upgradelog_with_patch_logging_fluentd_fluentbit()
{
  local namespace="${UPGRADE_NAMESPACE}"
  local upgradelogname=$(kubectl get upgrades.harvesterhci.io "${HARVESTER_UPGRADE_NAME}" -n "${namespace}" -ojsonpath="{.status.upgradeLog}")

  if [[ -z "${upgradelogname}" ]]; then
    echo "upgradelog is not found from upgrade ${HARVESTER_UPGRADE_NAME}, nothing to patch"
    return 0
  fi

  local upgradeloguid=$(kubectl get upgradelog.harvesterhci.io "${upgradelogname}" -n "${namespace}" -ojsonpath="{.metadata.uid}")
  if [[ -z "${upgradeloguid}" ]]; then
    echo "upgradeloguid is not found from upgradelog ${upgradelogname}, this should not happen, skip"
    return 0
  fi

  # managedchart is upgraded on v150 if it is existing
  upgrade_managedchart_upgradelog_operator "${upgradelogname}" "${upgradeloguid}"

  # logging is deleted & re-created on v150
  upgrade_harvester_upgradelog_logging "${upgradelogname}" "${upgradeloguid}"

  # fluentbitagent is newly created on v150
  upgrade_harvester_upgradelog_fluentbit_agent "${upgradelogname}" "${upgradeloguid}"
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

patch_grafana_nginx_proxy_config_configmap() {
  local EXIT_CODE=0
  local cm=grafana-nginx-proxy-config
  local originValuesfile="/tmp/configmapvalue.yaml"
  rm -f ${originValuesfile}

  echo "try to patch configmap $cm when it exists"

  kubectl get configmap -n cattle-monitoring-system ${cm} -ojsonpath="{.data['nginx\.conf']}" > ${originValuesfile} || EXIT_CODE=$?
  if [[ $EXIT_CODE -gt 0 ]]; then
    # e.g. the rancher-monitoring addon is not enabled
    echo "did not find configmap $cm, skip"
    return 0
  fi

  grep "c-m-" ${originValuesfile} || EXIT_CODE=$?
  if [[ $EXIT_CODE -gt 0 ]]; then
    echo "configmap $cm c-m- has been patched to c-"
     rm -f ${originValuesfile}
    return 0
  fi

  # replace the keyword "c-m-*" with "c-*"
  sed -i -e 's/c-m-/c-/' ${originValuesfile}
  # add 4 spaces to each line
  sed -i -e 's/^/    /' ${originValuesfile}
  local newvalues=$(<${originValuesfile})
  rm -f ${originValuesfile}
  local patchfile="/tmp/configmappatch.yaml"

cat > ${patchfile} <<EOF
data:
  nginx.conf: |
${newvalues}
EOF

  echo "the prepared patch file content"
  cat ${patchfile}

  kubectl patch configmap ${cm} -n cattle-monitoring-system --patch-file ${patchfile} --type merge || echo "patch configmap $cm failed"
  rm -f ${patchfile}

  echo "replace the grafana pod to use the new configmap"
  kubectl delete pods -n cattle-monitoring-system -l app.kubernetes.io/name=grafana || echo "failed to delete the grafana pod, wait until the related host node is rebooted and then it gets the new configmap"
}

# manage_kubeovn will apply the kubeovn-operator-crd managed chart
# and update the kubeovn-addon to latest version
manage_kubeovn() {
  ## read version of kubeovn addon. The kubeovn-operator-crd has the same version as kubeovn-operator
  ## so we can evaluate kubeovn-operator version and apply the same for kubeovn-operator-crd
  version=$(cat /usr/local/share/addons/kubeovn-operator.yaml  | yq '.spec.version')
  kubeovn_manifest="$(mktemp --suffix=.yaml)"

  cat > "$kubeovn_manifest" <<EOF
apiVersion: management.cattle.io/v3
kind: ManagedChart
metadata:
  name: kubeovn-operator-crd
  namespace: fleet-local
spec:
  chart: kubeovn-operator-crd
  releaseName: kubeovn-operator-crd
  version: "$version"
  defaultNamespace: kube-system
  repoName: harvester-charts
  timeoutSeconds: 600
  targets:
  - clusterName: local
    clusterSelector:
      matchExpressions:
      - key: provisioning.cattle.io/unmanaged-system-agent
        operator: DoesNotExist
EOF

  cat "$kubeovn_manifest"
  kubectl apply -f "$kubeovn_manifest"

  # wait for managedchart to be ready before updating the addon
  wait_managedchart_ready "kubeovn-operator-crd"

  ## addon patch 
  patch=$(cat /usr/local/share/addons/kubeovn-operator.yaml | yq '{"spec": .spec | pick(["version"])}')
  cat > addon-patch.yaml <<EOF
$patch
EOF

  item_count=$(kubectl get addons.harvesterhci kubeovn-operator -n kube-system -o  jsonpath='{..name}' || true)
  if [ -z "$item_count" ]; then
    install_addon kubeovn-operator kube-system
  else
    kubectl patch addons.harvesterhci kubeovn-operator -n kube-system --patch-file ./addon-patch.yaml --type merge
  fi

}

# add upgrade related information to managedchart annotations
update_managedchart_patch_file_annotations() {
  local fname=$1
  local tversion=$2
  local utime=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  if [ ! -f ${fname} ]; then
    echo "patch file $fname is not existing, skip update annotations"
    return
  fi
  NEW_UPGRADE=$HARVESTER_UPGRADE_NAME yq e '.metadata.annotations."upgrade.harvesterhci.io/last-upgrade-name" = strenv(NEW_UPGRADE)' ${fname} -i
  NEW_VERSION=${tversion} yq e '.metadata.annotations."upgrade.harvesterhci.io/last-upgrade-target-version" = strenv(NEW_VERSION)' ${fname} -i
  NEW_TIME=${utime} yq e '.metadata.annotations."upgrade.harvesterhci.io/last-upgrade-time" = strenv(NEW_TIME)' ${fname} -i
}

update_managedchart_patch_file_unpause() {
  local fname=$1
  if [ ! -f ${fname} ]; then
    echo "patch file $fname is not existing, skip update paused"
    return
  fi
  yq e '.spec.paused = false' ${fname} -i
}

update_managedchart_patch_file_timeoutseconds() {
  local fname=$1
  if [ ! -f ${fname} ]; then
    echo "patch file $fname is not existing, skip update timeoutSeconds"
    return
  fi

  local cnamespace=$2
  local cname=$3
  local cur_timeout=$(kubectl get managedcharts.management.cattle.io -n ${cnamespace} ${cname} -o=jsonpath='{.spec.timeoutSeconds}')
  # if there is no timeoutSeconds set on managedchart, use value 599; by default harvester-installer sets it to 600
  # otherwise increase it by 1
  local tseconds=$((${cur_timeout:-598} + 1))
  if [ ${tseconds} -gt 800 ]; then
    tseconds=600
  fi
  echo "the ${cnamespace}/${cname} increased timeoutSeconds is ${tseconds}"
  NEW_TIMEOUT=${tseconds} yq e '.spec.timeoutSeconds = env(NEW_TIMEOUT)' ${fname} -i
}
