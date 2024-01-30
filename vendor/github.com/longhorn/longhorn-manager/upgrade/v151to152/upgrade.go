package v151to152

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	upgradeutil "github.com/longhorn/longhorn-manager/upgrade/util"
)

const (
	upgradeLogPrefix = "upgrade from v1.5.1 to v1.5.2: "
)

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset, resourceMaps map[string]interface{}) error {
	// We will probably need to upgrade other resources as well. See upgradeVolumeAttachments or previous Longhorn
	// versions for examples.
	if err := upgradeVolumeAttachments(namespace, lhClient, resourceMaps); err != nil {
		return err
	}

	return deleteCSIServices(namespace, kubeClient)
}

func upgradeVolumeAttachments(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade VolumeAttachment failed")
	}()

	volumeAttachmentMap, err := upgradeutil.ListAndUpdateVolumeAttachmentsInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn VolumeAttachments during the Longhorn VolumeAttachment upgrade")
	}

	snapshotMap, err := upgradeutil.ListAndUpdateSnapshotsInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Snapshots during the Longhorn VolumeAttachment a upgrade")
	}

	ticketIDsForExistingSnapshotsMap := map[string]interface{}{}
	for snapshotName := range snapshotMap {
		ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeSnapshotController, snapshotName)
		ticketIDsForExistingSnapshotsMap[ticketID] = nil
	}

	// Previous Longhorn versions may have created attachmentTickets for snapshots that no longer exist. Clean these up.
	for _, volumeAttachment := range volumeAttachmentMap {
		for ticketID, ticket := range volumeAttachment.Spec.AttachmentTickets {
			if ticket.Type != longhorn.AttacherTypeSnapshotController {
				continue
			}
			if _, ok := ticketIDsForExistingSnapshotsMap[ticketID]; !ok {
				delete(volumeAttachment.Spec.AttachmentTickets, ticketID)
			}
		}
	}

	return nil
}

func deleteCSIServices(namespace string, kubeClient *clientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"delete CSI service failed")
	}()

	servicesToDelete := map[string]struct{}{
		types.CSIAttacherName:    {},
		types.CSIProvisionerName: {},
		types.CSIResizerName:     {},
		types.CSISnapshotterName: {},
	}

	servicesInCluster, err := kubeClient.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list all existing Longhorn services during the CSI service deletion")
	}

	for _, serviceInCluster := range servicesInCluster.Items {
		if _, ok := servicesToDelete[serviceInCluster.Name]; !ok {
			continue
		}
		if err = kubeClient.CoreV1().Services(namespace).Delete(context.TODO(), serviceInCluster.Name, metav1.DeleteOptions{}); err != nil {
			// Best effort. Dummy services have no function and no finalizer, so we can proceed on failure.
			logrus.Warnf("Deprecated CSI dummy service could not be deleted during upgrade: %v", err)
		}
	}

	return nil
}

func UpgradeResourcesStatus(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset, resourceMaps map[string]interface{}) error {
	// Currently there are no statuses to upgrade. See UpgradeResources -> upgradeVolumes or previous Longhorn versions
	// for examples.
	return nil
}
