#!/bin/bash -ex

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

source $SCRIPT_DIR/lib.sh
UPGRADE_TMP_DIR=$HOST_DIR/usr/local/upgrade_tmp

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

get_running_vm_count()
{
  local count

  count=$(kubectl get vmi -A -l kubevirt.io/nodeName=$HARVESTER_UPGRADE_NODE_NAME -ojson | jq '.items | length' || true)
  echo $count
}

wait_vms_out()
{
  local vm_count="$(get_running_vm_count)"

  until [ "$vm_count" = "0" ]
  do
    echo "Waiting for VM live-migration or shutdown...($vm_count left)"
    sleep 5
    vm_count="$(get_running_vm_count)"
  done
}

wait_vms_out_or_shutdown()
{
  local vm_count
  local max_retries=240

  retries=0
  while [ true ]; do
    vm_count="$(get_running_vm_count)"

    if [ "$vm_count" = "0" ]; then
      break
    fi

    if [ "$retries" = "$max_retries" ]; then
      echo "WARNING: fail to live-migrate $vm_count VM(s). Shutting down them..."
      shutdown_vms_on_node
    fi

    echo "Waiting for VM live-migration or shutdown...($vm_count left)"
    sleep 5
    retries=$((retries+1))
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

shutdown_non_migrate_able_vms()
{
  # VMs with nodeSelector
  kubectl get vmi -A -l kubevirt.io/nodeName=$HARVESTER_UPGRADE_NODE_NAME -o json |
    jq -r '.items[] | select(.spec.nodeSelector != null) | [.metadata.name, .metadata.namespace] | @tsv' |
    while IFS=$'\t' read -r name namespace; do
      if [ -z "$name" ]; then
        break
      fi
      echo "Stop ${namespace}/${name}"
      virtctl stop $name -n $namespace
    done

  # VMs with nodeAffinity
  kubectl get vmi -A -l kubevirt.io/nodeName=$HARVESTER_UPGRADE_NODE_NAME -o json |
    jq -r '.items[] | select(.spec.affinity.nodeAffinity != null) | [.metadata.name, .metadata.namespace] | @tsv' |
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


command_pre_drain() {
  shutdown_non_migrate_able_vms

  # Live migrate VMs
  kubectl taint node $HARVESTER_UPGRADE_NODE_NAME --overwrite kubevirt.io/drain=draining:NoSchedule

  # Wait for VM migrated
  wait_vms_out_or_shutdown

  # KubeVirt's pdb might cause drain fail
  wait_evacuation_pdb_gone
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

upgrade_os() {
  CURRENT_OS_VERSION=$(source $HOST_DIR/etc/os-release && echo $PRETTY_NAME)

  if [ "$REPO_OS_PRETTY_NAME" = "$CURRENT_OS_VERSION" ]; then
    echo "Skip upgrading OS. The OS version is already \"$CURRENT_OS_VERSION\"."
    return
  fi
  
  # upgrade OS image and reboot
  mount --rbind $HOST_DIR/dev /dev
  mount --rbind $HOST_DIR/run /run

  if [ -n "$NEW_OS_SQUASHFS_IMAGE_FILE" ]; then
    tmp_rootfs_squashfs="$NEW_OS_SQUASHFS_IMAGE_FILE"
  else
    tmp_rootfs_squashfs=$(mktemp -p $UPGRADE_TMP_DIR)
    curl -fL $UPGRADE_REPO_SQUASHFS_IMAGE -o $tmp_rootfs_squashfs
  fi

  tmp_rootfs_mount=$(mktemp -d -p $HOST_DIR/tmp)
  mount $tmp_rootfs_squashfs $tmp_rootfs_mount

  chroot $HOST_DIR elemental upgrade --directory ${tmp_rootfs_mount#"$HOST_DIR"}
  umount $tmp_rootfs_mount
  rm -rf $tmp_rootfs_squashfs

  umount -R /run

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

  # A post-drain signal from Rancher doesn't mean RKE2 agent/server is already patched and restarted
  # Let's wait until the RKE2 settled.
  wait_rke2_upgrade
  clean_rke2_archives

  kubectl taint node $HARVESTER_UPGRADE_NODE_NAME kubevirt.io/drain- || true
  upgrade_os
}

command_single_node_upgrade() {
  echo "Upgrade single node"

  wait_repo
  detect_repo

  # Copy OS things, we need to shutdown repo VMs.
  NEW_OS_SQUASHFS_IMAGE_FILE=$(mktemp -p $UPGRADE_TMP_DIR)
  curl -fL $UPGRADE_REPO_SQUASHFS_IMAGE -o $NEW_OS_SQUASHFS_IMAGE_FILE

  # Stop all VMs
  shutdown_all_vms
  wait_vms_out

  # Upgarde RKE2
  upgrade_rke2
  wait_rke2_upgrade
  clean_rke2_archives

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
