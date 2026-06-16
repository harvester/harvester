package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/* Hugepages
 *
 * These are the type definitions on which the API structure for hugepage
 * support of Harvester is based.
 * There are two ways to utilize hugepages: Transparent Hugepages and HugeTLBFS.
 * The actual size of Hugepages differs between processor architectures and
 * also depends on what the processor implementation supports. On modern x86_64
 * processors it is common to see 2MiB and 1GiB supported.
 *
 * We are currently (2025-07-21) only exposing Transparent Hugepages here.
 */

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=hugetlb,scope=Cluster
// +kubebuilder:subresource:status

type Hugepage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HugepageSpec   `json:"spec"`
	Status HugepageStatus `json:"status,omitempty"`
}

type HugepageSpec struct {
	// +optional
	// +kubebuilder:validation:Optional
	Transparent THPConfig `json:"transparent,omitempty"`
}

type HugepageStatus struct {
	// +optional
	// +kubebuilder:validation:Optional
	Transparent THPConfig `json:"transparent,omitempty"`

	// +optional
	// +kubebuilder:validation:Optional
	Meminfo Meminfo `json:"meminfo"`
}

type Meminfo struct {
	// +optional
	// +kubebuilder:default:=0
	AnonHugePages uint64 `json:"anonHugePages"`

	// +optional
	// +kubebuilder:default:=0
	ShmemHugePages uint64 `json:"shmemHugePages"`

	// +optional
	// +kubebuilder:default:=0
	HugePagesTotal uint64 `json:"hugePagesTotal"`

	// +optional
	// +kubebuilder:default:=0
	HugePagesFree uint64 `json:"hugePagesFree"`

	// +optional
	// +kubebuilder:default:=0
	HugePagesRsvd uint64 `json:"hugePagesRsvd"`

	// +optional
	// +kubebuilder:default:=0
	HugePagesSurp uint64 `json:"hugePagesSurp"`

	// +optional
	// +kubebuilder:default:=0
	HugepageSize uint64 `json:"hugepageSize"`
}

type THPConfig struct {
	// +optional
	// +kubebuilder:validation:Enum=always;madvise;never
	// +kubebuilder:default=always
	Enabled THPEnabled `json:"enabled"`

	// +optional
	// +kubebuilder:validation:Enum=always;within_size;advise;never;deny;force
	// +kubebuilder:default=never
	ShmemEnabled THPShmemEnabled `json:"shmemEnabled"`

	// +optional
	// +kubebuilder:validation:Enum=always;defer;defer+madvise;madvise;never
	// +kubebuilder:default=defer
	Defrag THPDefrag `json:"defrag"`
}

type THPEnabled string

const (
	THPEnabledAlways  THPEnabled = "always"
	THPEnabledMadvise THPEnabled = "madvise"
	THPEnabledNever   THPEnabled = "never"
)

type THPShmemEnabled string

const (
	THPShmemEnabledAlways     THPShmemEnabled = "always"
	THPShmemEnabledWithinSize THPShmemEnabled = "within_size"
	THPShmemEnabledAdvise     THPShmemEnabled = "advise"
	THPShmemEnabledNever      THPShmemEnabled = "never"
	THPShmemEnabledDeny       THPShmemEnabled = "deny"
	THPShmemEnabledForce      THPShmemEnabled = "force"
)

type THPDefrag string

const (
	THPDefragAlways          THPDefrag = "always"
	THPDefragDefer           THPDefrag = "defer"
	THPDefragDeferAndMadvise THPDefrag = "defer+madvise"
	THPDefragMadvise         THPDefrag = "madvise"
	THPDefragNever           THPDefrag = "never"
)
