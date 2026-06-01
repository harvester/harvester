package upgrade

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/name"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade/repoinfo"
	"github.com/harvester/harvester/pkg/util"
)

const (
	nodeComponent                = "node"
	manifestComponent            = "manifest"
	cleanupComponent             = "cleanup"
	skipManifestsApplyComponent  = "apply-skip-rke2-manifests"
	skipManifestsRemoveComponent = "remove-skip-rke2-manifests"

	labelArch               = "kubernetes.io/arch"
	labelCriticalAddonsOnly = "CriticalAddonsOnly"

	// upgradeHeartbeatInterval defines a coarse re-enqueue interval for the upgrade controller.
	upgradeHeartbeatInterval = time.Minute * 2
	// upgradeCommonRequeueInterval defines a fast re-enqueue interval for plan/node handlers.
	upgradeCommonRequeueInterval = time.Second * 20
	// keep jobs for 7 days
	defaultTTLSecondsAfterFinished = 604800
	// Give up to an hour for slower hardware to preload images.
	defaultPrepareDeadlineSeconds = 3600
	skipManifestScript            = `
#!/usr/bin/env sh
set -e

HOST_DIR="${HOST_DIR:-/host}"
MANIFESTS_DIR="$HOST_DIR/var/lib/rancher/rke2/server/manifests"

if [ -z "$MANIFESTS" ]; then
  echo "No manifests specified, nothing to do"
  exit 0
fi

if [ ! -d "$MANIFESTS_DIR" ]; then
  echo "Manifests directory $MANIFESTS_DIR does not exist, skipping"
  exit 0
fi

for manifest in $MANIFESTS; do
  skip_file="$MANIFESTS_DIR/${manifest}.skip"
  case "$SKIP_ACTION" in
    apply)
      echo "Creating skip file: $skip_file"
      touch "$skip_file"
      ;;
    remove)
      echo "Removing skip file: $skip_file"
      rm -vf "$skip_file"
      ;;
    *)
      echo "Unknown SKIP_ACTION: $SKIP_ACTION (must be 'apply' or 'remove')"
      exit 1
      ;;
  esac
done
`

	// imageCleanupScript houses a best-effort shell automation workflow executed on
	// cluster hosts during system upgrades. It addresses critical disk reclamation
	// requirements by targeting specific version diffs and host-level dangling assets.
	//
	// CONCERNS & EDGE CASES HANDLED:
	// 1. Decoupled Registry Overrides: Upstream controllers track canonical image targets
	//    (e.g., "rancher/harvester-abc:v0.5.0"), but nodes might pull from private, air-gapped,
	//    or customized registries (e.g., "example.local/rancher/harvester-abc:v0.5.0").
	//    A strict string match would miss these, leaving stale data on disk.
	// 2. Orphaned "Ghost" Images: Leftover, untagged, or broken layers (<none>:<none>)
	//    frequently accumulate during layered cluster shifts and require active pruning.
	// 3. Controller Fault Tolerance: Variable injection from the controller can
	//    occasionally yield empty lists or whitespace-only arrays. The script must fail
	//    safely without crashing or creating evaluation syntax errors.
	// 4. "Try-Best" Over "Fail-Fast": If one image is locked by a terminating or stubborn
	//    pod, standard batch calls fail entirely. The workflow must gracefully skip locked
	//    assets and continue reclaiming space from the remaining targets.
	// 5. Environment Resiliency: On non-standard hosts where the runtime binary paths
	//    deviate or are unavailable, the automation logs a clear troubleshooting manual
	//    action notice instead of breaking the broader execution sequence.

	imageCleanupScript = `
#!/usr/bin/env bash
set -e

# Read from environment parameter if present; default to "false" if unset or empty for silent execution.
DEBUG_MODE="${DEBUG_MODE:-false}"

# For Host Testing: Leave HOST_DIR empty.
# For Container Execution: Set HOST_DIR to your host root mount (e.g., /host).
HOST_DIR="${HOST_DIR:-/host}"

# Target configurations to pass into the cleanup function
# Following example is for local test
# IMAGES="any/vmdp:1.0.0 any/alpine:latest temporary-worker:latest hello-world:latest"

REAL_RKE2_CONFIG="$HOST_DIR/var/lib/rancher/rke2/agent/etc/crictl.yaml"

export CONTAINER_RUNTIME_ENDPOINT=unix:///$HOST_DIR/run/k3s/containerd/containerd.sock
export CONTAINERD_ADDRESS=$HOST_DIR/run/k3s/containerd/containerd.sock

if [ -f "$REAL_RKE2_CONFIG" ]; then
  export CRI_CONFIG_FILE="$REAL_RKE2_CONFIG"
else
  export CRI_CONFIG_FILE="/dev/null"
  export CONTAINER_RUNTIME_ENDPOINT=unix:///$HOST_DIR/run/k3s/containerd/containerd.sock
  export CONTAINERD_ADDRESS=$HOST_DIR/run/k3s/containerd/containerd.sock
fi

cleanup_images() {
  local to_purge_images_list="$1"
  echo "Prepare to purge images $to_purge_images_list"

  # Skip VMDP data images; they may be needed by stopped or suspended
  # VMs, and removing them risks breaking future VM restoration.
  # 1. Pre-process the input target list
  local short_targets=()
  local img
  for img in $to_purge_images_list; do
    local short_name="${img##*/}"
    if [[ "$short_name" == vmdp:* ]]; then
      echo "[INFO] Skipping system data image target: $img"
      continue
    fi
    short_targets+=("$short_name")
  done

  # Safely convert Bash array targets into a native JSON array
  local targets_json
  targets_json=$(printf '%s\n' "${short_targets[@]}" | jq -R . | jq -s -c .)

  # Target isolation: Branched dynamically by the DEBUG_MODE toggle
  local targets_to_kill

  if [[ "$DEBUG_MODE" == "true" ]]; then
    echo "[DEBUG] DEBUG_MODE=true. Streaming live JQ evaluations to stderr..."
    targets_to_kill=$("$CRICTL" images -o json 2>/dev/null | jq -r --argjson targets "${targets_json:-[]}" '
      .images[]? | select(.pinned != true) | . as $img |
      if ($img.repoTags == null or ($img.repoTags | length == 0) or $img.repoTags[0] == "<none>:<none>") then
        ("[DEBUG] Dangling: " + $img.id) | debug | $img.id
      else
        $img.repoTags[] | . as $tag | ("[DEBUG] Tag: " + $tag) | debug |
        if ($targets | any(. == $tag or . == ($tag | split("/")[-1]))) then
          ("[DEBUG] -> MATCHED: " + $tag) | debug | $tag
        else
          empty
        end
      end
    ' || true)
  else
    # Standard Production Run: Silences internal logs and discards execution noise
    targets_to_kill=$("$CRICTL" images -o json 2>/dev/null | jq -r --argjson targets "${targets_json:-[]}" '
      .images[]? | select(.pinned != true) | . as $img |
      if ($img.repoTags == null or ($img.repoTags | length == 0) or $img.repoTags[0] == "<none>:<none>") then
        $img.id
      else
        $img.repoTags[] | . as $tag |
        if ($targets | any(. == $tag or . == ($tag | split("/")[-1]))) then
          $tag
        else
          empty
        end
      end
    ' 2>/dev/null || true)
  fi

  # Convert newline-separated targets into a Bash array
  local -a kill_array=()
  if [[ -n "$targets_to_kill" ]]; then
    mapfile -t kill_array <<< "$targets_to_kill"
  fi

  if [[ "$DEBUG_MODE" == "true" ]]; then
    echo "[DEBUG] Content of kill_array:"
    declare -p kill_array
  fi

  # Purge sequentially to maximize successful deletions. Bulk execution
  #    via crictl aborts entirely if a single image is locked (e.g., actively
  #    referenced by a newly spawned dynamic workload). Deleting one-by-one
  #    adds minimal overhead; even with 100+ images, modern CPU execution
  #    times remain completely insignificant.
  if [[ ${#kill_array[@]} -gt 0 ]]; then
    echo "[ACTION] Found ${#kill_array[@]} target items to clear. Starting ..."

    local success_count=0
    local fail_count=0
    local target
    local exit_code=0

    for target in "${kill_array[@]}"; do
      exit_code=0
      echo "Purge image $target"
      "$CRICTL" rmi "$target" || exit_code=$?
      if [[ $exit_code -eq 0 ]]; then
        echo "  [SUCCESS]"
        success_count=$((success_count + 1))
      else
        fail_count=$((fail_count + 1))
        echo "  [WARN] Failed."
      fi
    done

    # Final Execution Summary
    echo "[INFO] Purge sequence complete."
    echo "       Successfully removed: $success_count"
    if [[ $fail_count -gt 0 ]]; then
      echo "       Skipped/Failed      : $fail_count"
    fi
  else
    echo "[INFO] Nothing to do, no matching or dangling images found."
  fi
}

# Resolve and assign the binary path dynamically
CRICTL="$HOST_DIR/$(readlink $HOST_DIR/var/lib/rancher/rke2/bin)/crictl"
if [ ! -x "$CRICTL" ]; then
  echo "[ERROR] Host crictl binary not found/executable at: $CRICTL"
  exit 0
fi

# Fire execution sequence using the configuration target list
cleanup_images "$IMAGES"
`
)

func getNodeUpgradePauseMap(upgrade *harvesterv1.Upgrade) (map[string]string, error) {
	if upgrade.Annotations == nil {
		return nil, nil
	}

	value, ok := upgrade.Annotations[util.AnnotationNodeUpgradePauseMap]
	if !ok {
		return nil, nil
	}

	var pauseMap map[string]string
	if err := json.Unmarshal([]byte(value), &pauseMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal node upgrade pause map: %w", err)
	}

	return pauseMap, nil
}

func shouldPauseNodeUpgrade(upgrade *harvesterv1.Upgrade, nodeName string) bool {
	pauseMap, err := getNodeUpgradePauseMap(upgrade)
	if err != nil || pauseMap == nil {
		return false
	}

	return pauseMap[nodeName] == util.NodePause
}

func setNodeUpgradeStatus(upgrade *harvesterv1.Upgrade, nodeName string, state, reason, message string) {
	if upgrade == nil {
		return
	}
	if upgrade.Status.NodeStatuses == nil {
		upgrade.Status.NodeStatuses = make(map[string]harvesterv1.NodeUpgradeStatus)
	}
	if current, ok := upgrade.Status.NodeStatuses[nodeName]; ok &&
		current.State == state && current.Reason == reason && current.Message == message {
		return
	}
	upgrade.Status.NodeStatuses[nodeName] = harvesterv1.NodeUpgradeStatus{
		State:   state,
		Reason:  reason,
		Message: message,
	}
	if state == StateFailed {
		setNodesUpgradedCondition(upgrade, corev1.ConditionFalse, reason, message)
		return
	}

	if upgrade.Labels[upgradeStateLabel] == StateUpgradingNodes {
		for _, nodeStatus := range upgrade.Status.NodeStatuses {
			if nodeStatus.State != StateSucceeded {
				return
			}
		}
		setNodesUpgradedCondition(upgrade, corev1.ConditionTrue, "", "")
	}
}

func setLogReadyCondition(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.LogReady.SetStatus(upgrade, string(status))
	harvesterv1.LogReady.Reason(upgrade, reason)
	harvesterv1.LogReady.Message(upgrade, message)
	markComplete(upgrade)
}

func setImageReadyCondition(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.ImageReady.SetStatus(upgrade, string(status))
	harvesterv1.ImageReady.Reason(upgrade, reason)
	harvesterv1.ImageReady.Message(upgrade, message)
	markComplete(upgrade)
}

func setRepoProvisionedCondition(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.RepoProvisioned.SetStatus(upgrade, string(status))
	harvesterv1.RepoProvisioned.Reason(upgrade, reason)
	harvesterv1.RepoProvisioned.Message(upgrade, message)
	markComplete(upgrade)
}

func setNodesPreparedCondition(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.NodesPrepared.SetStatus(upgrade, string(status))
	harvesterv1.NodesPrepared.Reason(upgrade, reason)
	harvesterv1.NodesPrepared.Message(upgrade, message)
	markComplete(upgrade)
}

func setNodesUpgradedCondition(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.NodesUpgraded.SetStatus(upgrade, string(status))
	harvesterv1.NodesUpgraded.Reason(upgrade, reason)
	harvesterv1.NodesUpgraded.Message(upgrade, message)
	markComplete(upgrade)
}

func setUpgradeCompletedCondition(upgrade *harvesterv1.Upgrade, state string, status corev1.ConditionStatus, reason, message string) {
	upgrade.Labels[upgradeStateLabel] = state
	harvesterv1.UpgradeCompleted.SetStatus(upgrade, string(status))
	harvesterv1.UpgradeCompleted.Reason(upgrade, reason)
	harvesterv1.UpgradeCompleted.Message(upgrade, message)
}

func setHelmChartUpgradeStatus(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	if upgrade == nil ||
		harvesterv1.SystemServicesUpgraded.IsTrue(upgrade) ||
		harvesterv1.SystemServicesUpgraded.IsFalse(upgrade) {
		return
	}
	harvesterv1.SystemServicesUpgraded.SetStatus(upgrade, string(status))
	harvesterv1.SystemServicesUpgraded.Reason(upgrade, reason)
	harvesterv1.SystemServicesUpgraded.Message(upgrade, message)
	markComplete(upgrade)
}

func markComplete(upgrade *harvesterv1.Upgrade) {
	if upgrade.Labels == nil {
		upgrade.Labels = make(map[string]string)
	}
	if harvesterv1.SystemServicesUpgraded.IsTrue(upgrade) &&
		harvesterv1.NodesUpgraded.IsTrue(upgrade) {
		harvesterv1.UpgradeCompleted.True(upgrade)
		upgrade.Labels[upgradeStateLabel] = StateSucceeded
		upgrade.Labels[upgradeCleanupLabel] = StatePending
	}
	if harvesterv1.ImageReady.IsFalse(upgrade) || harvesterv1.RepoProvisioned.IsFalse(upgrade) ||
		harvesterv1.SystemServicesUpgraded.IsFalse(upgrade) || harvesterv1.NodesUpgraded.IsFalse(upgrade) {
		harvesterv1.UpgradeCompleted.False(upgrade)
		upgrade.Labels[upgradeStateLabel] = StateFailed
		upgrade.Labels[upgradeCleanupLabel] = StatePending
	}
}

func prepareUpgradeLog(upgrade *harvesterv1.Upgrade) *harvesterv1.UpgradeLog {
	return &harvesterv1.UpgradeLog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName(upgrade.Name, "upgradelog"),
			Namespace: upgradeNamespace,
			Labels: map[string]string{
				harvesterUpgradeLabel: upgrade.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(upgrade),
			},
		},
		Spec: harvesterv1.UpgradeLogSpec{
			UpgradeName: upgrade.Name,
		},
	}
}

func prepareSkipManifestPlan(upgrade *harvesterv1.Upgrade, manifests []string, skip bool) *upgradev1.Plan {
	imageVersion := upgrade.Status.PreviousVersion

	action, componentLabel := "remove", skipManifestsRemoveComponent
	if skip {
		action, componentLabel = "apply", skipManifestsApplyComponent
	}

	return &upgradev1.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-skip-manifests-%s", upgrade.Name, action),
			Namespace: sucNamespace,
			Labels: map[string]string{
				harvesterUpgradeLabel:          upgrade.Name,
				harvesterUpgradeComponentLabel: componentLabel,
			},
		},
		Spec: upgradev1.PlanSpec{
			Concurrency: int64(len(upgrade.Status.NodeStatuses)),
			Version:     upgrade.Name,
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					harvesterManagedLabel: "true",
				},
			},
			ServiceAccountName: upgradeServiceAccount,
			Tolerations: []corev1.Toleration{
				{
					Key:      labelCriticalAddonsOnly,
					Operator: corev1.TolerationOpExists,
				},
				{
					Key:      "kubevirt.io/drain",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
				{
					Key:      util.KubeControlPlaneNodeLabelKey,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoExecute,
				},
				{
					Key:      util.KubeEtcdNodeLabelKey,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoExecute,
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "amd64",
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "arm64",
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "arm",
				},
				{
					Key:      corev1.TaintNodeNotReady,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
				{
					Key:      corev1.TaintNodeNetworkUnavailable,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			Upgrade: &upgradev1.ContainerSpec{
				Image: upgrade.GetUpgradeImage(util.HarvesterUpgradeImageRepository, imageVersion),
				Command: []string{
					"sh", "-c", skipManifestScript,
				},
				Env: []corev1.EnvVar{
					{
						Name:  "MANIFESTS",
						Value: strings.Join(manifests, " "),
					},
					{
						Name:  "SKIP_ACTION",
						Value: action,
					},
				},
			},
		},
	}
}

func prepareCleanupPlan(upgrade *harvesterv1.Upgrade, imageList []string) *upgradev1.Plan {
	concurrency := len(upgrade.Status.NodeStatuses)
	planVersion := upgrade.Name
	imageVersion := upgrade.Status.PreviousVersion

	return &upgradev1.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-cleanup", upgrade.Name),
			Namespace: sucNamespace,
			Labels: map[string]string{
				harvesterUpgradeLabel:          upgrade.Name,
				harvesterUpgradeComponentLabel: cleanupComponent,
			},
		},
		Spec: upgradev1.PlanSpec{
			Concurrency: int64(concurrency),
			Version:     planVersion,
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					harvesterManagedLabel: "true",
				},
			},
			ServiceAccountName: upgradeServiceAccount,
			Tolerations: []corev1.Toleration{
				{
					Key:      labelCriticalAddonsOnly,
					Operator: corev1.TolerationOpExists,
				},
				{
					Key:      "kubevirt.io/drain",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
				{
					Key:      util.KubeControlPlaneNodeLabelKey,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoExecute,
				},
				{
					Key:      util.KubeEtcdNodeLabelKey,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoExecute,
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "amd64",
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "arm64",
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "arm",
				},
			},
			Upgrade: &upgradev1.ContainerSpec{
				Image: upgrade.GetUpgradeImage(util.HarvesterUpgradeImageRepository, imageVersion),
				Command: []string{
					"bash", "-c", imageCleanupScript,
				},
				Env: []corev1.EnvVar{
					{
						Name:  "IMAGES",
						Value: strings.Join(imageList, " "),
					},
				},
			},
		},
	}
}

func preparePlan(upgrade *harvesterv1.Upgrade, concurrency int) *upgradev1.Plan {
	planVersion := upgrade.Name

	// Use current running version because new images are not preloaded yet.
	imageVersion := upgrade.Status.PreviousVersion
	return &upgradev1.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-prepare", upgrade.Name),
			Namespace: sucNamespace,
			Labels: map[string]string{
				harvesterUpgradeLabel:          upgrade.Name,
				harvesterUpgradeComponentLabel: nodeComponent,
			},
		},
		Spec: upgradev1.PlanSpec{
			Concurrency:           int64(concurrency),
			JobActiveDeadlineSecs: defaultPrepareDeadlineSeconds,
			Version:               planVersion,
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					harvesterManagedLabel: "true",
				},
			},
			ServiceAccountName: upgradeServiceAccount,
			Tolerations: []corev1.Toleration{
				{
					Key:      labelCriticalAddonsOnly,
					Operator: corev1.TolerationOpExists,
				},
				{
					Key:      "kubevirt.io/drain",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
				{
					Key:      util.KubeControlPlaneNodeLabelKey,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoExecute,
				},
				{
					Key:      util.KubeEtcdNodeLabelKey,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoExecute,
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "amd64",
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "arm64",
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "arm",
				},
			},
			Upgrade: &upgradev1.ContainerSpec{
				Image:   upgrade.GetUpgradeImage(util.HarvesterUpgradeImageRepository, imageVersion),
				Command: []string{"do_upgrade_node.sh"},
				Args:    []string{"prepare"},
				Env: []corev1.EnvVar{
					{
						Name:  "HARVESTER_UPGRADE_NAME",
						Value: upgrade.Name,
					},
				},
			},
		},
	}
}

func applyNodeJob(upgrade *harvesterv1.Upgrade, repoInfo *repoinfo.RepoInfo, nodeName string, jobType string) *batchv1.Job {
	// Use the image tag in the upgrade repo because it's already preloaded and might contain updated codes.
	imageVersion := repoInfo.Release.Harvester
	hostPathDirectory := corev1.HostPathDirectory
	privileged := true
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName(upgrade.Name, jobType, nodeName),
			Namespace: upgrade.Namespace,
			Labels: map[string]string{
				harvesterUpgradeLabel:          upgrade.Name,
				harvesterUpgradeComponentLabel: nodeComponent,
				harvesterNodeLabel:             nodeName,
				upgradeJobTypeLabel:            jobType,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(upgrade),
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptr.To(int32(defaultTTLSecondsAfterFinished)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						harvesterUpgradeLabel:          upgrade.Name,
						harvesterUpgradeComponentLabel: nodeComponent,
						upgradeJobTypeLabel:            jobType,
					},
				},
				Spec: corev1.PodSpec{
					HostIPC:       true,
					HostPID:       true,
					HostNetwork:   true,
					DNSPolicy:     corev1.DNSClusterFirstWithHostNet,
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "apply",
							Image:   upgrade.GetUpgradeImage(util.HarvesterUpgradeImageRepository, imageVersion),
							Command: []string{"do_upgrade_node.sh"},
							Args:    []string{jobType},
							Env: []corev1.EnvVar{
								{
									Name:  "HARVESTER_UPGRADE_NAME",
									Value: upgrade.Name,
								},
								{
									Name:  "HARVESTER_UPGRADE_NODE_NAME",
									Value: nodeName,
								},
								{
									Name: "HARVESTER_UPGRADE_POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "host-root", MountPath: "/host"},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										corev1.Capability("CAP_SYS_BOOT"),
									},
								},
							},
						},
					},
					ServiceAccountName: "harvester",
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											nodeName,
										},
									}},
								}},
							},
						},
					},
					Tolerations: getDefaultTolerations(),
					Volumes: []corev1.Volume{
						{
							Name: `host-root`,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/", Type: &hostPathDirectory,
								},
							},
						},
					},
				},
			},
		},
	}
}

func applyRestoreVMJob(upgrade *harvesterv1.Upgrade, repoInfo *repoinfo.RepoInfo, nodeName string) *batchv1.Job {
	// Use the image tag in the upgrade repo because it's already preloaded and might contain updated codes.
	imageVersion := repoInfo.Release.Harvester
	jobType := upgradeJobTypeRestoreVM
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName(upgrade.Name, jobType, nodeName),
			Namespace: upgrade.Namespace,
			Labels: map[string]string{
				harvesterUpgradeLabel: upgrade.Name,
				harvesterNodeLabel:    nodeName,
				upgradeJobTypeLabel:   jobType,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(upgrade),
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptr.To(int32(defaultTTLSecondsAfterFinished)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						harvesterUpgradeLabel: upgrade.Name,
						upgradeJobTypeLabel:   jobType,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "apply",
							Image:   upgrade.GetUpgradeImage(util.HarvesterUpgradeImageRepository, imageVersion),
							Command: []string{"upgrade-helper"},
							Args: []string{
								"restore-vm",
								"--node", nodeName,
								"--upgrade", upgrade.Name,
							},
						},
					},
					ServiceAccountName: "harvester",
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											nodeName,
										},
									}},
								}},
							},
						},
					},
					Tolerations: getDefaultTolerations(),
				},
			},
		},
	}
}

func applyManifestsJob(upgrade *harvesterv1.Upgrade, repoInfo *repoinfo.RepoInfo) *batchv1.Job {
	// Use the image tag in the upgrade repo because it's already preloaded and might contain updated codes.
	imageVersion := repoInfo.Release.Harvester
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName(upgrade.Name, "apply-manifests"),
			Namespace: upgrade.Namespace,
			Labels: map[string]string{
				harvesterUpgradeLabel:          upgrade.Name,
				harvesterUpgradeComponentLabel: manifestComponent,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(upgrade),
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptr.To(int32(defaultTTLSecondsAfterFinished)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						harvesterUpgradeLabel:          upgrade.Name,
						harvesterUpgradeComponentLabel: manifestComponent,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "apply",
							Image:   upgrade.GetUpgradeImage(util.HarvesterUpgradeImageRepository, imageVersion),
							Command: []string{"upgrade_manifests.sh"},
							Env: []corev1.EnvVar{
								{
									Name:  "HARVESTER_UPGRADE_NAME",
									Value: upgrade.Name,
								},
							},
						},
					},
					ServiceAccountName: "harvester",
					Tolerations:        getDefaultTolerations(),
				},
			},
		},
	}
}

func getDefaultTolerations() []corev1.Toleration {
	return []corev1.Toleration{
		{
			Key:      corev1.TaintNodeUnschedulable,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      util.KubeControlPlaneNodeLabelKey,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoExecute,
		},
		{
			Key:      util.KubeEtcdNodeLabelKey,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoExecute,
		},
		{
			Key:      "kubevirt.io/drain",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      labelCriticalAddonsOnly,
			Operator: corev1.TolerationOpExists,
		},
		{
			Key:      corev1.TaintNodeUnreachable,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoExecute,
		},
		{
			Key:      labelArch,
			Operator: corev1.TolerationOpEqual,
			Effect:   corev1.TaintEffectNoSchedule,
			Value:    "amd64",
		},
		{
			Key:      labelArch,
			Operator: corev1.TolerationOpEqual,
			Effect:   corev1.TaintEffectNoSchedule,
			Value:    "arm64",
		},
		{
			Key:      labelArch,
			Operator: corev1.TolerationOpEqual,
			Effect:   corev1.TaintEffectNoSchedule,
			Value:    "arm",
		},
	}
}

type jobBuilder struct {
	job *batchv1.Job
}

func newJobBuilder(name string) *jobBuilder {
	return &jobBuilder{
		job: &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeNamespace,
			},
		},
	}
}

func (j *jobBuilder) WithLabel(key, value string) *jobBuilder {
	if j.job.Labels == nil {
		j.job.Labels = make(map[string]string)
	}
	j.job.Labels[key] = value
	return j
}

func (j *jobBuilder) Running() *jobBuilder {
	j.job.Status.Active = 1
	return j
}

func (j *jobBuilder) Completed() *jobBuilder {
	j.job.Status.Succeeded = 1
	j.job.Status.Conditions = append(j.job.Status.Conditions, batchv1.JobCondition{
		Type:   batchv1.JobComplete,
		Status: "True",
	})
	return j
}

func (j *jobBuilder) Failed(reason, message string) *jobBuilder {
	j.job.Status.Failed = 1
	j.job.Status.Conditions = append(j.job.Status.Conditions, batchv1.JobCondition{
		Type:    batchv1.JobFailed,
		Status:  "True",
		Reason:  reason,
		Message: message,
	})
	return j
}

func (j *jobBuilder) Build() *batchv1.Job {
	return j.job
}

type upgradeBuilder struct {
	upgrade *harvesterv1.Upgrade
}

func newUpgradeBuilder(name string) *upgradeBuilder {
	return &upgradeBuilder{
		upgrade: &harvesterv1.Upgrade{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: harvesterSystemNamespace,
			},
		},
	}
}

func (p *upgradeBuilder) WithLabel(key, value string) *upgradeBuilder {
	if p.upgrade.Labels == nil {
		p.upgrade.Labels = make(map[string]string)
	}
	p.upgrade.Labels[key] = value
	return p
}

func (p *upgradeBuilder) WithAnnotation(key, value string) *upgradeBuilder {
	if p.upgrade.Annotations == nil {
		p.upgrade.Annotations = make(map[string]string)
	}
	p.upgrade.Annotations[key] = value
	return p
}

func (p *upgradeBuilder) WithLogEnabled(value bool) *upgradeBuilder {
	p.upgrade.Spec.LogEnabled = value
	return p
}

func (p *upgradeBuilder) WithImage(image string) *upgradeBuilder {
	p.upgrade.Spec.Image = fmt.Sprintf("%s/%s", upgradeNamespace, image)
	return p
}

func (p *upgradeBuilder) Version(version string) *upgradeBuilder {
	p.upgrade.Spec.Version = version
	return p
}

func (p *upgradeBuilder) LogReadyCondition(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setLogReadyCondition(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) ImageReadyCondition(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setImageReadyCondition(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) RepoProvisionedCondition(status corev1.ConditionStatus, _, _ string) *upgradeBuilder {
	setRepoProvisionedCondition(p.upgrade, status, "", "")
	return p
}

func (p *upgradeBuilder) NodeUpgradeStatus(nodeName string, state, reason, message string) *upgradeBuilder {
	setNodeUpgradeStatus(p.upgrade, nodeName, state, reason, message)
	return p
}

func (p *upgradeBuilder) ImageIDStatus(imageName string) *upgradeBuilder {
	p.upgrade.Status.ImageID = imageName
	return p
}

func (p *upgradeBuilder) UpgradeLogStatus(upgradeLogName string) *upgradeBuilder {
	p.upgrade.Status.UpgradeLog = upgradeLogName
	return p
}

func (p *upgradeBuilder) NodesPreparedCondition(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setNodesPreparedCondition(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) NodesUpgradedCondition(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setNodesUpgradedCondition(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) ChartUpgradeStatus(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setHelmChartUpgradeStatus(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) InitStatus() *upgradeBuilder {
	initStatus(p.upgrade)
	return p
}

func (p *upgradeBuilder) Build() *harvesterv1.Upgrade {
	return p.upgrade
}

type versionBuilder struct {
	version *harvesterv1.Version
}

func newVersionBuilder(name string) *versionBuilder {
	return &versionBuilder{
		version: &harvesterv1.Version{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeNamespace,
			},
		},
	}
}

func (v *versionBuilder) Build() *harvesterv1.Version {
	return v.version
}

type planBuilder struct {
	plan *upgradev1.Plan
}

func newPlanBuilder(name string) *planBuilder {
	return &planBuilder{
		plan: &upgradev1.Plan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: sucNamespace,
			},
		},
	}
}

func (p *planBuilder) WithLabel(key, value string) *planBuilder {
	if p.plan.Labels == nil {
		p.plan.Labels = make(map[string]string)
	}
	p.plan.Labels[key] = value
	return p
}

func (p *planBuilder) Concurrency(concurrency int) *planBuilder {
	p.plan.Spec.Concurrency = int64(concurrency)
	return p
}

func (p *planBuilder) Version(version string) *planBuilder {
	p.plan.Spec.Version = version
	return p
}

func (p *planBuilder) Hash(hash string) *planBuilder {
	p.plan.Status.LatestHash = hash
	return p
}

func (p *planBuilder) Build() *upgradev1.Plan {
	return p.plan
}

type nodeBuilder struct {
	node *corev1.Node
}

func newNodeBuilder(name string) *nodeBuilder {
	return &nodeBuilder{
		node: &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

func (n *nodeBuilder) ControlPlane() *nodeBuilder {
	n.WithLabel(util.KubeControlPlaneNodeLabelKey, "true")
	return n
}

func (n *nodeBuilder) Managed() *nodeBuilder {
	n.WithLabel(harvesterManagedLabel, "true")
	return n
}

func (n *nodeBuilder) WithLabel(key, value string) *nodeBuilder {
	if n.node.Labels == nil {
		n.node.Labels = make(map[string]string)
	}
	n.node.Labels[key] = value
	return n
}

func (n *nodeBuilder) Build() *corev1.Node {
	return n.node
}

func upgradeReference(upgrade *harvesterv1.Upgrade) metav1.OwnerReference {
	return metav1.OwnerReference{
		Name:       upgrade.Name,
		Kind:       upgrade.Kind,
		UID:        upgrade.UID,
		APIVersion: upgrade.APIVersion,
	}
}

// removeItemFromSlice removes one element at index i from the slice. By
// removing the element, it simply copies the last element in the slice to the
// slot at index i and returns the same slice but excluding the last element.
// That is to say, the order of elements in the slice might change, depending
// on what element is going to be removed.
func removeItemFromSlice(slice []string, i int) []string {
	slice[i] = slice[len(slice)-1]
	return slice[:len(slice)-1]
}

func difference(setA, setB map[string]bool) []string {
	var diff []string
	for key := range setA {
		if !setB[key] {
			diff = append(diff, key)
		}
	}
	return diff
}
