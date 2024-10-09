package v1

// Copyright 2019 Rancher Labs, Inc.
// SPDX-License-Identifier: Apache-2.0

import (
	"time"

	"github.com/rancher/system-upgrade-controller/pkg/apis/condition"
	"github.com/rancher/wrangler/pkg/genericcondition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Plan represents a "JobSet" of ApplyingNodes
type Plan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlanSpec   `json:"spec,omitempty"`
	Status PlanStatus `json:"status,omitempty"`
}

// PlanSpec represents the user-configurable details of a Plan.
type PlanSpec struct {
	Concurrency           int64                 `json:"concurrency,omitempty"`
	JobActiveDeadlineSecs int64                 `json:"jobActiveDeadlineSecs,omitempty"`
	NodeSelector          *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	ServiceAccountName    string                `json:"serviceAccountName,omitempty"`

	Channel string       `json:"channel,omitempty"`
	Version string       `json:"version,omitempty"`
	Secrets []SecretSpec `json:"secrets,omitempty"`

	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	Exclusive bool `json:"exclusive,omitempty"`

	Prepare          *ContainerSpec                `json:"prepare,omitempty"`
	Cordon           bool                          `json:"cordon,omitempty"`
	Drain            *DrainSpec                    `json:"drain,omitempty"`
	Upgrade          *ContainerSpec                `json:"upgrade,omitempty" wrangler:"required"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// PlanStatus represents the resulting state from processing Plan events.
type PlanStatus struct {
	Conditions    []genericcondition.GenericCondition `json:"conditions,omitempty"`
	LatestVersion string                              `json:"latestVersion,omitempty"`
	LatestHash    string                              `json:"latestHash,omitempty"`
	Applying      []string                            `json:"applying,omitempty"`
}

// ContainerSpec is a simplified container template.
type ContainerSpec struct {
	Image           string                  `json:"image,omitempty"`
	Command         []string                `json:"command,omitempty"`
	Args            []string                `json:"args,omitempty"`
	Env             []corev1.EnvVar         `json:"envs,omitempty"`
	EnvFrom         []corev1.EnvFromSource  `json:"envFrom,omitempty"`
	Volumes         []VolumeSpec            `json:"volumes,omitempty"`
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

type VolumeSpec struct {
	Name        string `json:"name,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
}

// DrainSpec encapsulates `kubectl drain` parameters minus node/pod selectors.
type DrainSpec struct {
	Timeout                  *time.Duration        `json:"timeout,omitempty"`
	GracePeriod              *int32                `json:"gracePeriod,omitempty"`
	DeleteLocalData          *bool                 `json:"deleteLocalData,omitempty"`
	DeleteEmptydirData       *bool                 `json:"deleteEmptydirData,omitempty"`
	IgnoreDaemonSets         *bool                 `json:"ignoreDaemonSets,omitempty"`
	Force                    bool                  `json:"force,omitempty"`
	DisableEviction          bool                  `json:"disableEviction,omitempty"`
	SkipWaitForDeleteTimeout int                   `json:"skipWaitForDeleteTimeout,omitempty"`
	PodSelector              *metav1.LabelSelector `json:"podSelector,omitempty"`
}

// SecretSpec describes a secret to be mounted for prepare/upgrade containers.
type SecretSpec struct {
	Name          string `json:"name,omitempty"`
	Path          string `json:"path,omitempty"`
	IgnoreUpdates bool   `json:"ignoreUpdates,omitempty"`
}
