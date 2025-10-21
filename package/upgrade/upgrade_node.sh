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

sparsify_passive_img()
{
  # See https://github.com/harvester/harvester/issues/7518
  PASSIVE_IMG=${STATE_DIR}/cOS/passive.img
  if [ -f $PASSIVE_IMG ]; then
    echo "Ensuring $PASSIVE_IMG is sparse..."
    mount -o rw,remount ${STATE_DIR}
    echo "  was: $(du -h $PASSIVE_IMG)"
    fallocate --dig-holes $PASSIVE_IMG
    echo "  now: $(du -h $PASSIVE_IMG)"
    mount -o ro,remount ${STATE_DIR}
  else
    echo "$PASSIVE_IMG does not exist"
  fi
}

# elemental scans /dev for blockdevices and checks for their mount points
# if the disk is already mounted then the current logic remounts them as read/write partition
# and if a partition is not mounted elemental tries to just mount it as a read/write partition
# in case of multipath devices, the /dev/mapper device is not detected, and elemental
# finds the path making the disk as not mounted.
# due to change introduced by https://github.com/rancher/elemental-toolkit/pull/2302 elemental reconciles
# the actual disk partition in use and finds the correct mapper device related to the cos partition
# however the logic still scans the underlying disk for mount path
# elemental assumes disk is not mounted and tries to mount the mapper device to a new partition
# in the case, below COS_STATE is normally mounted as a readonly partition under /run/initramfs/cos-state
# when multipath is enabled for boot disks, elemental tries to mount COS_STATE under /run/cos/state
# the partition is mounted as readonly partition as elemental is not aware that the partition is already mounted
# this results in upgrade being broken since the partition is mounted in readonly state, and we need to ensure
# remount option is select. Mounting the partition under /run/cos/state works around this,
# as elemental now finds a disk mounted under /run/cos/state and uses the `remount` option to mount the partition
check_and_mount_state(){
  local MAPPER_IN_USE=$(chroot ${HOST_DIR} blkid -L COS_STATE | grep mapper)
  if [ -n "$MAPPER_IN_USE" ]; then
    echo "mapper devices in use, performing mount of COS_STATE partition"
    mkdir -p ${HOST_DIR}/run/cos/state
    chroot ${HOST_DIR} mount -L COS_STATE /run/cos/state
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

  if is_mounted "/run/cos/state"; then
    echo "Trying to unmount /run/cos/state"
    umount /run/cos/state || echo "Umount /run/cos/state failed with return code: $?"
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

    check_migrations_phase "Scheduling"
    check_migrations_phase "Pending"
    check_migrations_phase "Failed"

    echo "Waiting for VM live-migration or shutdown...(count: $vm_count)"
    sleep 5
  done
  echo "all VMs on node $HARVESTER_UPGRADE_NODE_NAME have been live-migrated or shutdown"
}

check_migrations_phase() {
  local migration_phase="${1}"
  local running_vmis=($(kubectl get vmi -A -lkubevirt.io/nodeName="${HARVESTER_UPGRADE_NODE_NAME}" -ojsonpath='{range .items[?(@.status.phase=="Running")]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}'))
  for vmi in "${running_vmis[@]}"; do
    vmi_namespace=$(echo "${vmi}" | cut -d '/' -f1)
    vmi_name=$(echo "${vmi}" | cut -d '/' -f2)
    vmims=($(kubectl get vmim -n "${vmi_namespace}" -lkubevirt.io/vmi-name=${vmi_name} -ojsonpath="{.items[?(@.status.phase=='${migration_phase}')].metadata.name}"))
    for vmim in "${vmims[@]}"; do
      # filter out non-upgrade related vmims to avoid shutting down the vm due
      # to any previous unrelated pre-upgrade migration failures.
      # the vmim's name prefix is used as the filter because the vmim doesn't
      # have any labels or annotations that reference the upgrade resource.
      # all upgrade-related vmims have the 'kubevirt-evacuation-' prefix.
      if [[ "${vmim}" != "kubevirt-evacuation"* ]]; then
        echo "skipping non-upgrade related vmim ${vmim}"
        continue
      fi

      echo "found virtual machine migration. phase:${migration_phase}, node:${HARVESTER_UPGRADE_NODE_NAME}, ns: ${vmi_namespace}, vm:${vmi_name}, vmim:${vmim}"

      # if the warning events confirmed a migration failure, shutdown this vm
      if [ "${migration_phase}" = "Failed" ]; then
        warnings=$(kubectl -n "${vmi_namespace}" events --for=virtualmachineinstancemigration/"${vmim}" --types=warning -ojsonpath='{.items[*].reason}' 2>/dev/null)
        if [ ! -z "${warnings}" ]; then
          if echo "${warnings}" | grep -i -q "failedmigration"; then
            echo "shutting down virtual machine ${vmi_namespace}/${vmi_name} due to failed migration. vmim: ${vmi_namespace}/${vmim}, reasons: ${warnings}"
            virtctl -n "${vmi_namespace}" stop "${vmi_name}" || true # don't fail upgrade if virtctl failed
          fi
        fi
      fi
    done
  done
}

shutdown_repo_vm() {
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
  local node_count=$(kubectl get nodes --selector=harvesterhci.io/managed=true,node-role.harvesterhci.io/witness!=true -o json | jq -r '.items | length')

  while true; do
    local unhealthy_found=0

    # For each running engine and its volume
    while read -r lh_engine lh_volume; do
      echo Checking running engine "${lh_engine}..."

      if [ -f "/tmp/skip-$lh_volume" ]; then
        echo "Skip $lh_volume."
        continue
      fi

      # Wait until volume turn healthy (except two-node clusters)
      if [ $node_count -gt 2 ];then
        robustness=$(kubectl get volumes.longhorn.io/$lh_volume -n longhorn-system -o jsonpath='{.status.robustness}')
        if [ "$robustness" != "healthy" ]; then
          echo "Volume $lh_volume is not healthy."
          unhealthy_found=1
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
                            jq -r '.status.replicaModeMap // {} | to_entries | map(select(.value == "RW")) | length')
        if [ $ready_replicas -lt $expected_replicas ]; then
          echo "Volume $lh_volume does not have enough healthy replicas."
          unhealthy_found=1
          break
        fi
      fi
    done < <(
      kubectl get engines.longhorn.io -n longhorn-system -o json |
        jq -r '.items | map(select(.status.currentState == "running")) | map(.metadata.name + " " + .metadata.labels.longhornvolume) | .[]'
    )

    if [ $unhealthy_found -eq 0 ]; then
      echo "All Longhorn volumes are healthy."
      break
    fi

    echo "Waiting for all Longhorn volumes to be healthy..."
    sleep 10
  done
}

command_pre_drain() {
  recover_rancher_system_agent

  wait_longhorn_engines

  # Shut down non-live migratable VMs
  upgrade-helper vm-live-migrate-detector "$HARVESTER_UPGRADE_NODE_NAME" --shutdown --upgrade "$HARVESTER_UPGRADE_NAME"

  # Live migrate VMs
  kubectl taint node $HARVESTER_UPGRADE_NODE_NAME --overwrite kubevirt.io/drain=draining:NoSchedule

  # Wait for VM migrated
  wait_vms_out_or_shutdown

  # KubeVirt's pdb might cause drain fail
  wait_evacuation_pdb_gone

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

set_rke2_device_permissions() {
  local rke2DevicePermissionConfig="$HOST_DIR/etc/rancher/rke2/config.yaml.d/91-harvester-cdi.yaml"
  if [ ! -e $rke2DevicePermissionConfig ]; then
    cat > $rke2DevicePermissionConfig << EOF
# handle the permission issue of Longhorn for CDI
"nonroot-devices": true
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

generate_networkmanager_config() {
   if [ -z "$UPGRADE_PREVIOUS_VERSION" ]; then
    detect_upgrade
  fi

  # NetworkManager is new in Harvester v1.7.0, and we can only upgrade to
  # v1.7.x from v1.6.x, which means we only need to generate NetworkManager
  # config if the previous version is v1.6.x.
  if [[ ! "$UPGRADE_PREVIOUS_VERSION" =~ ^v1\.6\.[0-9]$ ]]; then
    echo "version: $UPGRADE_PREVIOUS_VERSION does not require generating NetworkManager config"
    return
  fi

  # Just in case NetworkManager config has already been generated
  # and/or potentially modified by the user, let's not overwrite it.
  if [ -e ${HOST_DIR}/oem/91_networkmanager.yaml ]; then
    echo "skipping NetworkManager config generation (${HOST_DIR}/oem/91_networkmanager.yaml already exists)"
    return
  fi

  echo "Generating NetworkManager config..."
  # Whether this succeeds or fails, it will print a message either way...
  /usr/local/bin/harvester-installer generate-network-yaml --config ${HOST_DIR}/oem/harvester.config --cloud-init ${HOST_DIR}/oem/91_networkmanager.yaml 2>&1
  # ...but because we're running with set -e, if the above fails, the script
  # will abort, and the rest of the OS upgrade will not proceed.  If that
  # happens, the upgrade job will probably be re-run indefinitely.  Is it
  # better to get stuck here in that way?  Or would it be better to barrel
  # on regardless and continue the OS upgrade with the knowledge that
  # when the node comes back up after reboot, networking may be broken?
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

  # make sure the current passive image isn't using too much disk space
  sparsify_passive_img

  # perform mount of /run/cos/state if needed when multipath is being used for boot
  check_and_mount_state

  # copy elemental from upgrade image so latest elemental binary is used for upgrades.
  cp /usr/local/bin/elemental $HOST_DIR/tmp/elemental

  elemental_upgrade_log="${UPGRADE_TMP_DIR#"$HOST_DIR"}/elemental-upgrade-$(date +%Y%m%d%H%M%S).log"
  local ret=0
  # elemental-toolkit built for SL Micro 6.1 needs a newer glibc than
  # is available on SLE Micro 5.5 hosts.  We can work around this by
  # bind mounting /lib64 from this newer container into the host
  # environment, but we only want to do this if we know there's a
  # problem (if the host is already running SL Micro 6.1, then bind
  # mounting /lib64 from a SLE 15 SP7 container will actually break
  # things).
  local glibc_too_old=$(chroot $HOST_DIR /tmp/elemental version 2>&1 | grep 'GLIBC.*not found')
  if [ -n "$glibc_too_old" ]; then
    echo "GLIBC on host is too old for new elemental build; bind mounting /lib64 to fix"
    mount -o bind /lib64 $HOST_DIR/lib64
  fi
  chroot $HOST_DIR /tmp/elemental upgrade \
    --logfile "$elemental_upgrade_log" \
    --directory ${tmp_rootfs_mount#"$HOST_DIR"} \
    --config-dir ${tmp_elemental_config_dir#"$HOST_DIR"} \
    --debug || ret=$?
  if [ -n "$glibc_too_old" ]; then
    umount $HOST_DIR/lib64
  fi
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

  multiPathEnabled=$(yq '.os.externalstorage.enabled // false' ${HOST_DIR}/oem/harvester.config)
  if [ ${multiPathEnabled} == false ]
  then
    thirdPartyArgs=$(chroot $HOST_DIR grub2-editenv /oem/grubenv list |grep third_party_kernel_args | awk -F"third_party_kernel_args=" '{print $2}')
    # tweaked check to ensure the multipath arguments are only added in 1.6.x if they are not present
    # users may have multipath=on for externalStorageSupport and we need to respect that
    if [[ ${thirdPartyArgs} != *"multipath"* ]]
    then
      thirdPartyArgs="${thirdPartyArgs} multipath=off"
      thirdPartyArgs=$(echo ${thirdPartyArgs} | xargs)
      chroot $HOST_DIR grub2-editenv /oem/grubenv set third_party_kernel_args="${thirdPartyArgs}"
    fi
    # add cloud-init directive to disable multipathing for longhorn
    cat > ${HOST_DIR}/oem/99_disable_lh_multipathd.yaml << EOF
name: "disable longhorn multipathing"
stages:
   initramfs:
     - directories:
       - path: "/etc/multipath/conf.d"
         permissions: 0644
         owner: 0
         group: 0
     - files:
       - path: "/etc/multipath/conf.d/99-longhorn.conf"
         content: YmxhY2tsaXN0IHsgCiAgZGV2aWNlIHsgCiAgICB2ZW5kb3IgIklFVCIgCiAgICBwcm9kdWN0ICJWSVJUVUFMLURJU0siCiAgfQp9Cg==
         encoding: "base64"
         permissions: 0644
         owner: 0
         group: 0
       - path: /etc/systemd/system/multipathd.service
         permissions: 420
         owner: 0
         group: 0
         content: W1VuaXRdCkRlc2NyaXB0aW9uPURldmljZS1NYXBwZXIgTXVsdGlwYXRoIERldmljZSBDb250cm9sbGVyCkJlZm9yZT1sdm0yLWFjdGl2YXRpb24tZWFybHkuc2VydmljZQpCZWZvcmU9bG9jYWwtZnMtcHJlLnRhcmdldCBibGstYXZhaWxhYmlsaXR5LnNlcnZpY2Ugc2h1dGRvd24udGFyZ2V0CldhbnRzPXN5c3RlbWQtdWRldmQta2VybmVsLnNvY2tldApBZnRlcj1zeXN0ZW1kLXVkZXZkLWtlcm5lbC5zb2NrZXQKQWZ0ZXI9bXVsdGlwYXRoZC5zb2NrZXQgc3lzdGVtZC1yZW1vdW50LWZzLnNlcnZpY2UKQmVmb3JlPWluaXRyZC1jbGVhbnVwLnNlcnZpY2UKRGVmYXVsdERlcGVuZGVuY2llcz1ubwpDb25mbGljdHM9c2h1dGRvd24udGFyZ2V0CkNvbmZsaWN0cz1pbml0cmQtY2xlYW51cC5zZXJ2aWNlCkNvbmRpdGlvbktlcm5lbENvbW1hbmRMaW5lPSFub21wYXRoCkNvbmRpdGlvblZpcnR1YWxpemF0aW9uPSFjb250YWluZXIKCltTZXJ2aWNlXQpUeXBlPW5vdGlmeQpOb3RpZnlBY2Nlc3M9bWFpbgpFeGVjU3RhcnQ9L3NiaW4vbXVsdGlwYXRoZCAtZCAtcwpFeGVjUmVsb2FkPS9zYmluL211bHRpcGF0aGQgcmVjb25maWd1cmUKVGFza3NNYXg9aW5maW5pdHkKCltJbnN0YWxsXQpXYW50ZWRCeT1zeXNpbml0LnRhcmdldA==
         encoding: base64
         ownerstring: ""
EOF
  fi

  # SLE Micro 5.5 uses /usr/lib/ssh/sftp-server
  # SL Micro 6.1 uses /usr/libexec/ssh/sftp-server
  if [ -e ${HOST_DIR}/etc/ssh/sshd_config.d/sftp.conf ]; then
    sed -i 's%/usr/lib/ssh/sftp-server%/usr/libexec/ssh/sftp-server%' ${HOST_DIR}/etc/ssh/sshd_config.d/sftp.conf
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
  set_rke2_device_permissions
  set_oem_cleanup_kubelet
  # A post-drain signal from Rancher doesn't mean RKE2 agent/server is already patched and restarted
  # Let's wait until the RKE2 settled.
  wait_rke2_upgrade
  clean_rke2_archives

  kubectl taint node $HARVESTER_UPGRADE_NODE_NAME kubevirt.io/drain- || true

  generate_networkmanager_config

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

  # Shut down non-live migratable VMs
  upgrade-helper vm-live-migrate-detector "$HARVESTER_UPGRADE_NODE_NAME" --shutdown --upgrade "$HARVESTER_UPGRADE_NAME"
  wait_vms_out

  echo "wait for fleet bundles before upgrading RKE2"
  # wait all fleet bundles in limited time
  wait_for_fleet_bundles

  # update max-pods to 200
  set_max_pods
  set_reserved_resource
  set_rke2_device_permissions
  set_oem_cleanup_kubelet
  # Upgarde RKE2
  upgrade_rke2

  wait_rke2_upgrade
  clean_rke2_archives

  generate_networkmanager_config

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
