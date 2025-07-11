/*
Copyright 2022 The CDI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	ocpconfigv1 "github.com/openshift/api/config/v1"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	runtimecache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdiv1utils "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1/utils"
	"kubevirt.io/containerized-data-importer/pkg/client/clientset/versioned/scheme"
	"kubevirt.io/containerized-data-importer/pkg/common"
	featuregates "kubevirt.io/containerized-data-importer/pkg/feature-gates"
	"kubevirt.io/containerized-data-importer/pkg/token"
	"kubevirt.io/containerized-data-importer/pkg/util"
	sdkapi "kubevirt.io/controller-lifecycle-operator-sdk/api"
)

const (
	// DataVolName provides a const to use for creating volumes in pod specs
	DataVolName = "cdi-data-vol"

	// ScratchVolName provides a const to use for creating scratch pvc volumes in pod specs
	ScratchVolName = "cdi-scratch-vol"

	// AnnAPIGroup is the APIGroup for CDI
	AnnAPIGroup = "cdi.kubevirt.io"
	// AnnCreatedBy is a pod annotation indicating if the pod was created by the PVC
	AnnCreatedBy = AnnAPIGroup + "/storage.createdByController"
	// AnnPodPhase is a PVC annotation indicating the related pod progress (phase)
	AnnPodPhase = AnnAPIGroup + "/storage.pod.phase"
	// AnnPodReady tells whether the pod is ready
	AnnPodReady = AnnAPIGroup + "/storage.pod.ready"
	// AnnPodRestarts is a PVC annotation that tells how many times a related pod was restarted
	AnnPodRestarts = AnnAPIGroup + "/storage.pod.restarts"
	// AnnPopulatedFor is a PVC annotation telling the datavolume controller that the PVC is already populated
	AnnPopulatedFor = AnnAPIGroup + "/storage.populatedFor"
	// AnnPrePopulated is a PVC annotation telling the datavolume controller that the PVC is already populated
	AnnPrePopulated = AnnAPIGroup + "/storage.prePopulated"
	// AnnPriorityClassName is PVC annotation to indicate the priority class name for importer, cloner and uploader pod
	AnnPriorityClassName = AnnAPIGroup + "/storage.pod.priorityclassname"
	// AnnExternalPopulation annotation marks a PVC as "externally populated", allowing the import-controller to skip it
	AnnExternalPopulation = AnnAPIGroup + "/externalPopulation"

	// AnnDeleteAfterCompletion is PVC annotation for deleting DV after completion
	AnnDeleteAfterCompletion = AnnAPIGroup + "/storage.deleteAfterCompletion"
	// AnnPodRetainAfterCompletion is PVC annotation for retaining transfer pods after completion
	AnnPodRetainAfterCompletion = AnnAPIGroup + "/storage.pod.retainAfterCompletion"

	// AnnPreviousCheckpoint provides a const to indicate the previous snapshot for a multistage import
	AnnPreviousCheckpoint = AnnAPIGroup + "/storage.checkpoint.previous"
	// AnnCurrentCheckpoint provides a const to indicate the current snapshot for a multistage import
	AnnCurrentCheckpoint = AnnAPIGroup + "/storage.checkpoint.current"
	// AnnFinalCheckpoint provides a const to indicate whether the current checkpoint is the last one
	AnnFinalCheckpoint = AnnAPIGroup + "/storage.checkpoint.final"
	// AnnCheckpointsCopied is a prefix for recording which checkpoints have already been copied
	AnnCheckpointsCopied = AnnAPIGroup + "/storage.checkpoint.copied"

	// AnnCurrentPodID keeps track of the latest pod servicing this PVC
	AnnCurrentPodID = AnnAPIGroup + "/storage.checkpoint.pod.id"
	// AnnMultiStageImportDone marks a multi-stage import as totally finished
	AnnMultiStageImportDone = AnnAPIGroup + "/storage.checkpoint.done"

	// AnnPopulatorProgress is a standard annotation that can be used progress reporting
	AnnPopulatorProgress = AnnAPIGroup + "/storage.populator.progress"

	// AnnPreallocationRequested provides a const to indicate whether preallocation should be performed on the PV
	AnnPreallocationRequested = AnnAPIGroup + "/storage.preallocation.requested"
	// AnnPreallocationApplied provides a const for PVC preallocation annotation
	AnnPreallocationApplied = AnnAPIGroup + "/storage.preallocation"

	// AnnRunningCondition provides a const for the running condition
	AnnRunningCondition = AnnAPIGroup + "/storage.condition.running"
	// AnnRunningConditionMessage provides a const for the running condition
	AnnRunningConditionMessage = AnnAPIGroup + "/storage.condition.running.message"
	// AnnRunningConditionReason provides a const for the running condition
	AnnRunningConditionReason = AnnAPIGroup + "/storage.condition.running.reason"

	// AnnBoundCondition provides a const for the running condition
	AnnBoundCondition = AnnAPIGroup + "/storage.condition.bound"
	// AnnBoundConditionMessage provides a const for the running condition
	AnnBoundConditionMessage = AnnAPIGroup + "/storage.condition.bound.message"
	// AnnBoundConditionReason provides a const for the running condition
	AnnBoundConditionReason = AnnAPIGroup + "/storage.condition.bound.reason"

	// AnnSourceRunningCondition provides a const for the running condition
	AnnSourceRunningCondition = AnnAPIGroup + "/storage.condition.source.running"
	// AnnSourceRunningConditionMessage provides a const for the running condition
	AnnSourceRunningConditionMessage = AnnAPIGroup + "/storage.condition.source.running.message"
	// AnnSourceRunningConditionReason provides a const for the running condition
	AnnSourceRunningConditionReason = AnnAPIGroup + "/storage.condition.source.running.reason"

	// AnnVddkVersion shows the last VDDK library version used by a DV's importer pod
	AnnVddkVersion = AnnAPIGroup + "/storage.pod.vddk.version"
	// AnnVddkHostConnection shows the last ESX host that serviced a DV's importer pod
	AnnVddkHostConnection = AnnAPIGroup + "/storage.pod.vddk.host"
	// AnnVddkInitImageURL saves a per-DV VDDK image URL on the PVC
	AnnVddkInitImageURL = AnnAPIGroup + "/storage.pod.vddk.initimageurl"

	// AnnRequiresScratch provides a const for our PVC requiring scratch annotation
	AnnRequiresScratch = AnnAPIGroup + "/storage.import.requiresScratch"

	// AnnRequiresDirectIO provides a const for our PVC requiring direct io annotation (due to OOMs we need to try qemu cache=none)
	AnnRequiresDirectIO = AnnAPIGroup + "/storage.import.requiresDirectIo"
	// OOMKilledReason provides a value that container runtimes must return in the reason field for an OOMKilled container
	OOMKilledReason = "OOMKilled"

	// AnnContentType provides a const for the PVC content-type
	AnnContentType = AnnAPIGroup + "/storage.contentType"

	// AnnSource provide a const for our PVC import source annotation
	AnnSource = AnnAPIGroup + "/storage.import.source"
	// AnnEndpoint provides a const for our PVC endpoint annotation
	AnnEndpoint = AnnAPIGroup + "/storage.import.endpoint"

	// AnnSecret provides a const for our PVC secretName annotation
	AnnSecret = AnnAPIGroup + "/storage.import.secretName"
	// AnnCertConfigMap is the name of a configmap containing tls certs
	AnnCertConfigMap = AnnAPIGroup + "/storage.import.certConfigMap"
	// AnnRegistryImportMethod provides a const for registry import method annotation
	AnnRegistryImportMethod = AnnAPIGroup + "/storage.import.registryImportMethod"
	// AnnRegistryImageStream provides a const for registry image stream annotation
	AnnRegistryImageStream = AnnAPIGroup + "/storage.import.registryImageStream"
	// AnnImportPod provides a const for our PVC importPodName annotation
	AnnImportPod = AnnAPIGroup + "/storage.import.importPodName"
	// AnnDiskID provides a const for our PVC diskId annotation
	AnnDiskID = AnnAPIGroup + "/storage.import.diskId"
	// AnnUUID provides a const for our PVC uuid annotation
	AnnUUID = AnnAPIGroup + "/storage.import.uuid"
	// AnnBackingFile provides a const for our PVC backing file annotation
	AnnBackingFile = AnnAPIGroup + "/storage.import.backingFile"
	// AnnThumbprint provides a const for our PVC backing thumbprint annotation
	AnnThumbprint = AnnAPIGroup + "/storage.import.vddk.thumbprint"
	// AnnExtraHeaders provides a const for our PVC extraHeaders annotation
	AnnExtraHeaders = AnnAPIGroup + "/storage.import.extraHeaders"
	// AnnSecretExtraHeaders provides a const for our PVC secretExtraHeaders annotation
	AnnSecretExtraHeaders = AnnAPIGroup + "/storage.import.secretExtraHeaders"

	// AnnCloneToken is the annotation containing the clone token
	AnnCloneToken = AnnAPIGroup + "/storage.clone.token"
	// AnnExtendedCloneToken is the annotation containing the long term clone token
	AnnExtendedCloneToken = AnnAPIGroup + "/storage.extended.clone.token"
	// AnnPermissiveClone annotation allows the clone-controller to skip the clone size validation
	AnnPermissiveClone = AnnAPIGroup + "/permissiveClone"
	// AnnOwnerUID annotation has the owner UID
	AnnOwnerUID = AnnAPIGroup + "/ownerUID"
	// AnnCloneType is the comuuted/requested clone type
	AnnCloneType = AnnAPIGroup + "/cloneType"
	// AnnCloneSourcePod name of the source clone pod
	AnnCloneSourcePod = AnnAPIGroup + "/storage.sourceClonePodName"

	// AnnUploadRequest marks that a PVC should be made available for upload
	AnnUploadRequest = AnnAPIGroup + "/storage.upload.target"

	// AnnCheckStaticVolume checks if a statically allocated PV exists before creating the target PVC.
	// If so, PVC is still created but population is skipped
	AnnCheckStaticVolume = AnnAPIGroup + "/storage.checkStaticVolume"

	// AnnPersistentVolumeList is an annotation storing a list of PV names
	AnnPersistentVolumeList = AnnAPIGroup + "/storage.persistentVolumeList"

	// AnnPopulatorKind annotation is added to a PVC' to specify the population kind, so it's later
	// checked by the common populator watches.
	AnnPopulatorKind = AnnAPIGroup + "/storage.populator.kind"
	// AnnUsePopulator annotation indicates if the datavolume population will use populators
	AnnUsePopulator = AnnAPIGroup + "/storage.usePopulator"

	// AnnDefaultStorageClass is the annotation indicating that a storage class is the default one
	AnnDefaultStorageClass = "storageclass.kubernetes.io/is-default-class"
	// AnnDefaultVirtStorageClass is the annotation indicating that a storage class is the default one for virtualization purposes
	AnnDefaultVirtStorageClass = "storageclass.kubevirt.io/is-default-virt-class"
	// AnnDefaultSnapshotClass is the annotation indicating that a snapshot class is the default one
	AnnDefaultSnapshotClass = "snapshot.storage.kubernetes.io/is-default-class"

	// AnnSourceVolumeMode is the volume mode of the source PVC specified as an annotation on snapshots
	AnnSourceVolumeMode = AnnAPIGroup + "/storage.import.sourceVolumeMode"

	// AnnOpenShiftImageLookup is the annotation for OpenShift image stream lookup
	AnnOpenShiftImageLookup = "alpha.image.policy.openshift.io/resolve-names"

	// AnnCloneRequest sets our expected annotation for a CloneRequest
	AnnCloneRequest = "k8s.io/CloneRequest"
	// AnnCloneOf is used to indicate that cloning was complete
	AnnCloneOf = "k8s.io/CloneOf"

	// AnnPodNetwork is used for specifying Pod Network
	AnnPodNetwork = "k8s.v1.cni.cncf.io/networks"
	// AnnPodMultusDefaultNetwork is used for specifying default Pod Network
	AnnPodMultusDefaultNetwork = "v1.multus-cni.io/default-network"
	// AnnPodSidecarInjectionIstio is used for enabling/disabling Pod istio/AspenMesh sidecar injection
	AnnPodSidecarInjectionIstio = "sidecar.istio.io/inject"
	// AnnPodSidecarInjectionIstioDefault is the default value passed for AnnPodSidecarInjection
	AnnPodSidecarInjectionIstioDefault = "false"
	// AnnPodSidecarInjectionLinkerd is used to enable/disable linkerd sidecar injection
	AnnPodSidecarInjectionLinkerd = "linkerd.io/inject"
	// AnnPodSidecarInjectionLinkerdDefault is the default value passed for AnnPodSidecarInjectionLinkerd
	AnnPodSidecarInjectionLinkerdDefault = "disabled"

	// AnnImmediateBinding provides a const to indicate whether immediate binding should be performed on the PV (overrides global config)
	AnnImmediateBinding = AnnAPIGroup + "/storage.bind.immediate.requested"

	// AnnSelectedNode annotation is added to a PVC that has been triggered by scheduler to
	// be dynamically provisioned. Its value is the name of the selected node.
	AnnSelectedNode = "volume.kubernetes.io/selected-node"

	// AnnGarbageCollected is a PVC annotation indicating that the PVC was garbage collected
	AnnGarbageCollected = AnnAPIGroup + "/garbageCollected"

	// CloneUniqueID is used as a special label to be used when we search for the pod
	CloneUniqueID = AnnAPIGroup + "/storage.clone.cloneUniqeId"

	// CloneSourceInUse is reason for event created when clone source pvc is in use
	CloneSourceInUse = "CloneSourceInUse"

	// CloneComplete message
	CloneComplete = "Clone Complete"

	cloneTokenLeeway = 10 * time.Second

	// Default value for preallocation option if not defined in DV or CDIConfig
	defaultPreallocation = false

	// ErrStartingPod provides a const to indicate that a pod wasn't able to start without providing sensitive information (reason)
	ErrStartingPod = "ErrStartingPod"
	// MessageErrStartingPod provides a const to indicate that a pod wasn't able to start without providing sensitive information (message)
	MessageErrStartingPod = "Error starting pod '%s': For more information, request access to cdi-deploy logs from your sysadmin"
	// ErrClaimNotValid provides a const to indicate a claim is not valid
	ErrClaimNotValid = "ErrClaimNotValid"
	// ErrExceededQuota provides a const to indicate the claim has exceeded the quota
	ErrExceededQuota = "ErrExceededQuota"
	// ErrIncompatiblePVC provides a const to indicate a clone is not possible due to an incompatible PVC
	ErrIncompatiblePVC = "ErrIncompatiblePVC"

	// SourceHTTP is the source type HTTP, if unspecified or invalid, it defaults to SourceHTTP
	SourceHTTP = "http"
	// SourceS3 is the source type S3
	SourceS3 = "s3"
	// SourceGCS is the source type GCS
	SourceGCS = "gcs"
	// SourceGlance is the source type of glance
	SourceGlance = "glance"
	// SourceNone means there is no source.
	SourceNone = "none"
	// SourceRegistry is the source type of Registry
	SourceRegistry = "registry"
	// SourceImageio is the source type ovirt-imageio
	SourceImageio = "imageio"
	// SourceVDDK is the source type of VDDK
	SourceVDDK = "vddk"

	// VolumeSnapshotClassSelected reports that a VolumeSnapshotClass was selected
	VolumeSnapshotClassSelected = "VolumeSnapshotClassSelected"
	// MessageStorageProfileVolumeSnapshotClassSelected reports that a VolumeSnapshotClass was selected according to StorageProfile
	MessageStorageProfileVolumeSnapshotClassSelected = "VolumeSnapshotClass selected according to StorageProfile"
	// MessageDefaultVolumeSnapshotClassSelected reports that the default VolumeSnapshotClass was selected
	MessageDefaultVolumeSnapshotClassSelected = "Default VolumeSnapshotClass selected"
	// MessageFirstVolumeSnapshotClassSelected reports that the first VolumeSnapshotClass was selected
	MessageFirstVolumeSnapshotClassSelected = "First VolumeSnapshotClass selected"

	// ClaimLost reason const
	ClaimLost = "ClaimLost"
	// NotFound reason const
	NotFound = "NotFound"

	// LabelDefaultInstancetype provides a default VirtualMachine{ClusterInstancetype,Instancetype} that can be used by a VirtualMachine booting from a given PVC
	LabelDefaultInstancetype = "instancetype.kubevirt.io/default-instancetype"
	// LabelDefaultInstancetypeKind provides a default kind of either VirtualMachineClusterInstancetype or VirtualMachineInstancetype
	LabelDefaultInstancetypeKind = "instancetype.kubevirt.io/default-instancetype-kind"
	// LabelDefaultPreference provides a default VirtualMachine{ClusterPreference,Preference} that can be used by a VirtualMachine booting from a given PVC
	LabelDefaultPreference = "instancetype.kubevirt.io/default-preference"
	// LabelDefaultPreferenceKind provides a default kind of either VirtualMachineClusterPreference or VirtualMachinePreference
	LabelDefaultPreferenceKind = "instancetype.kubevirt.io/default-preference-kind"

	// LabelDynamicCredentialSupport specifies if the OS supports updating credentials at runtime.
	//nolint:gosec // These are not credentials
	LabelDynamicCredentialSupport = "kubevirt.io/dynamic-credentials-support"

	// LabelExcludeFromVeleroBackup provides a const to indicate whether an object should be excluded from velero backup
	LabelExcludeFromVeleroBackup = "velero.io/exclude-from-backup"

	// ProgressDone this means we are DONE
	ProgressDone = "100.0%"

	// AnnEventSourceKind is the source kind that should be related to events
	AnnEventSourceKind = AnnAPIGroup + "/events.source.kind"
	// AnnEventSource is the source that should be related to events (namespace/name)
	AnnEventSource = AnnAPIGroup + "/events.source"

	// AnnAllowClaimAdoption is the annotation that allows a claim to be adopted by a DataVolume
	AnnAllowClaimAdoption = AnnAPIGroup + "/allowClaimAdoption"

	// AnnCdiCustomizeComponentHash annotation is a hash of all customizations that live under spec.CustomizeComponents
	AnnCdiCustomizeComponentHash = AnnAPIGroup + "/customizer-identifier"

	// AnnCreatedForDataVolume stores the UID of the datavolume that the PVC was created for
	AnnCreatedForDataVolume = AnnAPIGroup + "/createdForDataVolume"
)

// Size-detection pod error codes
const (
	NoErr int = iota
	ErrBadArguments
	ErrInvalidFile
	ErrInvalidPath
	ErrBadTermFile
	ErrUnknown
)

var (
	// BlockMode is raw block device mode
	BlockMode = corev1.PersistentVolumeBlock
	// FilesystemMode is filesystem device mode
	FilesystemMode = corev1.PersistentVolumeFilesystem

	// DefaultInstanceTypeLabels is a list of currently supported default instance type labels
	DefaultInstanceTypeLabels = []string{
		LabelDefaultInstancetype,
		LabelDefaultInstancetypeKind,
		LabelDefaultPreference,
		LabelDefaultPreferenceKind,
	}

	apiServerKeyOnce sync.Once
	apiServerKey     *rsa.PrivateKey

	// allowedAnnotations is a list of annotations
	// that can be propagated from the pvc/dv to a pod
	allowedAnnotations = map[string]string{
		AnnPodNetwork:                 "",
		AnnPodSidecarInjectionIstio:   AnnPodSidecarInjectionIstioDefault,
		AnnPodSidecarInjectionLinkerd: AnnPodSidecarInjectionLinkerdDefault,
		AnnPriorityClassName:          "",
		AnnPodMultusDefaultNetwork:    "",
	}

	validLabelsMatch = regexp.MustCompile(`^([\w.]+\.kubevirt.io|kubevirt.io)/[\w-]+$`)
)

// FakeValidator is a fake token validator
type FakeValidator struct {
	Match     string
	Operation token.Operation
	Name      string
	Namespace string
	Resource  metav1.GroupVersionResource
	Params    map[string]string
}

// Validate is a fake token validation
func (v *FakeValidator) Validate(value string) (*token.Payload, error) {
	if value != v.Match {
		return nil, fmt.Errorf("token does not match expected")
	}
	resource := metav1.GroupVersionResource{
		Resource: "persistentvolumeclaims",
	}
	return &token.Payload{
		Name:      v.Name,
		Namespace: v.Namespace,
		Operation: token.OperationClone,
		Resource:  resource,
		Params:    v.Params,
	}, nil
}

// MultiTokenValidator is a token validator that can validate both short and long tokens
type MultiTokenValidator struct {
	ShortTokenValidator token.Validator
	LongTokenValidator  token.Validator
}

// ValidatePVC validates a PVC
func (mtv *MultiTokenValidator) ValidatePVC(source, target *corev1.PersistentVolumeClaim) error {
	tok, v := mtv.getTokenAndValidator(target)
	return ValidateCloneTokenPVC(tok, v, source, target)
}

// ValidatePopulator valades a token for a populator
func (mtv *MultiTokenValidator) ValidatePopulator(vcs *cdiv1.VolumeCloneSource, pvc *corev1.PersistentVolumeClaim) error {
	if vcs.Namespace == pvc.Namespace {
		return nil
	}

	tok, v := mtv.getTokenAndValidator(pvc)

	tokenData, err := v.Validate(tok)
	if err != nil {
		return errors.Wrap(err, "error verifying token")
	}

	var tokenResourceName string
	switch vcs.Spec.Source.Kind {
	case "PersistentVolumeClaim":
		tokenResourceName = "persistentvolumeclaims"
	case "VolumeSnapshot":
		tokenResourceName = "volumesnapshots"
	}
	srcName := vcs.Spec.Source.Name

	return validateTokenData(tokenData, vcs.Namespace, srcName, pvc.Namespace, pvc.Name, string(pvc.UID), tokenResourceName)
}

func (mtv *MultiTokenValidator) getTokenAndValidator(pvc *corev1.PersistentVolumeClaim) (string, token.Validator) {
	v := mtv.LongTokenValidator
	tok, ok := pvc.Annotations[AnnExtendedCloneToken]
	if !ok {
		// if token doesn't exist, no prob for same namespace
		tok = pvc.Annotations[AnnCloneToken]
		v = mtv.ShortTokenValidator
	}
	return tok, v
}

// NewMultiTokenValidator returns a new multi token validator
func NewMultiTokenValidator(key *rsa.PublicKey) *MultiTokenValidator {
	return &MultiTokenValidator{
		ShortTokenValidator: NewCloneTokenValidator(common.CloneTokenIssuer, key),
		LongTokenValidator:  NewCloneTokenValidator(common.ExtendedCloneTokenIssuer, key),
	}
}

// NewCloneTokenValidator returns a new token validator
func NewCloneTokenValidator(issuer string, key *rsa.PublicKey) token.Validator {
	return token.NewValidator(issuer, key, cloneTokenLeeway)
}

// GetRequestedImageSize returns the PVC requested size
func GetRequestedImageSize(pvc *corev1.PersistentVolumeClaim) (string, error) {
	pvcSize, found := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if !found {
		return "", errors.Errorf("storage request is missing in pvc \"%s/%s\"", pvc.Namespace, pvc.Name)
	}
	return pvcSize.String(), nil
}

// GetVolumeMode returns the volumeMode from PVC handling default empty value
func GetVolumeMode(pvc *corev1.PersistentVolumeClaim) corev1.PersistentVolumeMode {
	return util.ResolveVolumeMode(pvc.Spec.VolumeMode)
}

// IsDataVolumeUsingDefaultStorageClass checks if the DataVolume is using the default StorageClass
func IsDataVolumeUsingDefaultStorageClass(dv *cdiv1.DataVolume) bool {
	return GetStorageClassFromDVSpec(dv) == nil
}

// GetStorageClassFromDVSpec returns the StorageClassName from DataVolume PVC or Storage spec
func GetStorageClassFromDVSpec(dv *cdiv1.DataVolume) *string {
	if dv.Spec.PVC != nil {
		return dv.Spec.PVC.StorageClassName
	}

	if dv.Spec.Storage != nil {
		return dv.Spec.Storage.StorageClassName
	}

	return nil
}

// getStorageClassByName looks up the storage class based on the name.
// If name is nil, it performs fallback to default according to the provided content type
// If no storage class is found, returns nil
func getStorageClassByName(ctx context.Context, client client.Client, name *string, contentType cdiv1.DataVolumeContentType) (*storagev1.StorageClass, error) {
	if name == nil {
		return getFallbackStorageClass(ctx, client, contentType)
	}

	// look up storage class by name
	storageClass := &storagev1.StorageClass{}
	if err := client.Get(ctx, types.NamespacedName{Name: *name}, storageClass); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		klog.V(3).Info("Unable to retrieve storage class", "storage class name", *name)
		return nil, errors.Errorf("unable to retrieve storage class %s", *name)
	}

	return storageClass, nil
}

// GetStorageClassByNameWithK8sFallback looks up the storage class based on the name
// If name is nil, it looks for the default k8s storage class storageclass.kubernetes.io/is-default-class
// If no storage class is found, returns nil
func GetStorageClassByNameWithK8sFallback(ctx context.Context, client client.Client, name *string) (*storagev1.StorageClass, error) {
	return getStorageClassByName(ctx, client, name, cdiv1.DataVolumeArchive)
}

// GetStorageClassByNameWithVirtFallback looks up the storage class based on the name
// If name is nil, it looks for the following, in this order:
// default kubevirt storage class (if the caller is interested) storageclass.kubevirt.io/is-default-class
// default k8s storage class storageclass.kubernetes.io/is-default-class
// If no storage class is found, returns nil
func GetStorageClassByNameWithVirtFallback(ctx context.Context, client client.Client, name *string, contentType cdiv1.DataVolumeContentType) (*storagev1.StorageClass, error) {
	return getStorageClassByName(ctx, client, name, contentType)
}

// getFallbackStorageClass looks for a default virt/k8s storage class according to the content type
// If no storage class is found, returns nil
func getFallbackStorageClass(ctx context.Context, client client.Client, contentType cdiv1.DataVolumeContentType) (*storagev1.StorageClass, error) {
	storageClasses := &storagev1.StorageClassList{}
	if err := client.List(ctx, storageClasses); err != nil {
		klog.V(3).Info("Unable to retrieve available storage classes")
		return nil, errors.New("unable to retrieve storage classes")
	}

	if GetContentType(contentType) == cdiv1.DataVolumeKubeVirt {
		if virtSc := GetPlatformDefaultStorageClass(storageClasses, AnnDefaultVirtStorageClass); virtSc != nil {
			return virtSc, nil
		}
	}
	return GetPlatformDefaultStorageClass(storageClasses, AnnDefaultStorageClass), nil
}

// GetPlatformDefaultStorageClass returns the default storage class according to the provided annotation or nil if none found
func GetPlatformDefaultStorageClass(storageClasses *storagev1.StorageClassList, defaultAnnotationKey string) *storagev1.StorageClass {
	defaultClasses := []storagev1.StorageClass{}

	for _, storageClass := range storageClasses.Items {
		if storageClass.Annotations[defaultAnnotationKey] == "true" {
			defaultClasses = append(defaultClasses, storageClass)
		}
	}

	if len(defaultClasses) == 0 {
		return nil
	}

	// Primary sort by creation timestamp, newest first
	// Secondary sort by class name, ascending order
	// Follows k8s behavior
	// https://github.com/kubernetes/kubernetes/blob/731068288e112c8b5af70f676296cc44661e84f4/pkg/volume/util/storageclass.go#L58-L59
	sort.Slice(defaultClasses, func(i, j int) bool {
		if defaultClasses[i].CreationTimestamp.UnixNano() == defaultClasses[j].CreationTimestamp.UnixNano() {
			return defaultClasses[i].Name < defaultClasses[j].Name
		}
		return defaultClasses[i].CreationTimestamp.UnixNano() > defaultClasses[j].CreationTimestamp.UnixNano()
	})
	if len(defaultClasses) > 1 {
		klog.V(3).Infof("%d default StorageClasses were found, choosing: %s", len(defaultClasses), defaultClasses[0].Name)
	}

	return &defaultClasses[0]
}

// GetFilesystemOverheadForStorageClass determines the filesystem overhead defined in CDIConfig for the storageClass.
func GetFilesystemOverheadForStorageClass(ctx context.Context, client client.Client, storageClassName *string) (cdiv1.Percent, error) {
	if storageClassName != nil && *storageClassName == "" {
		klog.V(3).Info("No storage class name passed")
		return "0", nil
	}

	cdiConfig := &cdiv1.CDIConfig{}
	if err := client.Get(ctx, types.NamespacedName{Name: common.ConfigName}, cdiConfig); err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(1).Info("CDIConfig does not exist, pod will not start until it does")
			return "0", nil
		}
		return "0", err
	}

	targetStorageClass, err := GetStorageClassByNameWithK8sFallback(ctx, client, storageClassName)
	if err != nil || targetStorageClass == nil {
		klog.V(3).Info("Storage class", storageClassName, "not found, trying default storage class")
		targetStorageClass, err = GetStorageClassByNameWithK8sFallback(ctx, client, nil)
		if err != nil {
			klog.V(3).Info("No default storage class found, continuing with global overhead")
			return cdiConfig.Status.FilesystemOverhead.Global, nil
		}
	}

	if cdiConfig.Status.FilesystemOverhead == nil {
		klog.Errorf("CDIConfig filesystemOverhead used before config controller ran reconcile. Hopefully this only happens during unit testing.")
		return "0", nil
	}

	if targetStorageClass == nil {
		klog.V(3).Info("Storage class", storageClassName, "not found, continuing with global overhead")
		return cdiConfig.Status.FilesystemOverhead.Global, nil
	}

	klog.V(3).Info("target storage class for overhead", targetStorageClass.GetName())

	perStorageConfig := cdiConfig.Status.FilesystemOverhead.StorageClass

	storageClassOverhead, found := perStorageConfig[targetStorageClass.GetName()]
	if found {
		return storageClassOverhead, nil
	}

	return cdiConfig.Status.FilesystemOverhead.Global, nil
}

// GetDefaultPodResourceRequirements gets default pod resource requirements from cdi config status
func GetDefaultPodResourceRequirements(client client.Client) (*corev1.ResourceRequirements, error) {
	cdiconfig := &cdiv1.CDIConfig{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: common.ConfigName}, cdiconfig); err != nil {
		klog.Errorf("Unable to find CDI configuration, %v\n", err)
		return nil, err
	}

	return cdiconfig.Status.DefaultPodResourceRequirements, nil
}

// GetImagePullSecrets gets the imagePullSecrets needed to pull images from the cdi config
func GetImagePullSecrets(client client.Client) ([]corev1.LocalObjectReference, error) {
	cdiconfig := &cdiv1.CDIConfig{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: common.ConfigName}, cdiconfig); err != nil {
		klog.Errorf("Unable to find CDI configuration, %v\n", err)
		return nil, err
	}

	return cdiconfig.Status.ImagePullSecrets, nil
}

// GetPodFromPvc determines the pod associated with the pvc passed in.
func GetPodFromPvc(c client.Client, namespace string, pvc *corev1.PersistentVolumeClaim) (*corev1.Pod, error) {
	l, _ := labels.Parse(common.PrometheusLabelKey)
	pods := &corev1.PodList{}
	listOptions := client.ListOptions{
		LabelSelector: l,
	}
	if err := c.List(context.TODO(), pods, &listOptions); err != nil {
		return nil, err
	}

	pvcUID := pvc.GetUID()
	for _, pod := range pods.Items {
		if ShouldIgnorePod(&pod, pvc) {
			continue
		}
		for _, or := range pod.OwnerReferences {
			if or.UID == pvcUID {
				return &pod, nil
			}
		}

		// TODO: check this
		val, exists := pod.Labels[CloneUniqueID]
		if exists && val == string(pvcUID)+common.ClonerSourcePodNameSuffix {
			return &pod, nil
		}
	}
	return nil, errors.Errorf("Unable to find pod owned by UID: %s, in namespace: %s", string(pvcUID), namespace)
}

// AddVolumeDevices returns VolumeDevice slice with one block device for pods using PV with block volume mode
func AddVolumeDevices() []corev1.VolumeDevice {
	volumeDevices := []corev1.VolumeDevice{
		{
			Name:       DataVolName,
			DevicePath: common.WriteBlockPath,
		},
	}
	return volumeDevices
}

// GetPodsUsingPVCs returns Pods currently using PVCs
func GetPodsUsingPVCs(ctx context.Context, c client.Client, namespace string, names sets.Set[string], allowReadOnly bool) ([]corev1.Pod, error) {
	pl := &corev1.PodList{}
	// hopefully using cached client here
	err := c.List(ctx, pl, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return nil, err
	}

	var pods []corev1.Pod
	for _, pod := range pl.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.VolumeSource.PersistentVolumeClaim != nil &&
				names.Has(volume.PersistentVolumeClaim.ClaimName) {
				addPod := true
				if allowReadOnly {
					if !volume.VolumeSource.PersistentVolumeClaim.ReadOnly {
						onlyReadOnly := true
						for _, c := range pod.Spec.Containers {
							for _, vm := range c.VolumeMounts {
								if vm.Name == volume.Name && !vm.ReadOnly {
									onlyReadOnly = false
								}
							}
							for _, vm := range c.VolumeDevices {
								if vm.Name == volume.Name {
									// Node level rw mount and container can't mount block device ro
									onlyReadOnly = false
								}
							}
						}
						if onlyReadOnly {
							// no rw mounts
							addPod = false
						}
					} else {
						// all mounts must be ro
						addPod = false
					}
					if strings.HasSuffix(pod.Name, common.ClonerSourcePodNameSuffix) && pod.Labels != nil &&
						pod.Labels[common.CDIComponentLabel] == common.ClonerSourcePodName {
						// Host assisted clone source pod only reads from source
						// But some drivers disallow mounting a block PVC ReadOnly
						addPod = false
					}
				}
				if addPod {
					pods = append(pods, pod)
					break
				}
			}
		}
	}

	return pods, nil
}

// GetWorkloadNodePlacement extracts the workload-specific nodeplacement values from the CDI CR
func GetWorkloadNodePlacement(ctx context.Context, c client.Client) (*sdkapi.NodePlacement, error) {
	cr, err := GetActiveCDI(ctx, c)
	if err != nil {
		return nil, err
	}

	if cr == nil {
		return nil, fmt.Errorf("no active CDI")
	}

	return &cr.Spec.Workloads, nil
}

// GetActiveCDI returns the active CDI CR
func GetActiveCDI(ctx context.Context, c client.Client) (*cdiv1.CDI, error) {
	crList := &cdiv1.CDIList{}
	if err := c.List(ctx, crList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	if len(crList.Items) == 0 {
		return nil, nil
	}

	if len(crList.Items) == 1 {
		return &crList.Items[0], nil
	}

	var activeResources []cdiv1.CDI
	for _, cr := range crList.Items {
		if cr.Status.Phase != sdkapi.PhaseError {
			activeResources = append(activeResources, cr)
		}
	}

	if len(activeResources) != 1 {
		return nil, fmt.Errorf("invalid number of active CDI resources: %d", len(activeResources))
	}

	return &activeResources[0], nil
}

// IsPopulated returns if the passed in PVC has been populated according to the rules outlined in pkg/apis/core/<version>/utils.go
func IsPopulated(pvc *corev1.PersistentVolumeClaim, c client.Client) (bool, error) {
	return cdiv1utils.IsPopulated(pvc, func(name, namespace string) (*cdiv1.DataVolume, error) {
		dv := &cdiv1.DataVolume{}
		err := c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, dv)
		return dv, err
	})
}

// GetPreallocation returns the preallocation setting for the specified object (DV or VolumeImportSource), falling back to StorageClass and global setting (in this order)
func GetPreallocation(ctx context.Context, client client.Client, preallocation *bool) bool {
	// First, the DV's preallocation
	if preallocation != nil {
		return *preallocation
	}

	cdiconfig := &cdiv1.CDIConfig{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: common.ConfigName}, cdiconfig); err != nil {
		klog.Errorf("Unable to find CDI configuration, %v\n", err)
		return defaultPreallocation
	}

	return cdiconfig.Status.Preallocation
}

// ImmediateBindingRequested returns if an object has the ImmediateBinding annotation
func ImmediateBindingRequested(obj metav1.Object) bool {
	_, isImmediateBindingRequested := obj.GetAnnotations()[AnnImmediateBinding]
	return isImmediateBindingRequested
}

// GetPriorityClass gets PVC priority class
func GetPriorityClass(pvc *corev1.PersistentVolumeClaim) string {
	anno := pvc.GetAnnotations()
	return anno[AnnPriorityClassName]
}

// ShouldDeletePod returns whether the PVC workload pod should be deleted
func ShouldDeletePod(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc.GetAnnotations()[AnnPodRetainAfterCompletion] != "true" || pvc.GetAnnotations()[AnnRequiresScratch] == "true" || pvc.GetAnnotations()[AnnRequiresDirectIO] == "true" || pvc.DeletionTimestamp != nil
}

// AddFinalizer adds a finalizer to a resource
func AddFinalizer(obj metav1.Object, name string) {
	if HasFinalizer(obj, name) {
		return
	}

	obj.SetFinalizers(append(obj.GetFinalizers(), name))
}

// RemoveFinalizer removes a finalizer from a resource
func RemoveFinalizer(obj metav1.Object, name string) {
	if !HasFinalizer(obj, name) {
		return
	}

	var finalizers []string
	for _, f := range obj.GetFinalizers() {
		if f != name {
			finalizers = append(finalizers, f)
		}
	}

	obj.SetFinalizers(finalizers)
}

// HasFinalizer returns true if a resource has a specific finalizer
func HasFinalizer(object metav1.Object, value string) bool {
	for _, f := range object.GetFinalizers() {
		if f == value {
			return true
		}
	}
	return false
}

// ValidateCloneTokenPVC validates clone token for source and target PVCs
func ValidateCloneTokenPVC(t string, v token.Validator, source, target *corev1.PersistentVolumeClaim) error {
	if source.Namespace == target.Namespace {
		return nil
	}

	tokenData, err := v.Validate(t)
	if err != nil {
		return errors.Wrap(err, "error verifying token")
	}

	tokenResourceName := getTokenResourceNamePvc(source)
	srcName := getSourceNamePvc(source)

	return validateTokenData(tokenData, source.Namespace, srcName, target.Namespace, target.Name, string(target.UID), tokenResourceName)
}

// ValidateCloneTokenDV validates clone token for DV
func ValidateCloneTokenDV(validator token.Validator, dv *cdiv1.DataVolume) error {
	_, sourceName, sourceNamespace := GetCloneSourceInfo(dv)
	if sourceNamespace == "" || sourceNamespace == dv.Namespace {
		return nil
	}

	tok, ok := dv.Annotations[AnnCloneToken]
	if !ok {
		return errors.New("clone token missing")
	}

	tokenData, err := validator.Validate(tok)
	if err != nil {
		return errors.Wrap(err, "error verifying token")
	}

	tokenResourceName := getTokenResourceNameDataVolume(dv.Spec.Source)
	if tokenResourceName == "" {
		return errors.New("token resource name empty, can't verify properly")
	}

	return validateTokenData(tokenData, sourceNamespace, sourceName, dv.Namespace, dv.Name, "", tokenResourceName)
}

func getTokenResourceNameDataVolume(source *cdiv1.DataVolumeSource) string {
	if source.PVC != nil {
		return "persistentvolumeclaims"
	} else if source.Snapshot != nil {
		return "volumesnapshots"
	}

	return ""
}

func getTokenResourceNamePvc(sourcePvc *corev1.PersistentVolumeClaim) string {
	if v, ok := sourcePvc.Labels[common.CDIComponentLabel]; ok && v == common.CloneFromSnapshotFallbackPVCCDILabel {
		return "volumesnapshots"
	}

	return "persistentvolumeclaims"
}

func getSourceNamePvc(sourcePvc *corev1.PersistentVolumeClaim) string {
	if v, ok := sourcePvc.Labels[common.CDIComponentLabel]; ok && v == common.CloneFromSnapshotFallbackPVCCDILabel {
		if sourcePvc.Spec.DataSourceRef != nil {
			return sourcePvc.Spec.DataSourceRef.Name
		}
	}

	return sourcePvc.Name
}

func validateTokenData(tokenData *token.Payload, srcNamespace, srcName, targetNamespace, targetName, targetUID, tokenResourceName string) error {
	uid := tokenData.Params["uid"]
	if tokenData.Operation != token.OperationClone ||
		tokenData.Name != srcName ||
		tokenData.Namespace != srcNamespace ||
		tokenData.Resource.Resource != tokenResourceName ||
		tokenData.Params["targetNamespace"] != targetNamespace ||
		tokenData.Params["targetName"] != targetName ||
		(uid != "" && uid != targetUID) {
		return errors.New("invalid token")
	}

	return nil
}

// IsSnapshotValidForClone returns an error if the passed snapshot is not valid for cloning
func IsSnapshotValidForClone(sourceSnapshot *snapshotv1.VolumeSnapshot) error {
	if sourceSnapshot.Status == nil {
		return fmt.Errorf("no status on source snapshot yet")
	}
	if !IsSnapshotReady(sourceSnapshot) {
		klog.V(3).Info("snapshot not ReadyToUse, while we allow this, probably going to be an issue going forward", "namespace", sourceSnapshot.Namespace, "name", sourceSnapshot.Name)
	}
	if sourceSnapshot.Status.Error != nil {
		errMessage := "no details"
		if msg := sourceSnapshot.Status.Error.Message; msg != nil {
			errMessage = *msg
		}
		return fmt.Errorf("snapshot in error state with msg: %s", errMessage)
	}
	if sourceSnapshot.Spec.VolumeSnapshotClassName == nil ||
		*sourceSnapshot.Spec.VolumeSnapshotClassName == "" {
		return fmt.Errorf("snapshot %s/%s does not have volume snap class populated, can't clone", sourceSnapshot.Name, sourceSnapshot.Namespace)
	}
	return nil
}

// AddAnnotation adds an annotation to an object
func AddAnnotation(obj metav1.Object, key, value string) {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(make(map[string]string))
	}
	obj.GetAnnotations()[key] = value
}

// AddLabel adds a label to an object
func AddLabel(obj metav1.Object, key, value string) {
	if obj.GetLabels() == nil {
		obj.SetLabels(make(map[string]string))
	}
	obj.GetLabels()[key] = value
}

// HandleFailedPod handles pod-creation errors and updates the pod's PVC without providing sensitive information
func HandleFailedPod(err error, podName string, pvc *corev1.PersistentVolumeClaim, recorder record.EventRecorder, c client.Client) error {
	if err == nil {
		return nil
	}
	// Generic reason and msg to avoid providing sensitive information
	reason := ErrStartingPod
	msg := fmt.Sprintf(MessageErrStartingPod, podName)

	// Error handling to fine-tune the event with pertinent info
	if ErrQuotaExceeded(err) {
		reason = ErrExceededQuota
	}

	recorder.Event(pvc, corev1.EventTypeWarning, reason, msg)

	if isCloneSourcePod := CreateCloneSourcePodName(pvc) == podName; isCloneSourcePod {
		AddAnnotation(pvc, AnnSourceRunningCondition, "false")
		AddAnnotation(pvc, AnnSourceRunningConditionReason, reason)
		AddAnnotation(pvc, AnnSourceRunningConditionMessage, msg)
	} else {
		AddAnnotation(pvc, AnnRunningCondition, "false")
		AddAnnotation(pvc, AnnRunningConditionReason, reason)
		AddAnnotation(pvc, AnnRunningConditionMessage, msg)
	}

	AddAnnotation(pvc, AnnPodPhase, string(corev1.PodFailed))
	if err := c.Update(context.TODO(), pvc); err != nil {
		return err
	}

	return err
}

// GetSource returns the source string which determines the type of source. If no source or invalid source found, default to http
func GetSource(pvc *corev1.PersistentVolumeClaim) string {
	source, found := pvc.Annotations[AnnSource]
	if !found {
		source = ""
	}
	switch source {
	case
		SourceHTTP,
		SourceS3,
		SourceGCS,
		SourceGlance,
		SourceNone,
		SourceRegistry,
		SourceImageio,
		SourceVDDK:
	default:
		source = SourceHTTP
	}
	return source
}

// GetEndpoint returns the endpoint string which contains the full path URI of the target object to be copied.
func GetEndpoint(pvc *corev1.PersistentVolumeClaim) (string, error) {
	ep, found := pvc.Annotations[AnnEndpoint]
	if !found || ep == "" {
		verb := "empty"
		if !found {
			verb = "missing"
		}
		return ep, errors.Errorf("annotation %q in pvc \"%s/%s\" is %s", AnnEndpoint, pvc.Namespace, pvc.Name, verb)
	}
	return ep, nil
}

// AddImportVolumeMounts is being called for pods using PV with filesystem volume mode
func AddImportVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      DataVolName,
			MountPath: common.ImporterDataDir,
		},
	}
	return volumeMounts
}

// ValidateRequestedCloneSize validates the clone size requirements on block
func ValidateRequestedCloneSize(sourceResources, targetResources corev1.VolumeResourceRequirements) error {
	sourceRequest, hasSource := sourceResources.Requests[corev1.ResourceStorage]
	targetRequest, hasTarget := targetResources.Requests[corev1.ResourceStorage]
	if !hasSource || !hasTarget {
		return errors.New("source/target missing storage resource requests")
	}

	// Verify that the target PVC size is equal or larger than the source.
	if sourceRequest.Value() > targetRequest.Value() {
		return errors.Errorf("target resources requests storage size is smaller than the source %d < %d", targetRequest.Value(), sourceRequest.Value())
	}
	return nil
}

// CreateCloneSourcePodName creates clone source pod name
func CreateCloneSourcePodName(targetPvc *corev1.PersistentVolumeClaim) string {
	return string(targetPvc.GetUID()) + common.ClonerSourcePodNameSuffix
}

// IsPVCComplete returns true if a PVC is in 'Succeeded' phase, false if not
func IsPVCComplete(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc != nil {
		phase, exists := pvc.ObjectMeta.Annotations[AnnPodPhase]
		return exists && (phase == string(corev1.PodSucceeded))
	}
	return false
}

// IsMultiStageImportInProgress returns true when a PVC is being part of an ongoing multi-stage import
func IsMultiStageImportInProgress(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc != nil {
		multiStageImport := metav1.HasAnnotation(pvc.ObjectMeta, AnnCurrentCheckpoint)
		multiStageAlreadyDone := metav1.HasAnnotation(pvc.ObjectMeta, AnnMultiStageImportDone)
		return multiStageImport && !multiStageAlreadyDone
	}
	return false
}

// SetRestrictedSecurityContext sets the pod security params to be compatible with restricted PSA
func SetRestrictedSecurityContext(podSpec *corev1.PodSpec) {
	hasVolumeMounts := false
	for _, containers := range [][]corev1.Container{podSpec.InitContainers, podSpec.Containers} {
		for i := range containers {
			container := &containers[i]
			if container.SecurityContext == nil {
				container.SecurityContext = &corev1.SecurityContext{}
			}
			container.SecurityContext.Capabilities = &corev1.Capabilities{
				Drop: []corev1.Capability{
					"ALL",
				},
			}
			container.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			}
			container.SecurityContext.AllowPrivilegeEscalation = ptr.To[bool](false)
			container.SecurityContext.RunAsNonRoot = ptr.To[bool](true)
			container.SecurityContext.RunAsUser = ptr.To[int64](common.QemuSubGid)
			if len(container.VolumeMounts) > 0 {
				hasVolumeMounts = true
			}
		}
	}

	if hasVolumeMounts {
		if podSpec.SecurityContext == nil {
			podSpec.SecurityContext = &corev1.PodSecurityContext{}
		}
		podSpec.SecurityContext.FSGroup = ptr.To[int64](common.QemuSubGid)
	}
}

// SetNodeNameIfPopulator sets NodeName in a pod spec when the PVC is being handled by a CDI volume populator
func SetNodeNameIfPopulator(pvc *corev1.PersistentVolumeClaim, podSpec *corev1.PodSpec) {
	_, isPopulator := pvc.Annotations[AnnPopulatorKind]
	nodeName := pvc.Annotations[AnnSelectedNode]
	if isPopulator && nodeName != "" {
		podSpec.NodeName = nodeName
	}
}

// CreatePvc creates PVC
func CreatePvc(name, ns string, annotations, labels map[string]string) *corev1.PersistentVolumeClaim {
	return CreatePvcInStorageClass(name, ns, nil, annotations, labels, corev1.ClaimBound)
}

// CreatePvcInStorageClass creates PVC with storgae class
func CreatePvcInStorageClass(name, ns string, storageClassName *string, annotations, labels map[string]string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: annotations,
			Labels:      labels,
			UID:         types.UID(ns + "-" + name),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany, corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1G"),
				},
			},
			StorageClassName: storageClassName,
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}
	pvc.Status.Capacity = pvc.Spec.Resources.Requests.DeepCopy()
	if pvc.Status.Phase == corev1.ClaimBound {
		pvc.Spec.VolumeName = "pv-" + string(pvc.UID)
	}
	return pvc
}

// GetAPIServerKey returns API server RSA key
func GetAPIServerKey() *rsa.PrivateKey {
	apiServerKeyOnce.Do(func() {
		apiServerKey, _ = rsa.GenerateKey(rand.Reader, 2048)
	})
	return apiServerKey
}

// CreateStorageClass creates storage class CR
func CreateStorageClass(name string, annotations map[string]string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
	}
}

// CreateImporterTestPod creates importer test pod CR
func CreateImporterTestPod(pvc *corev1.PersistentVolumeClaim, dvname string, scratchPvc *corev1.PersistentVolumeClaim) *corev1.Pod {
	// importer pod name contains the pvc name
	podName := fmt.Sprintf("%s-%s", common.ImporterPodName, pvc.Name)

	blockOwnerDeletion := true
	isController := true

	volumes := []corev1.Volume{
		{
			Name: dvname,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.Name,
					ReadOnly:  false,
				},
			},
		},
	}

	if scratchPvc != nil {
		volumes = append(volumes, corev1.Volume{
			Name: ScratchVolName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: scratchPvc.Name,
					ReadOnly:  false,
				},
			},
		})
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: pvc.Namespace,
			Annotations: map[string]string{
				AnnCreatedBy: "yes",
			},
			Labels: map[string]string{
				common.CDILabelKey:        common.CDILabelValue,
				common.CDIComponentLabel:  common.ImporterPodName,
				common.PrometheusLabelKey: common.PrometheusLabelValue,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "v1",
					Kind:               "PersistentVolumeClaim",
					Name:               pvc.Name,
					UID:                pvc.GetUID(),
					BlockOwnerDeletion: &blockOwnerDeletion,
					Controller:         &isController,
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            common.ImporterPodName,
					Image:           "test/myimage",
					ImagePullPolicy: corev1.PullPolicy("Always"),
					Args:            []string{"-v=5"},
					Ports: []corev1.ContainerPort{
						{
							Name:          "metrics",
							ContainerPort: 8443,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Volumes:       volumes,
		},
	}

	ep, _ := GetEndpoint(pvc)
	source := GetSource(pvc)
	contentType := GetPVCContentType(pvc)
	imageSize, _ := GetRequestedImageSize(pvc)
	volumeMode := GetVolumeMode(pvc)

	env := []corev1.EnvVar{
		{
			Name:  common.ImporterSource,
			Value: source,
		},
		{
			Name:  common.ImporterEndpoint,
			Value: ep,
		},
		{
			Name:  common.ImporterContentType,
			Value: string(contentType),
		},
		{
			Name:  common.ImporterImageSize,
			Value: imageSize,
		},
		{
			Name:  common.OwnerUID,
			Value: string(pvc.UID),
		},
		{
			Name:  common.InsecureTLSVar,
			Value: "false",
		},
	}
	pod.Spec.Containers[0].Env = env
	if volumeMode == corev1.PersistentVolumeBlock {
		pod.Spec.Containers[0].VolumeDevices = AddVolumeDevices()
	} else {
		pod.Spec.Containers[0].VolumeMounts = AddImportVolumeMounts()
	}

	if scratchPvc != nil {
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      ScratchVolName,
			MountPath: common.ScratchDataDir,
		})
	}

	return pod
}

// CreateStorageClassWithProvisioner creates CR of storage class with provisioner
func CreateStorageClassWithProvisioner(name string, annotations, labels map[string]string, provisioner string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		Provisioner: provisioner,
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
			Labels:      labels,
		},
	}
}

// CreateClient creates a fake client
func CreateClient(objs ...runtime.Object) client.Client {
	s := scheme.Scheme
	_ = cdiv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = storagev1.AddToScheme(s)
	_ = ocpconfigv1.Install(s)

	return fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()
}

// ErrQuotaExceeded checked is the error is of exceeded quota
func ErrQuotaExceeded(err error) bool {
	return strings.Contains(err.Error(), "exceeded quota:")
}

// GetContentType returns the content type. If invalid or not set, default to kubevirt
func GetContentType(contentType cdiv1.DataVolumeContentType) cdiv1.DataVolumeContentType {
	switch contentType {
	case
		cdiv1.DataVolumeKubeVirt,
		cdiv1.DataVolumeArchive:
	default:
		// TODO - shouldn't archive be the default?
		contentType = cdiv1.DataVolumeKubeVirt
	}
	return contentType
}

// GetPVCContentType returns the content type of the source image. If invalid or not set, default to kubevirt
func GetPVCContentType(pvc *corev1.PersistentVolumeClaim) cdiv1.DataVolumeContentType {
	contentType, found := pvc.Annotations[AnnContentType]
	if !found {
		// TODO - shouldn't archive be the default?
		return cdiv1.DataVolumeKubeVirt
	}

	return GetContentType(cdiv1.DataVolumeContentType(contentType))
}

// GetNamespace returns the given namespace if not empty, otherwise the default namespace
func GetNamespace(namespace, defaultNamespace string) string {
	if namespace == "" {
		return defaultNamespace
	}
	return namespace
}

// IsErrCacheNotStarted checked is the error is of cache not started
func IsErrCacheNotStarted(err error) bool {
	target := &runtimecache.ErrCacheNotStarted{}
	return errors.As(err, &target)
}

// GetDataVolumeTTLSeconds gets the current DataVolume TTL in seconds if GC is enabled, or < 0 if GC is disabled
// Garbage collection is disabled by default
func GetDataVolumeTTLSeconds(config *cdiv1.CDIConfig) int32 {
	const defaultDataVolumeTTLSeconds = -1
	if config.Spec.DataVolumeTTLSeconds != nil {
		return *config.Spec.DataVolumeTTLSeconds
	}
	return defaultDataVolumeTTLSeconds
}

// NewImportDataVolume returns new import DataVolume CR
func NewImportDataVolume(name string) *cdiv1.DataVolume {
	return &cdiv1.DataVolume{
		TypeMeta: metav1.TypeMeta{APIVersion: cdiv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
			UID:       types.UID(metav1.NamespaceDefault + "-" + name),
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{
				HTTP: &cdiv1.DataVolumeSourceHTTP{
					URL: "http://example.com/data",
				},
			},
			PVC: &corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			PriorityClassName: "p0",
		},
	}
}

// GetCloneSourceInfo returns the type, name and namespace of the cloning source
func GetCloneSourceInfo(dv *cdiv1.DataVolume) (sourceType, sourceName, sourceNamespace string) {
	// Cloning sources are mutually exclusive
	if dv.Spec.Source.PVC != nil {
		return "pvc", dv.Spec.Source.PVC.Name, dv.Spec.Source.PVC.Namespace
	}

	if dv.Spec.Source.Snapshot != nil {
		return "snapshot", dv.Spec.Source.Snapshot.Name, dv.Spec.Source.Snapshot.Namespace
	}

	return "", "", ""
}

// IsWaitForFirstConsumerEnabled tells us if we should respect "real" WFFC behavior or just let our worker pods randomly spawn
func IsWaitForFirstConsumerEnabled(obj metav1.Object, gates featuregates.FeatureGates) (bool, error) {
	// when PVC requests immediateBinding it cannot honor wffc logic
	isImmediateBindingRequested := ImmediateBindingRequested(obj)
	pvcHonorWaitForFirstConsumer := !isImmediateBindingRequested
	globalHonorWaitForFirstConsumer, err := gates.HonorWaitForFirstConsumerEnabled()
	if err != nil {
		return false, err
	}

	return pvcHonorWaitForFirstConsumer && globalHonorWaitForFirstConsumer, nil
}

// AddImmediateBindingAnnotationIfWFFCDisabled adds the immediateBinding annotation if wffc feature gate is disabled
func AddImmediateBindingAnnotationIfWFFCDisabled(obj metav1.Object, gates featuregates.FeatureGates) error {
	globalHonorWaitForFirstConsumer, err := gates.HonorWaitForFirstConsumerEnabled()
	if err != nil {
		return err
	}
	if !globalHonorWaitForFirstConsumer {
		AddAnnotation(obj, AnnImmediateBinding, "")
	}
	return nil
}

// GetRequiredSpace calculates space required taking file system overhead into account
func GetRequiredSpace(filesystemOverhead float64, requestedSpace int64) int64 {
	// the `image` has to be aligned correctly, so the space requested has to be aligned to
	// next value that is a multiple of a block size
	alignedSize := util.RoundUp(requestedSpace, util.DefaultAlignBlockSize)

	// count overhead as a percentage of the whole/new size, including aligned image
	// and the space required by filesystem metadata
	spaceWithOverhead := int64(math.Ceil(float64(alignedSize) / (1 - filesystemOverhead)))
	return spaceWithOverhead
}

// InflateSizeWithOverhead inflates a storage size with proper overhead calculations
func InflateSizeWithOverhead(ctx context.Context, c client.Client, imgSize int64, pvcSpec *corev1.PersistentVolumeClaimSpec) (resource.Quantity, error) {
	var returnSize resource.Quantity

	if util.ResolveVolumeMode(pvcSpec.VolumeMode) == corev1.PersistentVolumeFilesystem {
		fsOverhead, err := GetFilesystemOverheadForStorageClass(ctx, c, pvcSpec.StorageClassName)
		if err != nil {
			return resource.Quantity{}, err
		}
		// Parse filesystem overhead (percentage) into a 64-bit float
		fsOverheadFloat, _ := strconv.ParseFloat(string(fsOverhead), 64)

		// Merge the previous values into a 'resource.Quantity' struct
		requiredSpace := GetRequiredSpace(fsOverheadFloat, imgSize)
		returnSize = *resource.NewScaledQuantity(requiredSpace, 0)
	} else {
		// Inflation is not needed with 'Block' mode
		returnSize = *resource.NewScaledQuantity(imgSize, 0)
	}

	return returnSize, nil
}

// IsBound returns if the pvc is bound
func IsBound(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc != nil && pvc.Status.Phase == corev1.ClaimBound
}

// IsUnbound returns if the pvc is not bound yet
func IsUnbound(pvc *corev1.PersistentVolumeClaim) bool {
	return !IsBound(pvc)
}

// IsLost returns if the pvc is lost
func IsLost(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc != nil && pvc.Status.Phase == corev1.ClaimLost
}

// IsImageStream returns true if registry source is ImageStream
func IsImageStream(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc.Annotations[AnnRegistryImageStream] == "true"
}

// ShouldIgnorePod checks if a pod should be ignored.
// If this is a completed pod that was used for one checkpoint of a multi-stage import, it
// should be ignored by pod lookups as long as the retainAfterCompletion annotation is set.
func ShouldIgnorePod(pod *corev1.Pod, pvc *corev1.PersistentVolumeClaim) bool {
	retain := pvc.ObjectMeta.Annotations[AnnPodRetainAfterCompletion]
	checkpoint := pvc.ObjectMeta.Annotations[AnnCurrentCheckpoint]
	if checkpoint != "" && pod.Status.Phase == corev1.PodSucceeded {
		return retain == "true"
	}
	return false
}

// BuildHTTPClient generates an http client that accepts any certificate, since we are using
// it to get prometheus data it doesn't matter if someone can intercept the data. Once we have
// a mechanism to properly sign the server, we can update this method to get a proper client.
func BuildHTTPClient(httpClient *http.Client) *http.Client {
	if httpClient == nil {
		defaultTransport := http.DefaultTransport.(*http.Transport)
		// Create new Transport that ignores self-signed SSL
		//nolint:gosec
		tr := &http.Transport{
			Proxy:                 defaultTransport.Proxy,
			DialContext:           defaultTransport.DialContext,
			MaxIdleConns:          defaultTransport.MaxIdleConns,
			IdleConnTimeout:       defaultTransport.IdleConnTimeout,
			ExpectContinueTimeout: defaultTransport.ExpectContinueTimeout,
			TLSHandshakeTimeout:   defaultTransport.TLSHandshakeTimeout,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{
			Transport: tr,
		}
	}
	return httpClient
}

// ErrConnectionRefused checks for connection refused errors
func ErrConnectionRefused(err error) bool {
	return strings.Contains(err.Error(), "connection refused")
}

// GetPodMetricsPort returns, if exists, the metrics port from the passed pod
func GetPodMetricsPort(pod *corev1.Pod) (int, error) {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == "metrics" {
				return int(port.ContainerPort), nil
			}
		}
	}
	return 0, errors.New("Metrics port not found in pod")
}

// GetMetricsURL builds the metrics URL according to the specified pod
func GetMetricsURL(pod *corev1.Pod) (string, error) {
	if pod == nil {
		return "", nil
	}
	port, err := GetPodMetricsPort(pod)
	if err != nil || pod.Status.PodIP == "" {
		return "", err
	}
	domain := net.JoinHostPort(pod.Status.PodIP, fmt.Sprint(port))
	url := fmt.Sprintf("https://%s/metrics", domain)
	return url, nil
}

// GetProgressReportFromURL fetches the progress report from the passed URL according to an specific metric expression and ownerUID
func GetProgressReportFromURL(url string, httpClient *http.Client, metricExp, ownerUID string) (string, error) {
	regExp := regexp.MustCompile(fmt.Sprintf("(%s)\\{ownerUID\\=%q\\} (\\d{1,3}\\.?\\d*)", metricExp, ownerUID))
	resp, err := httpClient.Get(url)
	if err != nil {
		if ErrConnectionRefused(err) {
			return "", nil
		}
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse the progress from the body
	progressReport := ""
	match := regExp.FindStringSubmatch(string(body))
	if match != nil {
		progressReport = match[len(match)-1]
	}
	return progressReport, nil
}

// UpdateHTTPAnnotations updates the passed annotations for proper http import
func UpdateHTTPAnnotations(annotations map[string]string, http *cdiv1.DataVolumeSourceHTTP) {
	annotations[AnnEndpoint] = http.URL
	annotations[AnnSource] = SourceHTTP

	if http.SecretRef != "" {
		annotations[AnnSecret] = http.SecretRef
	}
	if http.CertConfigMap != "" {
		annotations[AnnCertConfigMap] = http.CertConfigMap
	}
	for index, header := range http.ExtraHeaders {
		annotations[fmt.Sprintf("%s.%d", AnnExtraHeaders, index)] = header
	}
	for index, header := range http.SecretExtraHeaders {
		annotations[fmt.Sprintf("%s.%d", AnnSecretExtraHeaders, index)] = header
	}
}

// UpdateS3Annotations updates the passed annotations for proper S3 import
func UpdateS3Annotations(annotations map[string]string, s3 *cdiv1.DataVolumeSourceS3) {
	annotations[AnnEndpoint] = s3.URL
	annotations[AnnSource] = SourceS3
	if s3.SecretRef != "" {
		annotations[AnnSecret] = s3.SecretRef
	}
	if s3.CertConfigMap != "" {
		annotations[AnnCertConfigMap] = s3.CertConfigMap
	}
}

// UpdateGCSAnnotations updates the passed annotations for proper GCS import
func UpdateGCSAnnotations(annotations map[string]string, gcs *cdiv1.DataVolumeSourceGCS) {
	annotations[AnnEndpoint] = gcs.URL
	annotations[AnnSource] = SourceGCS
	if gcs.SecretRef != "" {
		annotations[AnnSecret] = gcs.SecretRef
	}
}

// UpdateRegistryAnnotations updates the passed annotations for proper registry import
func UpdateRegistryAnnotations(annotations map[string]string, registry *cdiv1.DataVolumeSourceRegistry) {
	annotations[AnnSource] = SourceRegistry
	pullMethod := registry.PullMethod
	if pullMethod != nil && *pullMethod != "" {
		annotations[AnnRegistryImportMethod] = string(*pullMethod)
	}
	url := registry.URL
	if url != nil && *url != "" {
		annotations[AnnEndpoint] = *url
	} else {
		imageStream := registry.ImageStream
		if imageStream != nil && *imageStream != "" {
			annotations[AnnEndpoint] = *imageStream
			annotations[AnnRegistryImageStream] = "true"
		}
	}
	secretRef := registry.SecretRef
	if secretRef != nil && *secretRef != "" {
		annotations[AnnSecret] = *secretRef
	}
	certConfigMap := registry.CertConfigMap
	if certConfigMap != nil && *certConfigMap != "" {
		annotations[AnnCertConfigMap] = *certConfigMap
	}
}

// UpdateVDDKAnnotations updates the passed annotations for proper VDDK import
func UpdateVDDKAnnotations(annotations map[string]string, vddk *cdiv1.DataVolumeSourceVDDK) {
	annotations[AnnEndpoint] = vddk.URL
	annotations[AnnSource] = SourceVDDK
	annotations[AnnSecret] = vddk.SecretRef
	annotations[AnnBackingFile] = vddk.BackingFile
	annotations[AnnUUID] = vddk.UUID
	annotations[AnnThumbprint] = vddk.Thumbprint
	if vddk.InitImageURL != "" {
		annotations[AnnVddkInitImageURL] = vddk.InitImageURL
	}
}

// UpdateImageIOAnnotations updates the passed annotations for proper imageIO import
func UpdateImageIOAnnotations(annotations map[string]string, imageio *cdiv1.DataVolumeSourceImageIO) {
	annotations[AnnEndpoint] = imageio.URL
	annotations[AnnSource] = SourceImageio
	annotations[AnnSecret] = imageio.SecretRef
	annotations[AnnCertConfigMap] = imageio.CertConfigMap
	annotations[AnnDiskID] = imageio.DiskID
}

// IsPVBoundToPVC checks if a PV is bound to a specific PVC
func IsPVBoundToPVC(pv *corev1.PersistentVolume, pvc *corev1.PersistentVolumeClaim) bool {
	claimRef := pv.Spec.ClaimRef
	return claimRef != nil && claimRef.Name == pvc.Name && claimRef.Namespace == pvc.Namespace && claimRef.UID == pvc.UID
}

// Rebind binds the PV of source to target
func Rebind(ctx context.Context, c client.Client, source, target *corev1.PersistentVolumeClaim) error {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: source.Spec.VolumeName,
		},
	}

	if err := c.Get(ctx, client.ObjectKeyFromObject(pv), pv); err != nil {
		return err
	}

	// Examine the claimref for the PV and see if it's still bound to PVC'
	if pv.Spec.ClaimRef == nil {
		return fmt.Errorf("PV %s claimRef is nil", pv.Name)
	}

	if !IsPVBoundToPVC(pv, source) {
		// Something is not right if the PV is neither bound to PVC' nor target PVC
		if !IsPVBoundToPVC(pv, target) {
			klog.Errorf("PV bound to unexpected PVC: Could not rebind to target PVC '%s'", target.Name)
			return fmt.Errorf("PV %s bound to unexpected claim %s", pv.Name, pv.Spec.ClaimRef.Name)
		}
		// our work is done
		return nil
	}

	// Rebind PVC to target PVC
	pv.Spec.ClaimRef = &corev1.ObjectReference{
		Namespace:       target.Namespace,
		Name:            target.Name,
		UID:             target.UID,
		ResourceVersion: target.ResourceVersion,
	}
	klog.V(3).Info("Rebinding PV to target PVC", "PVC", target.Name)
	if err := c.Update(context.TODO(), pv); err != nil {
		return err
	}

	return nil
}

// BulkDeleteResources deletes a bunch of resources
func BulkDeleteResources(ctx context.Context, c client.Client, obj client.ObjectList, lo client.ListOption) error {
	if err := c.List(ctx, obj, lo); err != nil {
		if meta.IsNoMatchError(err) {
			return nil
		}
		return err
	}

	sv := reflect.ValueOf(obj).Elem()
	iv := sv.FieldByName("Items")

	for i := 0; i < iv.Len(); i++ {
		obj := iv.Index(i).Addr().Interface().(client.Object)
		if obj.GetDeletionTimestamp().IsZero() {
			klog.V(3).Infof("Deleting type %+v %+v", reflect.TypeOf(obj), obj)
			if err := c.Delete(ctx, obj); err != nil {
				return err
			}
		}
	}

	return nil
}

// ValidateSnapshotCloneSize does proper size validation when doing a clone from snapshot operation
func ValidateSnapshotCloneSize(snapshot *snapshotv1.VolumeSnapshot, pvcSpec *corev1.PersistentVolumeClaimSpec, targetSC *storagev1.StorageClass, log logr.Logger) (bool, error) {
	restoreSize := snapshot.Status.RestoreSize
	if restoreSize == nil {
		return false, fmt.Errorf("snapshot has no RestoreSize")
	}
	targetRequest, hasTargetRequest := pvcSpec.Resources.Requests[corev1.ResourceStorage]
	allowExpansion := targetSC.AllowVolumeExpansion != nil && *targetSC.AllowVolumeExpansion
	if hasTargetRequest {
		// otherwise will just use restoreSize
		if restoreSize.Cmp(targetRequest) < 0 && !allowExpansion {
			log.V(3).Info("Can't expand restored PVC because SC does not allow expansion, need to fall back to host assisted")
			return false, nil
		}
	}
	return true, nil
}

// ValidateSnapshotCloneProvisioners validates the target PVC storage class against the snapshot class provisioner
func ValidateSnapshotCloneProvisioners(ctx context.Context, c client.Client, snapshot *snapshotv1.VolumeSnapshot, storageClass *storagev1.StorageClass) (bool, error) {
	// Do snapshot and storage class validation
	if storageClass == nil {
		return false, fmt.Errorf("target storage class not found")
	}
	if snapshot.Status == nil || snapshot.Status.BoundVolumeSnapshotContentName == nil {
		return false, fmt.Errorf("volumeSnapshotContent name not found")
	}
	volumeSnapshotContent := &snapshotv1.VolumeSnapshotContent{}
	if err := c.Get(ctx, types.NamespacedName{Name: *snapshot.Status.BoundVolumeSnapshotContentName}, volumeSnapshotContent); err != nil {
		return false, err
	}
	if storageClass.Provisioner != volumeSnapshotContent.Spec.Driver {
		return false, nil
	}
	// TODO: get sourceVolumeMode from volumesnapshotcontent and validate against target spec
	// currently don't have CRDs in CI with sourceVolumeMode which is pretty new
	// converting volume mode is possible but has security implications
	return true, nil
}

// GetSnapshotClassForSmartClone looks up the snapshot class based on the storage class
func GetSnapshotClassForSmartClone(pvc *corev1.PersistentVolumeClaim, targetPvcStorageClassName, snapshotClassName *string, log logr.Logger, client client.Client, recorder record.EventRecorder) (string, error) {
	logger := log.WithName("GetSnapshotClassForSmartClone").V(3)
	// Check if relevant CRDs are available
	if !isCsiCrdsDeployed(client, log) {
		logger.Info("Missing CSI snapshotter CRDs, falling back to host assisted clone")
		return "", nil
	}

	targetStorageClass, err := GetStorageClassByNameWithK8sFallback(context.TODO(), client, targetPvcStorageClassName)
	if err != nil {
		return "", err
	}
	if targetStorageClass == nil {
		logger.Info("Target PVC's Storage Class not found")
		return "", nil
	}

	vscName, err := GetVolumeSnapshotClass(context.TODO(), client, pvc, targetStorageClass.Provisioner, snapshotClassName, logger, recorder)
	if err != nil {
		return "", err
	}
	if vscName != nil {
		if pvc != nil {
			logger.Info("smart-clone is applicable for datavolume", "datavolume",
				pvc.Name, "snapshot class", *vscName)
		}
		return *vscName, nil
	}

	logger.Info("Could not match snapshotter with storage class, falling back to host assisted clone")
	return "", nil
}

// GetVolumeSnapshotClass looks up the snapshot class based on the driver and an optional specific name
// In case of multiple candidates, it returns the default-annotated one, or the sorted list first one if no such default
func GetVolumeSnapshotClass(ctx context.Context, c client.Client, pvc *corev1.PersistentVolumeClaim, driver string, snapshotClassName *string, log logr.Logger, recorder record.EventRecorder) (*string, error) {
	logger := log.WithName("GetVolumeSnapshotClass").V(3)

	logEvent := func(message, vscName string) {
		logger.Info(message, "name", vscName)
		if pvc != nil {
			msg := fmt.Sprintf("%s %s", message, vscName)
			recorder.Event(pvc, corev1.EventTypeNormal, VolumeSnapshotClassSelected, msg)
		}
	}

	if snapshotClassName != nil {
		vsc := &snapshotv1.VolumeSnapshotClass{}
		if err := c.Get(context.TODO(), types.NamespacedName{Name: *snapshotClassName}, vsc); err != nil {
			return nil, err
		}
		if vsc.Driver == driver {
			logEvent(MessageStorageProfileVolumeSnapshotClassSelected, vsc.Name)
			return snapshotClassName, nil
		}
		return nil, nil
	}

	vscList := &snapshotv1.VolumeSnapshotClassList{}
	if err := c.List(ctx, vscList); err != nil {
		if meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, err
	}

	var candidates []string
	for _, vsc := range vscList.Items {
		if vsc.Driver == driver {
			if vsc.Annotations[AnnDefaultSnapshotClass] == "true" {
				logEvent(MessageDefaultVolumeSnapshotClassSelected, vsc.Name)
				vscName := vsc.Name
				return &vscName, nil
			}
			candidates = append(candidates, vsc.Name)
		}
	}

	if len(candidates) > 0 {
		sort.Strings(candidates)
		logEvent(MessageFirstVolumeSnapshotClassSelected, candidates[0])
		return &candidates[0], nil
	}

	return nil, nil
}

// isCsiCrdsDeployed checks whether the CSI snapshotter CRD are deployed
func isCsiCrdsDeployed(c client.Client, log logr.Logger) bool {
	version := "v1"
	vsClass := "volumesnapshotclasses." + snapshotv1.GroupName
	vsContent := "volumesnapshotcontents." + snapshotv1.GroupName
	vs := "volumesnapshots." + snapshotv1.GroupName

	return isCrdDeployed(c, vsClass, version, log) &&
		isCrdDeployed(c, vsContent, version, log) &&
		isCrdDeployed(c, vs, version, log)
}

// isCrdDeployed checks whether a CRD is deployed
func isCrdDeployed(c client.Client, name, version string, log logr.Logger) bool {
	crd := &extv1.CustomResourceDefinition{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: name}, crd)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Info("Error looking up CRD", "crd name", name, "version", version, "error", err)
		}
		return false
	}

	for _, v := range crd.Spec.Versions {
		if v.Name == version && v.Served {
			return true
		}
	}

	return false
}

// IsSnapshotReady indicates if a volume snapshot is ready to be used
func IsSnapshotReady(snapshot *snapshotv1.VolumeSnapshot) bool {
	return snapshot.Status != nil && snapshot.Status.ReadyToUse != nil && *snapshot.Status.ReadyToUse
}

// GetResource updates given obj with the data of the object with the same name and namespace
func GetResource(ctx context.Context, c client.Client, namespace, name string, obj client.Object) (bool, error) {
	obj.SetNamespace(namespace)
	obj.SetName(name)

	err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// PatchArgs are the args for Patch
type PatchArgs struct {
	Client client.Client
	Log    logr.Logger
	Obj    client.Object
	OldObj client.Object
}

// GetAnnotatedEventSource returns resource referenced by AnnEventSource annotations
func GetAnnotatedEventSource(ctx context.Context, c client.Client, obj client.Object) (client.Object, error) {
	esk, ok := obj.GetAnnotations()[AnnEventSourceKind]
	if !ok {
		return obj, nil
	}
	if esk != "PersistentVolumeClaim" {
		return obj, nil
	}
	es, ok := obj.GetAnnotations()[AnnEventSource]
	if !ok {
		return obj, nil
	}
	namespace, name, err := cache.SplitMetaNamespaceKey(es)
	if err != nil {
		return nil, err
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(pvc), pvc); err != nil {
		return nil, err
	}
	return pvc, nil
}

// OwnedByDataVolume returns true if the object is owned by a DataVolume
func OwnedByDataVolume(obj metav1.Object) bool {
	owner := metav1.GetControllerOf(obj)
	return owner != nil && owner.Kind == "DataVolume"
}

// CopyAllowedAnnotations copies the allowed annotations from the source object
// to the destination object
func CopyAllowedAnnotations(srcObj, dstObj metav1.Object) {
	for ann, def := range allowedAnnotations {
		val, ok := srcObj.GetAnnotations()[ann]
		if !ok && def != "" {
			val = def
		}
		if val != "" {
			klog.V(1).Info("Applying annotation", "Name", dstObj.GetName(), ann, val)
			AddAnnotation(dstObj, ann, val)
		}
	}
}

// CopyAllowedLabels copies allowed labels matching the validLabelsMatch regexp from the
// source map to the destination object allowing overwrites
func CopyAllowedLabels(srcLabels map[string]string, dstObj metav1.Object, overwrite bool) {
	for label, value := range srcLabels {
		if _, found := dstObj.GetLabels()[label]; (!found || overwrite) && validLabelsMatch.MatchString(label) {
			AddLabel(dstObj, label, value)
		}
	}
}

// ClaimMayExistBeforeDataVolume returns true if the PVC may exist before the DataVolume
func ClaimMayExistBeforeDataVolume(c client.Client, pvc *corev1.PersistentVolumeClaim, dv *cdiv1.DataVolume) (bool, error) {
	if ClaimIsPopulatedForDataVolume(pvc, dv) {
		return true, nil
	}
	return AllowClaimAdoption(c, pvc, dv)
}

// ClaimIsPopulatedForDataVolume returns true if the PVC is populated for the given DataVolume
func ClaimIsPopulatedForDataVolume(pvc *corev1.PersistentVolumeClaim, dv *cdiv1.DataVolume) bool {
	return pvc != nil && dv != nil && pvc.Annotations[AnnPopulatedFor] == dv.Name
}

// AllowClaimAdoption returns true if the PVC may be adopted
func AllowClaimAdoption(c client.Client, pvc *corev1.PersistentVolumeClaim, dv *cdiv1.DataVolume) (bool, error) {
	if pvc == nil || dv == nil {
		return false, nil
	}
	anno, ok := pvc.Annotations[AnnCreatedForDataVolume]
	if ok && anno == string(dv.UID) {
		return false, nil
	}
	anno, ok = dv.Annotations[AnnAllowClaimAdoption]
	// if annotation exists, go with that regardless of featuregate
	if ok {
		val, _ := strconv.ParseBool(anno)
		return val, nil
	}
	return featuregates.NewFeatureGates(c).ClaimAdoptionEnabled()
}
