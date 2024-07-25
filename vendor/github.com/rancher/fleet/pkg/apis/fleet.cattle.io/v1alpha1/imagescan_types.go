package v1alpha1

import (
	"github.com/rancher/wrangler/v3/pkg/genericcondition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	InternalSchemeBuilder.Register(&ImageScan{}, &ImageScanList{})
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Latest",type=string,JSONPath=`.status.latestTag`

type ImageScan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageScanSpec   `json:"spec,omitempty"`
	Status ImageScanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImageScanList contains a list of ImageScan
type ImageScanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageScan `json:"items"`
}

// API is taken from https://github.com/fluxcd/image-reflector-controller
type ImageScanSpec struct {
	// TagName is the tag ref that needs to be put in manifest to replace fields
	// +nullable
	TagName string `json:"tagName,omitempty"`

	// GitRepo reference name
	// +nullable
	GitRepoName string `json:"gitrepoName,omitempty"`

	// Image is the name of the image repository
	// +required
	// +nullable
	Image string `json:"image,omitempty"`

	// Interval is the length of time to wait between
	// scans of the image repository.
	// +nullable
	// +required
	Interval metav1.Duration `json:"interval,omitempty"`

	// SecretRef can be given the name of a secret containing
	// credentials to use for the image registry. The secret should be
	// created with `kubectl create secret docker-registry`, or the
	// equivalent.
	// +nullable
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// This flag tells the controller to suspend subsequent image scans.
	// It does not apply to already started scans. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// Policy gives the particulars of the policy to be followed in
	// selecting the most recent image
	// +optional
	Policy ImagePolicyChoice `json:"policy"`
}

// ImagePolicyChoice is a union of all the types of policy that can be
// supplied.
type ImagePolicyChoice struct {
	// SemVer gives a semantic version range to check against the tags
	// available.
	// +optional
	// +nullable
	SemVer *SemVerPolicy `json:"semver,omitempty"`
	// Alphabetical set of rules to use for alphabetical ordering of the tags.
	// +optional
	// +nullable
	Alphabetical *AlphabeticalPolicy `json:"alphabetical,omitempty"`
}

// SemVerPolicy specifies a semantic version policy.
type SemVerPolicy struct {
	// Range gives a semver range for the image tag; the highest
	// version within the range that's a tag yields the latest image.
	// +optional
	// +nullable
	Range string `json:"range"`
}

// AlphabeticalPolicy specifies a alphabetical ordering policy.
type AlphabeticalPolicy struct {
	// Order specifies the sorting order of the tags. Given the letters of the
	// alphabet as tags, ascending order would select Z, and descending order
	// would select A.
	// +optional
	// +nullable
	Order string `json:"order,omitempty"`
}

const (
	ImageScanScanCondition = "ImageScanned"
	ImageScanSyncCondition = "ImageSynced"
)

type ImageScanStatus struct {
	// +optional
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`

	// LastScanTime is the last time image was scanned
	LastScanTime metav1.Time `json:"lastScanTime,omitempty"`

	// LatestImage gives the first in the list of images scanned by
	// the image repository, when filtered and ordered according to
	// the policy.
	LatestImage string `json:"latestImage,omitempty"`

	// Latest tag is the latest tag filtered by the policy
	LatestTag string `json:"latestTag,omitempty"`

	// LatestDigest is the digest of latest tag
	LatestDigest string `json:"latestDigest,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// CanonicalName is the name of the image repository with all the
	// implied bits made explicit; e.g., `docker.io/library/alpine`
	// rather than `alpine`.
	// +optional
	CanonicalImageName string `json:"canonicalImageName,omitempty"`
}
