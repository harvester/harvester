package v14xto150

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	"github.com/longhorn/longhorn-manager/types"
	upgradeutil "github.com/longhorn/longhorn-manager/upgrade/util"
	"github.com/longhorn/longhorn-manager/util"
)

const (
	upgradeLogPrefix = "upgrade from v1.4.x to v1.5.0: "

	maxRetryForDeploymentDeletion = 300
)

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset, resourceMaps map[string]interface{}) error {
	if err := upgradeCSIPlugin(namespace, kubeClient); err != nil {
		return err
	}

	if err := upgradeWebhookAndRecoveryService(namespace, kubeClient); err != nil {
		return err
	}

	if err := upgradeWebhookPDB(namespace, kubeClient); err != nil {
		return err
	}

	if err := upgradeVolumes(namespace, lhClient, resourceMaps); err != nil {
		return err
	}

	if err := upgradeVolumeAttachments(namespace, lhClient, kubeClient, resourceMaps); err != nil {
		return err
	}

	if err := upgradeNodes(namespace, lhClient, resourceMaps); err != nil {
		return err
	}

	if err := upgradeEngines(namespace, lhClient, resourceMaps); err != nil {
		return err
	}

	if err := upgradeReplicas(namespace, lhClient, resourceMaps); err != nil {
		return err
	}

	return upgradeOrphans(namespace, lhClient, resourceMaps)
}

func upgradeNodes(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade node failed")
	}()

	nodeMap, err := upgradeutil.ListAndUpdateNodesInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn nodes during the node upgrade")
	}

	for _, n := range nodeMap {
		if n.Spec.Disks != nil {
			for name, disk := range n.Spec.Disks {
				if disk.Type == "" {
					disk.Type = longhorn.DiskTypeFilesystem
					n.Spec.Disks[name] = disk
				}
			}
		}
	}

	return nil
}

func upgradeVolumes(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade volume failed")
	}()

	volumeMap, err := upgradeutil.ListAndUpdateVolumesInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn volumes during the volume upgrade")
	}

	for _, v := range volumeMap {
		if v.Spec.BackupCompressionMethod == "" {
			v.Spec.BackupCompressionMethod = longhorn.BackupCompressionMethodGzip
		}
		if v.Spec.DataLocality == longhorn.DataLocalityStrictLocal {
			v.Spec.RevisionCounterDisabled = true
		}
		if v.Spec.ReplicaSoftAntiAffinity == "" {
			v.Spec.ReplicaSoftAntiAffinity = longhorn.ReplicaSoftAntiAffinityDefault
		}
		if v.Spec.ReplicaZoneSoftAntiAffinity == "" {
			v.Spec.ReplicaZoneSoftAntiAffinity = longhorn.ReplicaZoneSoftAntiAffinityDefault
		}
		if v.Spec.BackendStoreDriver == "" {
			v.Spec.BackendStoreDriver = longhorn.BackendStoreDriverTypeV1
		}
		if v.Spec.OfflineReplicaRebuilding == "" {
			v.Spec.OfflineReplicaRebuilding = longhorn.OfflineReplicaRebuildingDisabled
		}
	}

	return nil
}

func upgradeVolumeAttachments(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade VolumeAttachment failed")
	}()

	volumeList, err := lhClient.LonghornV1beta2().Volumes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list all existing Longhorn volumes during the Longhorn VolumeAttachment upgrade")
	}
	kubeVAList, err := kubeClient.StorageV1().VolumeAttachments().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list all Kubernetes VolumeAttachments during the Longhorn VolumeAttachment upgrade")
	}
	smList, err := lhClient.LonghornV1beta2().ShareManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list all share manager during the Longhorn VolumeAttachment upgrade")
	}

	// We don't expect any volume attachments to exist when upgrading from v1.4.x to v1.5.0. However, we could be
	// walking the upgrade path again after a failure.
	volumeAttachmentMap, err := upgradeutil.ListAndUpdateVolumeAttachmentsInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn VolumeAttachments during the Longhorn VolumeAttachment upgrade")
	}

	kubeVAMap := map[string][]storagev1.VolumeAttachment{}
	for _, va := range kubeVAList.Items {
		if va.Spec.Source.PersistentVolumeName != nil {
			volName := *va.Spec.Source.PersistentVolumeName
			kubeVAMap[volName] = append(kubeVAMap[volName], va)
		}
	}

	smMap := map[string]longhorn.ShareManager{}
	for _, sm := range smList.Items {
		smMap[sm.Name] = sm
	}

	for _, v := range volumeList.Items {
		if _, ok := volumeAttachmentMap[types.GetLHVolumeAttachmentNameFromVolumeName(v.Name)]; ok {
			continue // We handled this one on a previous upgrade attempt.
		}

		va := longhorn.VolumeAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:   types.GetLHVolumeAttachmentNameFromVolumeName(v.Name),
				Labels: types.GetVolumeLabels(v.Name),
			},
			Spec: longhorn.VolumeAttachmentSpec{
				AttachmentTickets: generateVolumeAttachmentTickets(v, kubeVAMap, smMap),
				Volume:            v.Name,
			},
		}
		volumeAttachmentMap[va.Name] = &va
	}

	return nil
}

func generateVolumeAttachmentTickets(vol longhorn.Volume, kubeVAMap map[string][]storagev1.VolumeAttachment, smMap map[string]longhorn.ShareManager) map[string]*longhorn.AttachmentTicket {
	attachmentTickets := make(map[string]*longhorn.AttachmentTicket)

	kubeVAs := kubeVAMap[vol.Name]

	for _, kubeVA := range kubeVAs {
		if !kubeVA.DeletionTimestamp.IsZero() {
			continue
		}
		ticketID := kubeVA.Name
		attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
			ID:     ticketID,
			Type:   longhorn.AttacherTypeCSIAttacher,
			NodeID: kubeVA.Spec.NodeName,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: longhorn.FalseValue,
			},
		}
	}

	if sm, ok := smMap[vol.Name]; ok && sm.Status.State == longhorn.ShareManagerStateRunning {
		ticketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeShareManagerController, sm.Name)
		attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
			ID:     ticketID,
			Type:   longhorn.AttacherTypeShareManagerController,
			NodeID: sm.Status.OwnerID,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: longhorn.FalseValue,
			},
		}
	}

	if vol.Spec.NodeID != "" && len(attachmentTickets) == 0 {
		ticketID := "longhorn-ui"
		attachmentTickets[ticketID] = &longhorn.AttachmentTicket{
			ID:     ticketID,
			Type:   longhorn.AttacherTypeLonghornAPI,
			NodeID: vol.Spec.NodeID,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: strconv.FormatBool(vol.Spec.DisableFrontend),
			},
		}
	}

	return attachmentTickets
}

func upgradeWebhookAndRecoveryService(namespace string, kubeClient *clientset.Clientset) error {
	selectors := []string{"app=longhorn-conversion-webhook", "app=longhorn-admission-webhook", "app=longhorn-recovery-backend"}
	propagation := metav1.DeletePropagationForeground

	for _, selector := range selectors {
		deployments, err := kubeClient.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return errors.Wrapf(err, upgradeLogPrefix+"failed to get deployment with label %v during the upgrade", selector)
		}

		for _, deployment := range deployments.Items {
			err := kubeClient.AppsV1().Deployments(deployment.Namespace).Delete(context.TODO(), deployment.Name, metav1.DeleteOptions{PropagationPolicy: &propagation})
			if err != nil {
				return errors.Wrapf(err, upgradeLogPrefix+"failed to delete the deployment with label %v during the upgrade", selector)
			}

			err = util.WaitForResourceDeletion(kubeClient, deployment.Name, deployment.Namespace, selector, maxRetryForDeploymentDeletion, deploymentGetFunc)
			if err != nil {
				return errors.Wrapf(err, upgradeLogPrefix+"failed to wait for the deployment with label %v to be deleted during the upgrade", selector)
			}
			logrus.Infof("Deleted deployment %v with label %v during the upgrade", deployment.Name, selector)
		}
	}
	return nil
}

func deploymentGetFunc(kubeClient *clientset.Clientset, name, namespace string) (runtime.Object, error) {
	return kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func upgradeReplicas(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade replica failed")
	}()

	replicaMap, err := upgradeutil.ListAndUpdateReplicasInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn replicas during the replica upgrade")
	}

	for _, r := range replicaMap {
		if r.Spec.BackendStoreDriver == "" {
			r.Spec.BackendStoreDriver = longhorn.BackendStoreDriverTypeV1
		}
	}

	return nil
}

func upgradeEngines(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade engine failed")
	}()

	engineMap, err := upgradeutil.ListAndUpdateEnginesInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn engines during the engine upgrade")
	}

	for _, e := range engineMap {
		if e.Spec.BackendStoreDriver == "" {
			e.Spec.BackendStoreDriver = longhorn.BackendStoreDriverTypeV1
		}
	}

	return nil
}

func upgradeOrphans(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade orphan failed")
	}()

	orphanMap, err := upgradeutil.ListAndUpdateOrphansInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn orphans during the orphan upgrade")
	}

	for _, o := range orphanMap {
		if o.Spec.Parameters == nil {
			continue
		}

		if _, ok := o.Spec.Parameters[longhorn.OrphanDiskType]; !ok {
			o.Spec.Parameters[longhorn.OrphanDiskType] = string(longhorn.DiskTypeFilesystem)
		}
	}

	return nil
}

func UpgradeResourcesStatus(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset, resourceMaps map[string]interface{}) error {
	return upgradeNodeStatus(namespace, lhClient, resourceMaps)
}

func upgradeNodeStatus(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade node failed")
	}()

	nodeMap, err := upgradeutil.ListAndUpdateNodesInProvidedCache(namespace, lhClient, resourceMaps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn nodes during the node upgrade")
	}

	for _, n := range nodeMap {
		if n.Status.DiskStatus != nil {
			for name, disk := range n.Status.DiskStatus {
				if disk.Type == "" {
					disk.Type = longhorn.DiskTypeFilesystem
					n.Status.DiskStatus[name] = disk
				}
			}
		}
	}

	return nil
}

func upgradeWebhookPDB(namespace string, kubeClient *clientset.Clientset) error {
	webhookPDBs := []string{"longhorn-admission-webhook", "longhorn-conversion-webhook"}

	for _, pdb := range webhookPDBs {
		err := kubeClient.PolicyV1().PodDisruptionBudgets(namespace).Delete(context.TODO(), pdb, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, upgradeLogPrefix+"failed to delete the pdb %v during the upgrade", pdb)
		}
	}

	return nil
}

func upgradeCSIPlugin(namespace string, kubeClient *clientset.Clientset) error {
	err := kubeClient.AppsV1().DaemonSets(namespace).Delete(context.TODO(), types.CSIPluginName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, upgradeLogPrefix+"failed to delete the %v daemonset during the upgrade", types.CSIPluginName)
	}
	return nil
}
