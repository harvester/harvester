package v1

// Copyright 2019 Rancher Labs, Inc.
// SPDX-License-Identifier: Apache-2.0

import (
	"time"

	"github.com/kubereboot/kured/pkg/timewindow"
	"github.com/rancher/system-upgrade-controller/pkg/apis/condition"
	"github.com/rancher/wrangler/v3/pkg/genericcondition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	// PlanLatestResolved indicates that the latest version as per the spec has been determined.
	PlanLatestResolved = condition.Cond("LatestResolved")
	// PlanSpecValidated indicates that the plan spec has been validated.
	PlanSpecValidated = condition.Cond("Validated")
	// PlanComplete indicates that the latest version of the plan has completed on all selected nodes.
	PlanComplete = condition.Cond("Complete")
)

// +genclient
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.upgrade.image`
// +kubebuilder:printcolumn:name="Channel",type=string,JSONPath=`.spec.channel`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Complete",type=string,JSONPath=`.status.conditions[?(@.type=='Complete')].status`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.message!='')].message`
// +kubebuilder:printcolumn:name="Applying",type=string,JSONPath=`.status.applying`,priority=10
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Plan represents a set of Jobs to apply an upgrade (or other operation) to set of Nodes.
type Plan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlanSpec   `json:"spec,omitempty"`
	Status PlanStatus `json:"status,omitempty"`
}

// PlanSpec represents the user-configurable details of a Plan.
type PlanSpec struct {
	// The maximum number of concurrent nodes to apply this update on.
	Concurrency int64 `json:"concurrency,omitempty"`
	// Sets ActiveDeadlineSeconds on Jobs generated to apply this Plan.
	// If the Job does not complete within this time, the Plan will stop processing until it is updated to trigger a redeploy.
	// If set to 0, Jobs have no deadline. If not set, the controller default value is used.
	JobActiveDeadlineSecs *int64 `json:"jobActiveDeadlineSecs,omitempty"`
	// Select which nodes this plan can be applied to.
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// The service account for the pod to use. As with normal pods, if not specified the default service account from the namespace will be assigned.
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// A URL that returns HTTP 302 with the last path element of the value returned in the Location header assumed to be an image tag (after munging "+" to "-").
	Channel string `json:"channel,omitempty"`
	// Providing a value for version will prevent polling/resolution of the channel if specified.
	Version string `json:"version,omitempty"`
	// Secrets to be mounted into the Job Pod.
	Secrets []SecretSpec `json:"secrets,omitempty"`
	// Specify which node taints should be tolerated by pods applying the upgrade.
	// Anything specified here is appended to the default of:
	// - `{key: node.kubernetes.io/unschedulable, effect: NoSchedule, operator: Exists}`
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Jobs for exclusive plans cannot be run alongside any other exclusive plan.
	Exclusive bool `json:"exclusive,omitempty"`
	// A time window in which to execute Jobs for this Plan.
	// Jobs will not be generated outside this time window, but may continue executing into the window once started.
	Window *TimeWindowSpec `json:"window,omitempty"`
	// The prepare init container, if specified, is run before cordon/drain which is run before the upgrade container.
	Prepare *ContainerSpec `json:"prepare,omitempty"`
	// The upgrade container; must be specified.
	Upgrade *ContainerSpec `json:"upgrade"`
	// If Cordon is true, the node is cordoned before the upgrade container is run.
	// If drain is specified, the value for cordon is ignored, and the node is cordoned.
	// If neither drain nor cordon are specified and the node is marked as schedulable=false it will not be marked as schedulable=true when the Job completes.
	Cordon bool `json:"cordon,omitempty"`
	// Configuration for draining nodes prior to upgrade. If left unspecified, no drain will be performed.
	Drain *DrainSpec `json:"drain,omitempty"`
	// Image Pull Secrets, used to pull images for the Job.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Time after a Job for one Node is complete before a new Job will be created for the next Node.
	PostCompleteDelay *metav1.Duration `json:"postCompleteDelay,omitempty"`
	// Priority Class Name of Job, if specified.
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// Label key-value pairs to apply to a node when the job for this plan completes successfully.
	// Values may contain `$(LATEST_HASH)` or `$(LATEST_VERSION)`, which will be expanded from the plan status.
	PostCompleteLabels map[string]string `json:"postCompleteLabels,omitempty"`
}

// PlanStatus represents the resulting state from processing Plan events.
type PlanStatus struct {
	// `LatestResolved` indicates that the latest version as per the spec has been determined.
	// `Validated` indicates that the plan spec has been validated.
	// `Complete` indicates that the latest version of the plan has completed on all selected nodes. If any Jobs for the Plan fail to complete, this condition will remain false, and the reason and message will reflect the source of the error.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`

	// The latest version, as resolved from .spec.version, or the channel server.
	LatestVersion string `json:"latestVersion,omitempty"`
	// The hash of the most recently applied plan .spec.
	LatestHash string `json:"latestHash,omitempty"`
	// List of Node names that the Plan is currently being applied on.
	Applying []string `json:"applying,omitempty"`
}

// ContainerSpec is a simplified container template spec, used to configure the prepare and upgrade
// containers of the Job Pod.
type ContainerSpec struct {
	// Image name. If the tag is omitted, the value from .status.latestVersion will be used.
	// +kubebuilder:validation:Required
	Image           string                  `json:"image"`
	Command         []string                `json:"command,omitempty"`
	Args            []string                `json:"args,omitempty"`
	Env             []corev1.EnvVar         `json:"envs,omitempty"`
	EnvFrom         []corev1.EnvFromSource  `json:"envFrom,omitempty"`
	Volumes         []VolumeSpec            `json:"volumes,omitempty"`
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

// HostPath volume to mount into the pod
type VolumeSpec struct {
	// Name of the Volume as it will appear within the Pod spec.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// Path on the host to mount.
	// +kubebuilder:validation:Required
	Source string `json:"source"`
	// Path to mount the Volume at within the Pod.
	// +kubebuilder:validation:Required
	Destination string `json:"destination"`
}

// DrainSpec encapsulates kubectl drain parameters minus node/pod selectors. See:
// - https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/
// - https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#drain
type DrainSpec struct {
	// If a string, this is passed through directly to the `kubectl drain` command.
	// If an int, this represents the duration as a count of nanoseconds, and will be converted to a duration string when passed to the `kubectl drain` command.
	Timeout                  *intstr.IntOrString   `json:"timeout,omitempty"`
	GracePeriod              *int32                `json:"gracePeriod,omitempty"`
	DeleteLocalData          *bool                 `json:"deleteLocalData,omitempty"`
	DeleteEmptydirData       *bool                 `json:"deleteEmptydirData,omitempty"`
	IgnoreDaemonSets         *bool                 `json:"ignoreDaemonSets,omitempty"`
	Force                    bool                  `json:"force,omitempty"`
	DisableEviction          bool                  `json:"disableEviction,omitempty"`
	SkipWaitForDeleteTimeout int                   `json:"skipWaitForDeleteTimeout,omitempty"`
	PodSelector              *metav1.LabelSelector `json:"podSelector,omitempty"`
}

// SecretSpec describes a Secret to be mounted for prepare/upgrade containers.
type SecretSpec struct {
	// Secret name
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// Path to mount the Secret volume within the Pod.
	// +kubebuilder:validation:Required
	Path string `json:"path"`
	// If set to true, the Secret contents will not be hashed, and changes to the Secret will not trigger new application of the Plan.
	IgnoreUpdates bool `json:"ignoreUpdates,omitempty"`
	// Mode to mount the Secret volume with.
	// +kubebuilder:validation:Optional
	DefaultMode *int32 `json:"defaultMode,omitempty"`
}

// +kubebuilder:validation:Enum={"0","su","sun","sunday","1","mo","mon","monday","2","tu","tue","tuesday","3","we","wed","wednesday","4","th","thu","thursday","5","fr","fri","friday","6","sa","sat","saturday"}
type Day string

// TimeWindowSpec describes a time window in which a Plan should be processed.
type TimeWindowSpec struct {
	// Days that this time window is valid for
	// +kubebuilder:validation:MinItems=1
	Days []Day `json:"days,omitempty"`
	// Start of the time window.
	StartTime string `json:"startTime,omitempty"`
	// End of the time window.
	EndTime string `json:"endTime,omitempty"`
	// Time zone for the time window; if not specified UTC will be used.
	TimeZone string `json:"timeZone,omitempty"`
}

func (tws *TimeWindowSpec) Contains(t time.Time) bool {
	days := make([]string, len(tws.Days))
	for i, day := range tws.Days {
		days[i] = string(day)
	}
	tw, err := timewindow.New(days, tws.StartTime, tws.EndTime, tws.TimeZone)
	if err != nil {
		return false
	}
	return tw.Contains(t)
}
