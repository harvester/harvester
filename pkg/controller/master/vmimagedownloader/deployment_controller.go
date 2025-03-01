package vmimagedownloader

import (
	"fmt"
	"strings"

	ctlappsv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

// storageProfileHandler dynamically manages storage profiles
type deploymentHandler struct {
	vmImageDownloaderClient v1beta1.VirtualMachineImageDownloaderClient
	deploymentCache         ctlappsv1.DeploymentCache
	deploymentController    ctlappsv1.DeploymentController
}

func (h *deploymentHandler) OnChanged(_ string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	if deployment == nil || deployment.DeletionTimestamp != nil {
		return deployment, nil
	}

	return h.updateOrNoOPDownloaderStatus(deployment)
}

func (h *deploymentHandler) OnRemoved(_ string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	if deployment == nil {
		return nil, nil
	}
	return h.updateOrNoOPDownloaderStatus(deployment)
}

func (h *deploymentHandler) updateOrNoOPDownloaderStatus(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	downloaderName := strings.TrimSuffix(deployment.Name, "-downloader")
	vmImageDownloader, err := h.vmImageDownloaderClient.Get(deployment.Namespace, downloaderName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return deployment, nil
		}
		return deployment, fmt.Errorf("failed to get vm image downloader %s: %v", downloaderName, err)
	}

	// if the downloader is already in progress, no need to update the status
	if vmImageDownloader.Status.Status == harvesterv1.ImageDownloaderStatusProgressing {
		return deployment, nil
	}

	if deployment.DeletionTimestamp != nil || *deployment.Spec.Replicas != deployment.Status.ReadyReplicas {
		vmImageDownloaderCpy := vmImageDownloader.DeepCopy()
		conds := harvesterv1.VirtualMachineImageDownloaderCondition{
			Type:               harvesterv1.DownloaderCondsReconciling,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "VM Image Downloader Reconciling",
			Message:            "Waiting for the corresponding deployment to be ready",
		}
		updatedConditions := updateConds(vmImageDownloaderCpy.Status.Conditions, conds)
		vmImageDownloaderCpy.Status.Conditions = updatedConditions
		vmImageDownloaderCpy.Status.Status = harvesterv1.ImageDownloaderStatusProgressing
		if _, err := h.vmImageDownloaderClient.UpdateStatus(vmImageDownloaderCpy); err != nil {
			return deployment, fmt.Errorf("failed to update vm image downloader %s: %v", downloaderName, err)
		}
		return deployment, nil
	}

	return deployment, nil
}
