#!/bin/bash -ex

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
ELEMENTAL_DIR="elemental_cli"

source $SCRIPT_DIR/lib.sh
UPGRADE_TMP_DIR=$HOST_DIR/usr/local/upgrade_tmp

clean_up_tmp_files()
{
  if [ -n "$tmp_rootfs_mount" ]; then
    echo "Try to unmount $tmp_rootfs_mount..."
    umount $tmp_rootfs_mount || echo "Umount $tmp_rootfs_mount failed with return code: $?"
  fi
  if [ -n "$target_elemental_cli" ]; then
    echo "Try to unmount $target_elemental_cli..."
    umount $target_elemental_cli || echo "Umount $target_elemental_cli failed with return code: $?"
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
  # Skip only when the VMs have other places to go
  local node_labels=""
  local node_count=0
  kubectl get vmi -A -l kubevirt.io/nodeName=$HARVESTER_UPGRADE_NODE_NAME -o json |
    jq -r '.items[] | select(.spec.affinity.nodeAffinity != null) | [.metadata.name, .metadata.namespace] | @tsv' |
    while IFS=$'\t' read -r name namespace; do
      if [ -z "$name" ]; then
        break
      fi
      node_labels=$(kubectl -n "$namespace" get vmi "$name" -o yaml |
        yq '.spec.affinity.nodeAffinity.*.nodeSelectorTerms[].matchExpressions[] | select(.operator=="In" and .values[] == "true") | .key')
      for nl in $node_labels; do
        # For nodes considered "candidates" for the VM to live-migrate to, they should
        # 1. match the same labels as nodeAffinity described
        # 2. not be the node that the VM currently runs on
        # 3. be schedulable at the time
        node_count=$(kubectl get nodes -o yaml |
                nl="$nl" n="$HARVESTER_UPGRADE_NODE_NAME" yq '.items[] | select(.metadata.labels.[env(nl)] == "true" and .metadata.name != env(n) and .spec.unschedulable == null) | .metadata.name' | wc -l)

        if [ "$node_count" -gt 0 ]; then
          # If such nodes exist, continue to check for the next label
          echo "$namespace/$name still has $node_count place(s) to go for label $nl"
          continue
        else
          # If there is no candidates for any of the labels, just break the loop and shut the VM down
          echo "$namespace/$name has no other places to go for label $nl"
          break
        fi
      done

      if [ $node_count -gt 0 ]; then
        echo "$namespace/$name is considered live-migratable"
      else
        echo "$namespace/$name is non-migratable, shutdown immediately"
        virtctl stop "$name" -n "$namespace"
      fi
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
  wait_longhorn_engines

  shutdown_non_migrate_able_vms

  # Live migrate VMs
  kubectl taint node $HARVESTER_UPGRADE_NODE_NAME --overwrite kubevirt.io/drain=draining:NoSchedule

  # Wait for VM migrated
  wait_vms_out_or_shutdown

  # KubeVirt's pdb might cause drain fail
  wait_evacuation_pdb_gone

  # Add logging related kube-audit policy file
  patch_logging_event_audit

  remove_rke2_canal_config
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

convert_nodenetwork_to_vlanconfig() {
  detect_upgrade
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

  # replace the fixed elemental CLI for fix elemental upgrade issues
  new_elemental_cli=$SCRIPT_DIR/$ELEMENTAL_DIR/elemental
  target_elemental_cli=$HOST_DIR/usr/bin/elemental
  elemental_upgrade_log="${UPGRADE_TMP_DIR#"$HOST_DIR"}/elemental-upgrade-$(date +%Y%m%d%H%M%S).log"
  local ret=0
  mount --bind $new_elemental_cli $target_elemental_cli
  chroot $HOST_DIR elemental upgrade --logfile "$elemental_upgrade_log" --directory ${tmp_rootfs_mount#"$HOST_DIR"} || ret=$?
  if [ "$ret" != 0 ]; then
    echo "elemental upgrade failed with return code: $ret"
    cat "$HOST_DIR$elemental_upgrade_log"
    exit "$ret"
  fi

  # For some firmware (e.g., Dell BOSS adapter 3022), users may get
  # stuck on the grub file search for about 30 minutes, this can be
  # mitigated by adding the `grubenv` file.
  #
  # PATCH: add /oem/grubenv if it does not exist on upgrade_path
  GRUBENV_FILE="/oem/grubenv"
  chroot $HOST_DIR /bin/bash -c "if ! [ -f ${GRUBENV_FILE} ]; then grub2-editenv ${GRUBENV_FILE} create; fi"

  umount $target_elemental_cli
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

  wait_repo
  detect_repo

  remove_rke2_canal_config

  # Copy OS things, we need to shutdown repo VMs.
  NEW_OS_SQUASHFS_IMAGE_FILE=$(mktemp -p $UPGRADE_TMP_DIR)
  download_file "$UPGRADE_REPO_SQUASHFS_IMAGE" "$NEW_OS_SQUASHFS_IMAGE_FILE"

  # Stop all VMs
  shutdown_all_vms
  wait_vms_out

  # Add logging related kube-audit policy file
  patch_logging_event_audit

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
