#!/bin/bash -ex

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

source $SCRIPT_DIR/lib.sh
UPGRADE_TMP_DIR=$HOST_DIR/usr/local/upgrade_tmp
STATE_DIR=$HOST_DIR/run/initramfs/cos-state
MAX_PODS=200

cleanup_incomplete_state_file() {
  STATE_FILE=${STATE_DIR}/state.yaml

  INCOMPLETE_STATE_FILE=0
  if [[ ! -f $STATE_FILE ]]; then
      INCOMPLETE_STATE_FILE=0
  else
    found=$(yq '.state| has("label")' ${STATE_FILE})
    if [[ $found == "false" ]]; then
        INCOMPLETE_STATE_FILE=1
    fi
  fi

  if [[ $INCOMPLETE_STATE_FILE == 1 ]]; then
    echo "Start to remove the incomplete state.yaml"
    mount -o rw,remount ${STATE_DIR}
    cp ${STATE_FILE} ${STATE_FILE}.bak
    rm -f ${STATE_FILE}
    mount -o ro,remount ${STATE_DIR}
  fi
}

is_mounted()
{
  mount | awk -v DIR="$1" '{ if ($3 == DIR) { exit 0 } } ENDFILE { exit 1 }'
}

clean_up_tmp_files()
{
  if [ -n "$tmp_rootfs_mount" ] && is_mounted "$tmp_rootfs_mount"; then
    echo "Try to unmount $tmp_rootfs_mount..."
    umount $tmp_rootfs_mount || echo "Umount $tmp_rootfs_mount failed with return code: $?"
  fi
  echo "Clean up tmp files..."
  if [ -n "$NEW_OS_SQUASHFS_IMAGE_FILE" ]; then
    echo "Try to remove $NEW_OS_SQUASHFS_IMAGE_FILE..."
    rm -vf "$NEW_OS_SQUASHFS_IMAGE_FILE"
  fi
  if [ -n "$tmp_rootfs_squashfs" ]; then
    echo "Try to remove $tmp_rootfs_squashfs..."
    rm -vf "$tmp_rootfs_squashfs"
  fi
}

# Create a systemd service on host to reboot the host if this running pod succeeds.
# This prevents job become from entering `Error`.
reboot_if_job_succeed()
{
  cat > $HOST_DIR/tmp/upgrade-reboot.sh << EOF
#!/bin/bash -ex
HARVESTER_UPGRADE_POD_NAME=$HARVESTER_UPGRADE_POD_NAME

EOF

  cat >> $HOST_DIR/tmp/upgrade-reboot.sh << 'EOF'
source /etc/bash.bashrc.local
pod_id=$(crictl pods --name $HARVESTER_UPGRADE_POD_NAME --namespace harvester-system -o json | jq -er '.items[0].id')

# get `upgrade` container ID
container_id=$(crictl ps --pod $pod_id --name apply -o json -a | jq -er '.containers[0].id')
container_state=$(crictl inspect $container_id | jq -er '.status.state')

if [ "$container_state" = "CONTAINER_EXITED" ]; then
  container_exit_code=$(crictl inspect $container_id | jq -r '.status.exitCode')

  if [ "$container_exit_code" = "0" ]; then
    sleep 10

    # workaround for https://github.com/harvester/harvester/issues/2865
    # kubelet could start from old manifest first and generate a new manifest later.
    rm -f /var/lib/rancher/rke2/agent/pod-manifests/*

    reboot
    exit 0
  fi
fi

exit 1
EOF

  chmod +x $HOST_DIR/tmp/upgrade-reboot.sh

  cat > $HOST_DIR/run/systemd/system/upgrade-reboot.service << 'EOF'
[Unit]
Description=Upgrade reboot
[Service]
Type=simple
ExecStart=/tmp/upgrade-reboot.sh
Restart=always
RestartSec=10
EOF

  chroot $HOST_DIR systemctl daemon-reload
  chroot $HOST_DIR systemctl start upgrade-reboot
}

preload_images()
{
  export CONTAINER_RUNTIME_ENDPOINT=unix:///$HOST_DIR/run/k3s/containerd/containerd.sock
  export CONTAINERD_ADDRESS=$HOST_DIR/run/k3s/containerd/containerd.sock

  CTR="$HOST_DIR/$(readlink $HOST_DIR/var/lib/rancher/rke2/bin)/ctr"
  if [ -z "$CTR" ];then
    echo "Fail to get host ctr binary."
    exit 1
  fi

  import_image_archives_from_repo "common" "$UPGRADE_TMP_DIR"

  # RKE2 images might be needed during nodes upgrade.
  # A waiting-for-upgrade node doesn't load RKE2 images yet because its RKE2
  # server/agent doesn't restart. We need to preload RKE2 images beforehand.
  import_image_archives_from_repo "rke2" "$UPGRADE_TMP_DIR"

  download_image_archives_from_repo "rke2" $HOST_DIR/var/lib/rancher/rke2/agent/images
  download_image_archives_from_repo "agent" $HOST_DIR/var/lib/rancher/agent/images
}

# return the running VM count
# if `kubectl get vmi` failed, then return "failed to get" for troubleshooting
get_running_vm_count()
{
  local vmioutput=/tmp/vmioutput.yaml
  rm -f "$vmioutput"
  local EXIT_CODE=0

  kubectl get vmi -A -l kubevirt.io/nodeName=$HARVESTER_UPGRADE_NODE_NAME -o yaml > "$vmioutput" || EXIT_CODE=$?

  if [[ "$EXIT_CODE" -eq 0 ]]; then
    local count=$(yq e '.items |  map(select(.status.phase!="Succeeded")) | length' "$vmioutput")
    echo "$count"
  else
    echo "failed to get"
  fi
  rm -f "$vmioutput"
}

wait_vms_out()
{
  local vm_count="$(get_running_vm_count)"

  until [ "$vm_count" = "0" ]
  do
    echo "Waiting for VM shutdown...(count: $vm_count)"
    sleep 5
    vm_count="$(get_running_vm_count)"
  done
  echo "all VMs are down"
}

wait_vms_out_or_shutdown()
{
  local vm_count

  while [ true ]; do
    vm_count="$(get_running_vm_count)"

    if [ "$vm_count" = "0" ]; then
      break
    fi

    echo "Waiting for VM live-migration or shutdown...(count: $vm_count)"
    sleep 5
  done
  echo "all VMs on node $HARVESTER_UPGRADE_NODE_NAME have been live-migrated or shutdown"
}

shutdown_repo_vm()
{
  # We don't need to live-migrate upgrade repo VM. Just make sure it's up when we need it.
  # Shutdown it if it's running on this upgrading node.
  repo_vm_name="upgrade-repo-$HARVESTER_UPGRADE_NAME"
  repo_node=$(kubectl get vmi -n harvester-system $repo_vm_name -o yaml | yq -e e '.status.nodeName' -)
  if [ "$repo_node" = "$HARVESTER_UPGRADE_NODE_NAME" ]; then
    echo "Stop upgrade repo VM: $repo_vm_name"
    virtctl stop $repo_vm_name -n harvester-system
  fi
}

shutdown_vms_on_node()
{
  kubectl get vmi -A -l kubevirt.io/nodeName=$HARVESTER_UPGRADE_NODE_NAME -o json |
    jq -r '.items[] | [.metadata.name, .metadata.namespace] | @tsv' |
    while IFS=$'\t' read -r name namespace; do
      if [ -z "$name" ]; then
        break
      fi
      echo "Stop ${namespace}/${name}"
      virtctl stop $name -n $namespace
    done
}

command_prepare()
{
  wait_repo
  detect_repo
  detect_upgrade
  check_version
  preload_images
}

wait_evacuation_pdb_gone()
{
  # TODO: fine-tune this to per-VM check
  until ! kubectl get pdb -o name -A | grep kubevirt-migration-pdb-kubevirt-evacuation-
  do
    echo "Waiting for evacuation PDB gone..."
    sleep 5
  done
}

recover_rancher_system_agent() {
  chroot "$HOST_DIR" /bin/bash -c "rm -rf /run/systemd/system/rancher-system-agent.service.d && systemctl daemon-reload && systemctl restart rancher-system-agent.service"
}

wait_longhorn_engines() {
  node_count=$(kubectl get nodes --selector=harvesterhci.io/managed=true -o json | jq -r '.items | length')

  # For each running engine and its volume
  kubectl get engines.longhorn.io -n longhorn-system -o json |
    jq -r '.items | map(select(.status.currentState == "running")) | map(.metadata.name + " " + .metadata.labels.longhornvolume) | .[]' |
    while read -r lh_engine lh_volume; do
      echo Checking running engine "${lh_engine}..."

      # Wait until volume turn healthy (except two-node clusters)
      while [ true ]; do
        if [ $node_count -gt 2 ];then
          robustness=$(kubectl get volumes.longhorn.io/$lh_volume -n longhorn-system -o jsonpath='{.status.robustness}')
          if [ "$robustness" = "healthy" ]; then
            echo "Volume $lh_volume is healthy."
            break
          fi
        else
          # two node situation, make sure maximum two replicas are healthy
          expected_replicas=2

          # replica 1 case
          volume_replicas=$(kubectl get volumes.longhorn.io/$lh_volume -n longhorn-system -o jsonpath='{.spec.numberOfReplicas}')
          if [ $volume_replicas -eq 1 ]; then
            expected_replicas=1
          fi

          ready_replicas=$(kubectl get engines.longhorn.io/$lh_engine -n longhorn-system -o json |
                             jq -r '.status.replicaModeMap | to_entries | map(select(.value == "RW")) | length')
          if [ $ready_replicas -ge $expected_replicas ]; then
            break
          fi
        fi

        if [ -f "/tmp/skip-$lh_volume" ]; then
          echo "Skip $lh_volume."
          break
        fi

        echo "Waiting for volume $lh_volume to be healthy..."
        sleep 10
      done
    done
}

patch_logging_event_audit()
{
  # enabling logging, audit, event when upgrading from v1.0.3 to 1.1.0
  # should happen before RKE2 is patched
  # note: the host '/' is mapped to pod '/host/', refer: pkg/controller/master/upgrade/common.go applyNodeJob

  # get UPGRADE_PREVIOUS_VERSION and NODE_CURRENT_HARVESTER_VERSION
  detect_upgrade
  detect_node_current_harvester_version
  echo "The UPGRADE_PREVIOUS_VERSION is $UPGRADE_PREVIOUS_VERSION, NODE_CURRENT_HARVESTER_VERSION is $NODE_CURRENT_HARVESTER_VERSION, will check Logging Event Audit upgrade node option"

  if test "$UPGRADE_PREVIOUS_VERSION" = "v1.0.3" || test "$NODE_CURRENT_HARVESTER_VERSION" = "v1.0.3"; then
    echo "Patch kube-audit policy file"
    # this file should be there in each NODE
    # keep syncing with harvester/harvester-installer/pkg/config/templates/rke2-92-harvester-kube-audit-policy.yaml

    ls /host/etc/rancher/rke2/config.yaml.d/ -alt

    KUBE_AUDIT_POLICY_FILE_IN_CONTAINER=/host/etc/rancher/rke2/config.yaml.d/92-harvester-kube-audit-policy.yaml
    KUBE_AUDIT_POLICY_FILE_IN_HOST=/etc/rancher/rke2/config.yaml.d/92-harvester-kube-audit-policy.yaml

    if [ ! -f $KUBE_AUDIT_POLICY_FILE_IN_CONTAINER ]; then
      echo "Create new policy file $KUBE_AUDIT_POLICY_FILE_IN_CONTAINER"

      cat > $KUBE_AUDIT_POLICY_FILE_IN_CONTAINER << 'EOF'
apiVersion: audit.k8s.io/v1
kind: Policy
omitStages:
  - "ResponseStarted"
  - "ResponseComplete"
rules:
  # Any include/exclude rules are added here

  # A catch-all rule to log all other (create/delete/patch) requests at the Metadata level
  - level: Metadata
    verbs: ["create", "delete", "patch"]
    omitStages:
      - "ResponseStarted"
      - "ResponseComplete"
EOF

    else
      echo "Reuse the existing policy file $KUBE_AUDIT_POLICY_FILE_IN_CONTAINER, the file content is:"
      cat $KUBE_AUDIT_POLICY_FILE_IN_CONTAINER
    fi

    # it means the NODE role is rke2-server when 90-harvester-server.yaml exists, patch it
    RKE2_SERVER_CONFIG_FILE_IN_CONTAINER=/host/etc/rancher/rke2/config.yaml.d/90-harvester-server.yaml
    PATCH_SERVER_IN_CUSTOM=1
    local param=audit-policy-file

    if [ -f "$RKE2_SERVER_CONFIG_FILE_IN_CONTAINER" ]; then
      local audit_policy_file_param=$(yq e '.'$param $RKE2_SERVER_CONFIG_FILE_IN_CONTAINER)

      if [ "$audit_policy_file_param" == "null" ]; then
        echo "$param parameter is not in $RKE2_SERVER_CONFIG_FILE_IN_CONTAINER"
        echo "Patch rke2 server config file with $param parameter"
        echo "$param: $KUBE_AUDIT_POLICY_FILE_IN_HOST" >> $RKE2_SERVER_CONFIG_FILE_IN_CONTAINER
        echo "After patch, the file content is"
        cat $RKE2_SERVER_CONFIG_FILE_IN_CONTAINER
      else
        echo "$param parameter is in $RKE2_SERVER_CONFIG_FILE_IN_CONTAINER, value: $audit_policy_file_param, skip patch"
      fi

    else
      # for rke2-agent node, do nothing now, this file will be patched when the node is promoted to server
      echo "There is no file $RKE2_SERVER_CONFIG_FILE_IN_CONTAINER, $param parameter is not patched"
      PATCH_SERVER_IN_CUSTOM=0
    fi

    #patch /oem/99_custom.yaml in host
    source $SCRIPT_DIR/patch_99_custom.sh
    SRC_FILE=/host/oem/99_custom.yaml
    TMP_FILE=/host/oem/99_custom_tmp.yaml # will be created and deleted
    patch_99_custom $SRC_FILE $TMP_FILE $PATCH_SERVER_IN_CUSTOM
  else
    echo "Logging Event Audit: nothing to do in $UPGRADE_PREVIOUS_VERSION"
  fi
}

command_pre_drain() {
  recover_rancher_system_agent

  wait_longhorn_engines

  # Shut down non-live migratable VMs
  upgrade-helper vm-live-migrate-detector "$HARVESTER_UPGRADE_NODE_NAME" --shutdown

  # Live migrate VMs
  kubectl taint node $HARVESTER_UPGRADE_NODE_NAME --overwrite kubevirt.io/drain=draining:NoSchedule

  # Wait for VM migrated
  wait_vms_out_or_shutdown

  # KubeVirt's pdb might cause drain fail
  wait_evacuation_pdb_gone

  # Add logging related kube-audit policy file
  patch_logging_event_audit

  remove_rke2_canal_config
  disable_rke2_charts
}

get_node_rke2_version() {
  kubectl get node $HARVESTER_UPGRADE_NODE_NAME -o yaml | yq -e e '.status.nodeInfo.kubeletVersion' -
}

upgrade_rke2() {
  patch_file=$(mktemp -p $UPGRADE_TMP_DIR)

cat > $patch_file <<EOF
spec:
  kubernetesVersion: $REPO_RKE2_VERSION
  rkeConfig: {}
EOF

  kubectl patch clusters.provisioning.cattle.io local -n fleet-local --patch-file $patch_file --type merge
}

wait_rke2_upgrade() {
  # RKE2 doesn't show '-rcX' in nodeInfo, so we remove '-rcX' in $REPO_RKE2_VERSION.
  # Warning: we can't upgrade from a '-rcX' to another in the same minor version like v1.22.12-rc1+rke2r1 to v1.22.12-rc2+rke2r1.
  REPO_RKE2_VERSION_WITHOUT_RC=$(echo -n $REPO_RKE2_VERSION | sed 's/-rc[[:digit:]]*//g') 
  until [ "$(get_node_rke2_version)" = "$REPO_RKE2_VERSION_WITHOUT_RC" ]
  do
    echo "Waiting for RKE2 to be upgraded..."
    sleep 5
  done
}

clean_rke2_archives() {
  yq -e -o=json e ".images.rke2" "$CACHED_BUNDLE_METADATA" | jq -r '.[] | [.list, .archive] | @tsv' |
    while IFS=$'\t' read -r list archive; do
      rm -f "$HOST_DIR/var/lib/rancher/rke2/agent/images/$(basename $archive)"
      rm -f "$HOST_DIR/var/lib/rancher/rke2/agent/images/$(basename $list)"
    done
}

remove_rke2_canal_config() {
  rm -f "$HOST_DIR/var/lib/rancher/rke2/server/manifests/rke2-canal-config.yaml"
}

# Disable snapshot-controller related charts because we manage them in Harvester.
# RKE2 enables these charts by default after v1.25.7 (https://github.com/rancher/rke2/releases/tag/v1.25.7%2Brke2r1)
disable_rke2_charts() {
  cat > "$HOST_DIR/etc/rancher/rke2/config.yaml.d/40-disable-charts.yaml" <<EOF
disable:
- rke2-snapshot-controller
- rke2-snapshot-controller-crd
- rke2-snapshot-validation-webhook
EOF
}

convert_nodenetwork_to_vlanconfig() {
  # sometimes (e.g. apiserver is busy/not ready), the kubectl may fail, use the saved value
  if [ -z "$UPGRADE_PREVIOUS_VERSION" ]; then
    detect_upgrade
  fi
  detect_node_current_harvester_version
  echo "The UPGRADE_PREVIOUS_VERSION is $UPGRADE_PREVIOUS_VERSION, NODE_CURRENT_HARVESTER_VERSION is $NODE_CURRENT_HARVESTER_VERSION, will check nodenetwork upgrade node option"

  if test "$UPGRADE_PREVIOUS_VERSION" != "v1.0.3" && test "$NODE_CURRENT_HARVESTER_VERSION" != "v1.0.3"; then
    echo "There is nothing to do in nodenetwork"
    return
  fi

  # Don't need to convert nodenetwork if harvester-mgmt is used as vlan interface in any nodenetwork
  [[ $(kubectl get nodenetwork -o yaml | yq '.items[].spec.nic | select(. == "harvester-mgmt")') ]] &&
  echo "Don't need to convert nodenetwork because harvester-mgmt is used as VLAN interface in not less than one nodenetwork" && return

  local name=${HARVESTER_UPGRADE_NODE_NAME}-vlan
  local EXIT_CODE=$?
  # when object is not existing, kubectl will return 1
  nn=$(kubectl get nodenetwork "$name" -o yaml) || EXIT_CODE=$?
  if [ $EXIT_CODE != 0 ]; then
    echo "Cannot find nodenetwork $name, skip patch"
    return
  fi

  vlan_interface=$(echo "$nn" | yq '.spec.nic')

  # the default ${HARVESTER_UPGRADE_NODE_NAME}-vlan has no nics
  [[ "$vlan_interface" == "null" ]] && echo "Default/invalid nodenetwork $name without any nics, skip patch" && return

  type=$(echo "$nn" | yq e '.spec.nic as $nic | .status.networkLinkStatus.[$nic].type')
  if [ "$type" == "bond" ]; then
    vlan_interface_details=$(v="$vlan_interface" yq '.install.networks.[env(v)]' $HOST_DIR/oem/harvester.config)
    nics=($(echo "$vlan_interface_details" | yq '.interfaces[].name'))
    mtu=$(echo "$vlan_interface_details" | yq '.mtu')
    mode=$(echo "$vlan_interface_details" | yq '.bondoptions.mode')
    miimon=$(echo "$vlan_interface_details" | yq '.bondoptions.miimon')
  else
    nics=("$vlan_interface")
    mtu=0
    mode="active-backup"
    miimon=0
  fi

  # below code is idempotent

  kubectl apply -f - <<EOF
apiVersion: network.harvesterhci.io/v1beta1
kind: VlanConfig
metadata:
  name: $name
spec:
  clusterNetwork: vlan
  nodeSelector:
    kubernetes.io/hostname: "$HARVESTER_UPGRADE_NODE_NAME"
  uplink:
    bondOptions:
      mode: $mode
      miimon: $miimon
    linkAttributes:
      mtu: $mtu
    nics: [$(IFS=,; echo "${nics[*]}")]
EOF
}

# host / is mounted under /host in the upgrade pod
set_max_pods() {
cat > /host/etc/rancher/rke2/config.yaml.d/99-max-pods.yaml <<EOF
kubelet-arg:
- "max-pods=$MAX_PODS"
EOF
}

# system-reserved/kube-reserved is required if we want to set cpu-manager-policy to static
set_reserved_resource() {
  local cores=$(chroot $HOST_DIR nproc)
  local reservedCPU=$(calculateCPUReservedInMilliCPU $cores $MAX_PODS)
  # make system:kube cpu reservation ration 2:3
  local systemReservedCPU=$((reservedCPU * 2 * 2 / 5))
  local kubeReservedCPU=$((reservedCPU * 2 * 3 / 5))
  local systemReserverConfig="$HOST_DIR/etc/rancher/rke2/config.yaml.d/99-z00-harvester-reserved-resources.yaml"
  if [ ! -e $systemReserverConfig ]; then
    cat > $systemReserverConfig << EOF
kubelet-arg+:
- "system-reserved=cpu=${systemReservedCPU}m"
- "kube-reserved=cpu=${kubeReservedCPU}m"
EOF
  fi
}

# Delete the cpu_manager_state file during the initramfs stage. During a reboot, this state file is always reverted
# because it was originally created during the system installation, becoming part of the root filesystem. As a result,
# the policy in cpu_manager_state file is "none" (default policy) after reboot. If we've already set the cpu-manager-policy
# to "static" before reboot, this mismatch can prevent kubelet from starting, and make the entire node unavailable.
set_oem_cleanup_kubelet() {
  cat > $HOST_DIR/oem/91_cleanup_kubelet.yaml << EOF
name: Cleanup Kubelet
stages:
    initramfs:
        - commands:
            - rm -f /var/lib/kubelet/cpu_manager_state
EOF
}

calculateCPUReservedInMilliCPU() {
  local cores=$1
  local maxPods=$2

  # This shouldn't happen
  if (( $cores <= 0 || $maxPods <= 0 )); then
    echo 0
    return
  fi

  local reserved=0

  # 6% of the first core (60 milliCPU)
  reserved=$(( $reserved + 6 * 1000 / 100 ))

  # 1% of the next core (up to 2 cores) (10 milliCPU)
  if (( cores > 1 )); then
    reserved=$(( $reserved + 1 * 1000 / 100 ))
  fi

  # 0.5% of the next 2 cores (up to 4 cores) (5 milliCPU)
  if (( cores > 2 )); then
    reserved=$(( $reserved + 2 * 500 / 100 ))
  fi

  # 0.25% of any cores above 4 cores (2.5 milliCPU per core)
  if (( cores > 4 )); then
    reserved=$(( $reserved + (cores - 4) * 250 / 100 ))
  fi

  # If the maximum number of Pods per node is beyond the default of 110,
  # reserve an extra 400 mCPU in addition to the preceding reservations.
  if (( maxPods > 110 )); then
    reserved=$(( $reserved + 400 ))
  fi

  echo $reserved
}

upgrade_os() {
  # The trap will be only effective from this point to the end of the execution
  trap clean_up_tmp_files EXIT

  CURRENT_OS_VERSION=$(source $HOST_DIR/etc/os-release && echo $PRETTY_NAME)

  if [ "$REPO_OS_PRETTY_NAME" = "$CURRENT_OS_VERSION" ]; then
    echo "Skip upgrading OS. The OS version is already \"$CURRENT_OS_VERSION\"."
    return
  fi
  
  # upgrade OS image and reboot
  if [ -n "$NEW_OS_SQUASHFS_IMAGE_FILE" ]; then
    tmp_rootfs_squashfs="$NEW_OS_SQUASHFS_IMAGE_FILE"
  else
    tmp_rootfs_squashfs=$(mktemp -p $UPGRADE_TMP_DIR)
    download_file "$UPGRADE_REPO_SQUASHFS_IMAGE" "$tmp_rootfs_squashfs"
  fi

  tmp_rootfs_mount=$(mktemp -d -p $HOST_DIR/tmp)
  mount $tmp_rootfs_squashfs $tmp_rootfs_mount

  tmp_elemental_config_dir=$(mktemp -d -p $HOST_DIR/tmp)
  # adjust new active.img size
  cat > $tmp_elemental_config_dir/config.yaml <<EOF
upgrade:
  system:
    size: 3072
EOF

  # we would like to clean up the incomplete state.yaml to avoid the issue of https://github.com/harvester/harvester/issues/4526
  cleanup_incomplete_state_file

  elemental_upgrade_log="${UPGRADE_TMP_DIR#"$HOST_DIR"}/elemental-upgrade-$(date +%Y%m%d%H%M%S).log"
  local ret=0
  chroot $HOST_DIR elemental upgrade \
    --logfile "$elemental_upgrade_log" \
    --directory ${tmp_rootfs_mount#"$HOST_DIR"} \
    --config-dir ${tmp_elemental_config_dir#"$HOST_DIR"} \
    --debug || ret=$?
  if [ "$ret" != 0 ]; then
    echo "elemental upgrade failed with return code: $ret"
    cat "$HOST_DIR$elemental_upgrade_log"
    exit "$ret"
  fi

  # For some firmware (e.g., Dell BOSS adapter 3022), users may get
  # stuck on the grub file search for about 30 minutes, this can be
  # mitigated by adding the `grubenv` file.
  #
  # We need to patch grubenv, grubcustom

  # PATCH1: add /oem/grubenv if it does not exist
  # grubenv use load_env to load, so we use grub2-editenv
  GRUBENV_FILE="/oem/grubenv"
  chroot $HOST_DIR /bin/bash -c "if ! [ -f ${GRUBENV_FILE} ]; then grub2-editenv ${GRUBENV_FILE} create; fi"

  # PATCH2: add /oem/grubcustom if it does not exist
  # grubcustom use source to load, so we can use touch directly
  GRUBENV_FILE="/oem/grubcustom"
  chroot $HOST_DIR /bin/bash -c "if ! [ -f ${GRUBENV_FILE} ]; then touch ${GRUBENV_FILE}; fi" 

  multiPathEnabled=$(yq '.os.externalStorageConfig.enabled // false' ${HOST_DIR}/oem/harvester.config)
  if [ ${multiPathEnabled} == false ]
  then
    thirdPartyArgs=$(chroot $HOST_DIR grub2-editenv /oem/grubenv list |grep third_party_kernel_args | awk -F"third_party_kernel_args=" '{print $2}')
    if [[ ${thirdPartyArgs} != *"multipath=off"* ]]
    then
      thirdPartyArgs="${thirdPartyArgs} multipath=off"
      thirdPartyArgs=$(echo ${thirdPartyArgs} | xargs)
      chroot $HOST_DIR grub2-editenv /oem/grubenv set third_party_kernel_args="${thirdPartyArgs}"
    fi
  fi

  umount $tmp_rootfs_mount
  rm -rf $tmp_rootfs_squashfs

  reboot_if_job_succeed
}

start_repo_vm() {
  repo_vm=$(kubectl get vm -l "harvesterhci.io/upgrade=$HARVESTER_UPGRADE_NAME" -n harvester-system -o yaml | yq -e e '.items[0].metadata.name' -)
  if [ -z $repo_vm ]; then
    echo "Fail to get upgrade repo VM name."
    exit 1
  fi

  virtctl start $repo_vm -n harvester-system || true
}

command_post_drain() {
  wait_repo
  detect_repo

  # update max-pods to 200
  set_max_pods
  set_reserved_resource
  set_oem_cleanup_kubelet
  # A post-drain signal from Rancher doesn't mean RKE2 agent/server is already patched and restarted
  # Let's wait until the RKE2 settled.
  wait_rke2_upgrade
  clean_rke2_archives

  kubectl taint node $HARVESTER_UPGRADE_NODE_NAME kubevirt.io/drain- || true

  convert_nodenetwork_to_vlanconfig

  upgrade_os
}

command_single_node_upgrade() {
  echo "Upgrade single node"

  recover_rancher_system_agent

  wait_repo
  detect_repo

  remove_rke2_canal_config
  disable_rke2_charts

  # Copy OS things, we need to shutdown repo VMs.
  NEW_OS_SQUASHFS_IMAGE_FILE=$(mktemp -p $UPGRADE_TMP_DIR)
  download_file "$UPGRADE_REPO_SQUASHFS_IMAGE" "$NEW_OS_SQUASHFS_IMAGE_FILE"

  # Stop all VMs
  shutdown_all_vms
  wait_vms_out

  # Add logging related kube-audit policy file
  patch_logging_event_audit

  echo "wait for fleet bundles before upgrading RKE2"
  # wait all fleet bundles in limited time
  wait_for_fleet_bundles

  # update max-pods to 200
  set_max_pods
  set_reserved_resource
  set_oem_cleanup_kubelet
  # Upgarde RKE2
  upgrade_rke2

  wait_rke2_upgrade
  clean_rke2_archives

  convert_nodenetwork_to_vlanconfig

  # Upgrade OS
  upgrade_os
}

mkdir -p $UPGRADE_TMP_DIR

case $1 in
  prepare)
    command_prepare
    ;;
  pre-drain)
    command_pre_drain
    ;;
  post-drain)
    command_post_drain
    ;;
  single-node-upgrade)
    command_single_node_upgrade
    ;;
esac
