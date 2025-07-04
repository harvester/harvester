package common

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/rancher/norman/condition"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	uploadFailReason = "UploadFailed"
)

var (
	ErrRetryAble  = errors.New("retryable error")
	ErrRetryLater = errors.New("retry later error")
)

func IsRetryAble(err error) bool {
	return apierrors.IsNotFound(err) || errors.Is(err, ErrRetryAble)
}

func IsRetryLater(err error) bool {
	return errors.Is(err, ErrRetryLater)
}

type VMImageState string

const (
	VMImageStateInitialFail         VMImageState = "initial-fail"
	VMImageStateImportFail          VMImageState = "import-fail"
	VMImageStateInitialized         VMImageState = "initialized"
	VMImageStateImporting           VMImageState = "importing"
	VMImageStateImported            VMImageState = "imported"
	VMImageStateBackingImageMissing VMImageState = "backing-image-missing"
)

type VMIOperator interface {
	UpdateVMI(oldVMI, newVMI *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error)

	GetVMImageObj(namespace, name string) (*harvesterv1.VirtualMachineImage, error)
	GetName(vmi *harvesterv1.VirtualMachineImage) string
	GetNamespace(vmi *harvesterv1.VirtualMachineImage) string
	GetVirtualSize(vmi *harvesterv1.VirtualMachineImage) int64
	GetSCParameters(vmi *harvesterv1.VirtualMachineImage) map[string]string
	GetSourceType(vmi *harvesterv1.VirtualMachineImage) harvesterv1.VirtualMachineImageSourceType
	GetURL(vmi *harvesterv1.VirtualMachineImage) string
	GetUID(vmi *harvesterv1.VirtualMachineImage) types.UID
	GetChecksum(vmi *harvesterv1.VirtualMachineImage) string
	GetPVCNamespace(vmi *harvesterv1.VirtualMachineImage) string
	GetPVCName(vmi *harvesterv1.VirtualMachineImage) string
	GetSecurityCryptoOption(vmi *harvesterv1.VirtualMachineImage) string
	GetSecuritySrcImgNamespace(vmi *harvesterv1.VirtualMachineImage) string
	GetSecuritySrcImgName(vmi *harvesterv1.VirtualMachineImage) string
	GetBackupTarget(vmi *harvesterv1.VirtualMachineImage) *harvesterv1.BackupTarget
	GetDisplayName(vmi *harvesterv1.VirtualMachineImage) string

	IsInitialized(vmi *harvesterv1.VirtualMachineImage) bool
	IsImported(vmi *harvesterv1.VirtualMachineImage) bool
	IsDecryptOperation(vmi *harvesterv1.VirtualMachineImage) bool
	IsEncryptOperation(vmi *harvesterv1.VirtualMachineImage) bool

	CheckURLAndUpdate(old *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error)
	UpdateVirtualSize(old *harvesterv1.VirtualMachineImage, virtualSize int64) (*harvesterv1.VirtualMachineImage, error)
	UpdateSize(old *harvesterv1.VirtualMachineImage, size int64) (*harvesterv1.VirtualMachineImage, error)
	UpdateVirtualSizeAndSize(old *harvesterv1.VirtualMachineImage, virtualSize, size int64) (*harvesterv1.VirtualMachineImage, error)
	UpdateLastFailedTime(old *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error)
	UpdateBackupTarget(old *harvesterv1.VirtualMachineImage, bt *harvesterv1.BackupTarget) (*harvesterv1.VirtualMachineImage, error)

	FailUpload(old *harvesterv1.VirtualMachineImage, msg string) error

	FailInitial(old *harvesterv1.VirtualMachineImage, err error) (*harvesterv1.VirtualMachineImage, error)
	FailImported(old *harvesterv1.VirtualMachineImage, err error, progress int) (*harvesterv1.VirtualMachineImage, error)
	Initialized(old *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error)
	Importing(old *harvesterv1.VirtualMachineImage, msg string, progress int) (*harvesterv1.VirtualMachineImage, error)
	Imported(old *harvesterv1.VirtualMachineImage, msg string, progress int, size, virtualSize int64) (*harvesterv1.VirtualMachineImage, error)
	BackingImageMissing(old *harvesterv1.VirtualMachineImage, err error) (*harvesterv1.VirtualMachineImage, error)
}

type vmiOperator struct {
	client     ctlharvesterv1.VirtualMachineImageClient
	cache      ctlharvesterv1.VirtualMachineImageCache
	httpClient http.Client
}

func GetVMIOperator(client ctlharvesterv1.VirtualMachineImageClient, cache ctlharvesterv1.VirtualMachineImageCache, httpClient http.Client) VMIOperator {
	return &vmiOperator{client, cache, httpClient}
}

func (vmio *vmiOperator) UpdateVMI(oldVMI, newVMI *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if reflect.DeepEqual(oldVMI, newVMI) {
		return newVMI, nil
	}

	return vmio.client.Update(newVMI)
}

func (vmio *vmiOperator) GetVMImageObj(namespace, name string) (*harvesterv1.VirtualMachineImage, error) {
	return vmio.cache.Get(namespace, name)
}

func (vmio *vmiOperator) GetName(vmi *harvesterv1.VirtualMachineImage) string {
	return vmi.Name
}

func (vmio *vmiOperator) GetNamespace(vmi *harvesterv1.VirtualMachineImage) string {
	return vmi.Namespace
}

func (vmio *vmiOperator) GetVirtualSize(vmi *harvesterv1.VirtualMachineImage) int64 {
	return vmi.Status.VirtualSize
}

func (vmio *vmiOperator) GetSCParameters(vmi *harvesterv1.VirtualMachineImage) map[string]string {
	return vmi.Spec.StorageClassParameters
}

func (vmio *vmiOperator) GetSourceType(vmi *harvesterv1.VirtualMachineImage) harvesterv1.VirtualMachineImageSourceType {
	return vmi.Spec.SourceType
}

func (vmio *vmiOperator) GetURL(vmi *harvesterv1.VirtualMachineImage) string {
	return vmi.Spec.URL
}

func (vmio *vmiOperator) GetUID(vmi *harvesterv1.VirtualMachineImage) types.UID {
	return vmi.GetUID()
}

func (vmio *vmiOperator) GetChecksum(vmi *harvesterv1.VirtualMachineImage) string {
	return vmi.Spec.Checksum
}

func (vmio *vmiOperator) GetPVCNamespace(vmi *harvesterv1.VirtualMachineImage) string {
	return vmi.Spec.PVCNamespace
}

func (vmio *vmiOperator) GetPVCName(vmi *harvesterv1.VirtualMachineImage) string {
	return vmi.Spec.PVCName
}

func (vmio *vmiOperator) GetSecurityCryptoOption(vmi *harvesterv1.VirtualMachineImage) string {
	return string(vmi.Spec.SecurityParameters.CryptoOperation)
}

func (vmio *vmiOperator) GetSecuritySrcImgNamespace(vmi *harvesterv1.VirtualMachineImage) string {
	return vmi.Spec.SecurityParameters.SourceImageNamespace
}

func (vmio *vmiOperator) GetSecuritySrcImgName(vmi *harvesterv1.VirtualMachineImage) string {
	return vmi.Spec.SecurityParameters.SourceImageName
}

func (vmio *vmiOperator) GetDisplayName(vmi *harvesterv1.VirtualMachineImage) string {
	return vmi.Spec.DisplayName
}

func (vmio *vmiOperator) GetBackupTarget(vmi *harvesterv1.VirtualMachineImage) *harvesterv1.BackupTarget {
	return vmi.Status.BackupTarget
}

func (vmio *vmiOperator) IsInitialized(vmi *harvesterv1.VirtualMachineImage) bool {
	return harvesterv1.ImageInitialized.IsTrue(vmi)
}

func (vmio *vmiOperator) IsImported(vmi *harvesterv1.VirtualMachineImage) bool {
	return harvesterv1.ImageImported.IsTrue(vmi)
}

func (vmio *vmiOperator) IsDecryptOperation(vmi *harvesterv1.VirtualMachineImage) bool {
	return vmi.Spec.SecurityParameters.CryptoOperation == harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt
}

func (vmio *vmiOperator) IsEncryptOperation(vmi *harvesterv1.VirtualMachineImage) bool {
	if vmi.Spec.SecurityParameters == nil {
		return false
	}

	return vmi.Spec.SecurityParameters.CryptoOperation == harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt
}

func (vmio *vmiOperator) CheckURLAndUpdate(old *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if old.Spec.SourceType != harvesterv1.VirtualMachineImageSourceTypeDownload {
		return old, nil
	}

	newVMI := old.DeepCopy()
	newVMI.Status.AppliedURL = newVMI.Spec.URL
	resp, err := vmio.httpClient.Head(newVMI.Spec.URL)
	if err != nil {
		return newVMI, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		err = fmt.Errorf("got %d status code from %s", resp.StatusCode, newVMI.Spec.URL)
		return newVMI, err
	}

	if resp.ContentLength > 0 {
		newVMI.Status.Size = resp.ContentLength
	}
	return vmio.UpdateVMI(old, newVMI)
}

func (vmio *vmiOperator) UpdateVirtualSize(old *harvesterv1.VirtualMachineImage, virtualSize int64) (*harvesterv1.VirtualMachineImage, error) {
	newVMI := old.DeepCopy()
	newVMI.Status.VirtualSize = virtualSize
	return vmio.UpdateVMI(old, newVMI)
}

func (vmio *vmiOperator) UpdateSize(old *harvesterv1.VirtualMachineImage, size int64) (*harvesterv1.VirtualMachineImage, error) {
	newVMI := old.DeepCopy()
	newVMI.Status.Size = size
	return vmio.UpdateVMI(old, newVMI)
}

func (vmio *vmiOperator) UpdateVirtualSizeAndSize(old *harvesterv1.VirtualMachineImage, virtualSize, size int64) (*harvesterv1.VirtualMachineImage, error) {
	newVMI := old.DeepCopy()
	newVMI.Status.VirtualSize = virtualSize
	newVMI.Status.Size = size
	return vmio.UpdateVMI(old, newVMI)
}

func (vmio *vmiOperator) UpdateLastFailedTime(old *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	newVMI := old.DeepCopy()
	newVMI.Status.LastFailedTime = time.Now().Format(time.RFC3339)
	return vmio.UpdateVMI(old, newVMI)
}

func (vmio *vmiOperator) UpdateBackupTarget(old *harvesterv1.VirtualMachineImage, bt *harvesterv1.BackupTarget) (*harvesterv1.VirtualMachineImage, error) {
	newVMI := old.DeepCopy()
	newVMI.Status.BackupTarget = bt
	return vmio.UpdateVMI(old, newVMI)
}

func (vmio *vmiOperator) FailUpload(old *harvesterv1.VirtualMachineImage, msg string) error {
	retry := 3
	for i := 0; i < retry; i++ {
		current, err := vmio.cache.Get(old.Namespace, old.Name)
		if err != nil {
			return err
		}
		if current.DeletionTimestamp != nil {
			return nil
		}
		newVMI := current.DeepCopy()
		harvesterv1.ImageImported.False(newVMI)
		harvesterv1.ImageImported.Reason(newVMI, uploadFailReason)
		harvesterv1.ImageImported.Message(newVMI, msg)
		_, err = vmio.UpdateVMI(old, newVMI)
		if err == nil || !apierrors.IsConflict(err) {
			return err
		}
		time.Sleep(2 * time.Second)
	}
	return errors.New("failed to update image uploaded condition, max retries exceeded")
}

func (vmio *vmiOperator) failStateTransit(old *harvesterv1.VirtualMachineImage, cond condition.Cond, msg string, progress int) (*harvesterv1.VirtualMachineImage, error) {
	newVMI := old.DeepCopy()
	newVMI.Status.Failed++
	newVMI.Status.LastFailedTime = time.Now().Format(time.RFC3339)
	newVMI.Status.Progress = progress
	errMsg := fmt.Sprintf("failed due to error: %s", msg)
	if newVMI.Status.Failed > 1 {
		errMsg = fmt.Sprintf("retry attempted %d/%d %s", newVMI.Status.Failed-1, newVMI.Spec.Retry, errMsg)
	}
	if newVMI.Status.Failed > newVMI.Spec.Retry {
		harvesterv1.ImageRetryLimitExceeded.True(newVMI)
		harvesterv1.ImageRetryLimitExceeded.Message(newVMI, errMsg)
		cond.False(newVMI)
		cond.Message(newVMI, errMsg)
	} else {
		harvesterv1.ImageRetryLimitExceeded.False(newVMI)
		harvesterv1.ImageRetryLimitExceeded.Message(newVMI, errMsg)
	}
	return vmio.UpdateVMI(old, newVMI)
}

func (vmio *vmiOperator) stateTransit(old *harvesterv1.VirtualMachineImage, state VMImageState, msg string, progress int, size, virtualSize int64) (*harvesterv1.VirtualMachineImage, error) {
	switch state {
	case VMImageStateInitialFail:
		return vmio.failStateTransit(old, condition.Cond(harvesterv1.ImageInitialized), msg, 0)
	case VMImageStateImportFail:
		return vmio.failStateTransit(old, condition.Cond(harvesterv1.ImageImported), msg, progress)
	case VMImageStateInitialized:
		newVMI := old.DeepCopy()
		newVMI.Status.AppliedURL = newVMI.Spec.URL
		newVMI.Status.StorageClassName = util.GetImageStorageClassName(newVMI)
		newVMI.Status.Progress = 0

		harvesterv1.ImageImported.Unknown(newVMI)
		harvesterv1.ImageImported.Reason(newVMI, "Importing")
		harvesterv1.ImageImported.LastUpdated(newVMI, time.Now().Format(time.RFC3339))
		harvesterv1.ImageInitialized.True(newVMI)
		harvesterv1.ImageInitialized.Message(newVMI, "")
		harvesterv1.ImageInitialized.Reason(newVMI, "Initialized")
		harvesterv1.ImageInitialized.LastUpdated(newVMI, time.Now().Format(time.RFC3339))
		return vmio.UpdateVMI(old, newVMI)
	case VMImageStateImporting:
		// backing image file upload progress can be 100 before it is ready
		// Set VM image progress to be 99 for better UX in this case
		if progress == 100 {
			progress = 99
		}

		newVMI := old.DeepCopy()
		newVMI.Status.Progress = progress
		harvesterv1.ImageImported.Unknown(newVMI)
		harvesterv1.ImageImported.Reason(newVMI, "Importing")
		harvesterv1.ImageImported.Message(newVMI, msg)
		return vmio.UpdateVMI(old, newVMI)
	case VMImageStateImported:
		newVMI := old.DeepCopy()
		newVMI.Status.Progress = progress
		newVMI.Status.Size = size
		newVMI.Status.VirtualSize = virtualSize
		harvesterv1.ImageImported.True(newVMI)
		harvesterv1.ImageImported.Reason(newVMI, "Imported")
		harvesterv1.ImageImported.Message(newVMI, msg)
		// Clear the ImageRetryLimitExceeded reason and message to prevent the error message
		// from lingering in the Harvester dashboard after multiple image import retries
		// have failed but eventually succeeded.
		harvesterv1.ImageRetryLimitExceeded.False(newVMI)
		harvesterv1.ImageRetryLimitExceeded.Reason(newVMI, "")
		harvesterv1.ImageRetryLimitExceeded.Message(newVMI, "")
		return vmio.UpdateVMI(old, newVMI)
	case VMImageStateBackingImageMissing:
		newVMI := old.DeepCopy()
		harvesterv1.BackingImageMissing.True(newVMI)
		harvesterv1.BackingImageMissing.Message(newVMI, msg)
		return vmio.UpdateVMI(old, newVMI)
	}
	return old, nil
}

func (vmio *vmiOperator) FailInitial(old *harvesterv1.VirtualMachineImage, err error) (*harvesterv1.VirtualMachineImage, error) {
	return vmio.stateTransit(old, VMImageStateInitialFail, err.Error(), 0, 0, 0)
}

func (vmio *vmiOperator) FailImported(old *harvesterv1.VirtualMachineImage, err error, progress int) (*harvesterv1.VirtualMachineImage, error) {
	return vmio.stateTransit(old, VMImageStateImportFail, err.Error(), progress, 0, 0)
}

func (vmio *vmiOperator) Initialized(old *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	return vmio.stateTransit(old, VMImageStateInitialized, "", 0, 0, 0)
}

func (vmio *vmiOperator) Importing(old *harvesterv1.VirtualMachineImage, msg string, progress int) (*harvesterv1.VirtualMachineImage, error) {
	return vmio.stateTransit(old, VMImageStateImporting, msg, progress, 0, 0)
}

func (vmio *vmiOperator) Imported(old *harvesterv1.VirtualMachineImage, msg string, progress int, size, virtualSize int64) (*harvesterv1.VirtualMachineImage, error) {
	return vmio.stateTransit(old, VMImageStateImported, msg, progress, size, virtualSize)
}

func (vmio *vmiOperator) BackingImageMissing(old *harvesterv1.VirtualMachineImage, err error) (*harvesterv1.VirtualMachineImage, error) {
	return vmio.stateTransit(old, VMImageStateBackingImageMissing, fmt.Sprintf("Failed to get backing image: %s", err.Error()), 0, 0, 0)
}
