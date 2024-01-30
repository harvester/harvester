package controller

import (
	"fmt"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type VolumeAttachmentController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced
}

func NewLonghornVolumeAttachmentController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string) *VolumeAttachmentController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)

	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	vac := &VolumeAttachmentController{
		baseController: newBaseController("longhorn-volume-attachment", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-volume-attachment-controller"}),
	}

	ds.LHVolumeAttachmentInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    vac.enqueueVolumeAttachment,
		UpdateFunc: func(old, cur interface{}) { vac.enqueueVolumeAttachment(cur) },
		DeleteFunc: vac.enqueueVolumeAttachment,
	}, 0)
	vac.cacheSyncs = append(vac.cacheSyncs, ds.LHVolumeAttachmentInformer.HasSynced)

	ds.VolumeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    vac.enqueueForLonghornVolume,
		UpdateFunc: func(old, cur interface{}) { vac.enqueueForLonghornVolume(cur) },
		DeleteFunc: vac.enqueueForLonghornVolume,
	}, 0)
	vac.cacheSyncs = append(vac.cacheSyncs, ds.VolumeInformer.HasSynced)

	ds.EngineInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    vac.enqueueEngineChange,
		UpdateFunc: func(old, cur interface{}) { vac.enqueueEngineChange(cur) },
		DeleteFunc: vac.enqueueEngineChange,
	}, 0)
	vac.cacheSyncs = append(vac.cacheSyncs, ds.EngineInformer.HasSynced)

	return vac
}

func (vac *VolumeAttachmentController) enqueueVolumeAttachment(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}
	vac.queue.Add(key)
}

func (vac *VolumeAttachmentController) enqueueVolumeAttachmentAfter(obj interface{}, duration time.Duration) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("enqueueVolumeAttachmentAfter: couldn't get key for object %#v: %v", obj, err))
		return
	}

	vac.queue.AddAfter(key, duration)
}

func (vac *VolumeAttachmentController) enqueueForLonghornVolume(obj interface{}) {
	vol, ok := obj.(*longhorn.Volume)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}
		// use the last known state, to enqueue, dependent objects
		vol, ok = deletedState.Obj.(*longhorn.Volume)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	volumeAttachments, err := vac.ds.ListLonghornVolumeAttachmentByVolumeRO(vol.Name)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to list Longhorn VolumeAttachment of volume %v: %v", vol.Name, err))
		return
	}

	for _, va := range volumeAttachments {
		vac.enqueueVolumeAttachment(va)
	}
}

func (vac *VolumeAttachmentController) enqueueEngineChange(obj interface{}) {
	e, ok := obj.(*longhorn.Engine)

	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}
		// use the last known state, to enqueue, dependent objects
		e, ok = deletedState.Obj.(*longhorn.Engine)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	volumeAttachments, err := vac.ds.ListLonghornVolumeAttachmentByVolumeRO(e.Spec.VolumeName)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to list Longhorn VolumeAttachment of volume %v: %v", e.Name, err))
		return
	}

	for _, va := range volumeAttachments {
		vac.enqueueVolumeAttachment(va)
	}

}

func (vac *VolumeAttachmentController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer vac.queue.ShutDown()

	vac.logger.Info("Start Longhorn VolumeAttachment controller")
	defer vac.logger.Info("Shutting down Longhorn VolumeAttachment controller")

	if !cache.WaitForNamedCacheSync(vac.name, stopCh, vac.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(vac.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (vac *VolumeAttachmentController) worker() {
	for vac.processNextWorkItem() {
	}
}

func (vac *VolumeAttachmentController) processNextWorkItem() bool {
	key, quit := vac.queue.Get()
	if quit {
		return false
	}
	defer vac.queue.Done(key)
	err := vac.syncHandler(key.(string))
	vac.handleErr(err, key)
	return true
}

func (vac *VolumeAttachmentController) handleErr(err error, key interface{}) {
	if err == nil {
		vac.queue.Forget(key)
		return
	}

	log := vac.logger.WithField("LonghornVolumeAttachment", key)
	handleReconcileErrorLogging(log, err, "Failed to sync Longhorn VolumeAttachment")
	vac.queue.AddRateLimited(key)
}

func (vac *VolumeAttachmentController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: failed to sync VolumeAttachment %v", vac.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != vac.namespace {
		return nil
	}
	return vac.reconcile(name)
}

func (vac *VolumeAttachmentController) reconcile(vaName string) (err error) {
	va, err := vac.ds.GetLHVolumeAttachment(vaName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	isResponsible, err := vac.isResponsibleFor(va)
	if err != nil {
		return err
	}
	if !isResponsible {
		return nil
	}

	vol, err := vac.ds.GetVolume(va.Spec.Volume)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if !va.DeletionTimestamp.IsZero() {
				return vac.ds.RemoveFinalizerForLHVolumeAttachment(va)
			}
			return nil
		}
		return err
	}

	existingVA := va.DeepCopy()
	existingVol := vol.DeepCopy()
	defer func() {
		if err != nil {
			return
		}
		if !reflect.DeepEqual(existingVol.Spec, vol.Spec) {
			if _, err = vac.ds.UpdateVolume(vol); err != nil {
				return
			}

		}
		if !reflect.DeepEqual(existingVA.Status, va.Status) {
			if _, err = vac.ds.UpdateLHVolumeAttachmentStatus(va); err != nil {
				return
			}
		}
		return
	}()

	// Note that in this controller the desire state is recorded in VA.Spec
	// and the current state of the world is recorded inside volume CR

	vac.handleVolumeDetachment(va, vol)

	vac.handleVolumeAttachment(va, vol)

	vac.handleVolumeMigration(va, vol)

	return vac.handleVAStatusUpdate(va, vol)
}

func (vac *VolumeAttachmentController) handleVolumeMigration(va *longhorn.VolumeAttachment, vol *longhorn.Volume) {
	if !isMigratableVolume(vol) {
		return
	}

	vac.handleVolumeMigrationStart(va, vol)
	vac.handleVolumeMigrationConfirmation(va, vol)
	vac.handleVolumeMigrationRollback(va, vol)
}

func (vac *VolumeAttachmentController) handleVolumeMigrationStart(va *longhorn.VolumeAttachment, vol *longhorn.Volume) {
	if isVolumeMigrating(vol) {
		return
	}
	// migration start
	// if the volume is currently attached by a csi attachment ticket and there is another
	// csi attachment ticket requesting a different node
	if vol.Status.State != longhorn.VolumeStateAttached ||
		vol.Status.CurrentNodeID != vol.Spec.NodeID {
		return
	}

	hasCSIAttachmentTicket := false
	for _, attachmentTicket := range va.Spec.AttachmentTickets {
		if attachmentTicket.Type != longhorn.AttacherTypeCSIAttacher {
			continue
		}
		// Found one csi attachmentTicket that is requesting volume to attach to the current node
		if attachmentTicket.NodeID == vol.Spec.NodeID && verifyAttachmentParameters(attachmentTicket.Parameters, vol) {
			hasCSIAttachmentTicket = true
		}
	}

	if !hasCSIAttachmentTicket {
		return
	}

	for _, attachmentTicket := range va.Spec.AttachmentTickets {
		if attachmentTicket.Type != longhorn.AttacherTypeCSIAttacher {
			continue
		}
		// Found one csi attachmentTicket that is requesting volume to attach to a different node
		if attachmentTicket.NodeID != vol.Spec.NodeID {
			vol.Spec.MigrationNodeID = attachmentTicket.NodeID
		}
	}

}

func (vac *VolumeAttachmentController) handleVolumeMigrationConfirmation(va *longhorn.VolumeAttachment, vol *longhorn.Volume) {
	// Migration has not been started yet
	if vol.Status.CurrentMigrationNodeID == "" {
		return
	}

	hasCSIAttachmentTicketRequestingPrevNode := false
	for _, attachmentTicket := range va.Spec.AttachmentTickets {
		if attachmentTicket.Type != longhorn.AttacherTypeCSIAttacher {
			continue
		}
		// Found one csi attachmentTicket that is requesting volume to attach to the current node
		if attachmentTicket.NodeID == vol.Spec.NodeID && verifyAttachmentParameters(attachmentTicket.Parameters, vol) {
			hasCSIAttachmentTicketRequestingPrevNode = true
			break
		}
	}
	migratingEngineSnapSynced, err := vac.checkMigratingEngineSyncSnapshots(va, vol)
	if err != nil {
		vac.logger.WithError(err).Warn("Failed to check migrating engine snapshot status")
		return
	}
	if !hasCSIAttachmentTicketRequestingPrevNode && migratingEngineSnapSynced {
		// TODO: Do we need check if the volume is available on the currentMigrationIDNode?
		vol.Spec.NodeID = vol.Status.CurrentMigrationNodeID
		vol.Spec.MigrationNodeID = ""
	}
}

func (vac *VolumeAttachmentController) checkMigratingEngineSyncSnapshots(va *longhorn.VolumeAttachment, vol *longhorn.Volume) (bool, error) {
	engines, err := vac.ds.ListVolumeEngines(vol.Name)
	if err != nil {
		return false, err
	}

	var migratingEngine *longhorn.Engine
	var oldEngine *longhorn.Engine
	for _, e := range engines {
		if e.Spec.Active {
			oldEngine = e
			continue
		}
		if e.Spec.NodeID == vol.Spec.MigrationNodeID {
			migratingEngine = e
		}
	}

	if oldEngine == nil {
		return false, fmt.Errorf("failed to find the active engine for volume %v", vol.Name)
	}

	if migratingEngine == nil {
		return false, fmt.Errorf("failed to find the migrating engine for volume %v", vol.Name)
	}

	if !hasSameKeys(oldEngine.Status.Snapshots, migratingEngine.Status.Snapshots) {
		vac.logger.Infof("Volume migration (%v) is in progress for synchronizing snapshots", vol.Name)
		// there is a chance that synchronizing engine snapshots does not finish and volume attachment controller will not receive changes anymore
		// check volumeAttachments again  to ensure that migration will be finished
		vac.enqueueVolumeAttachmentAfter(va, 10*time.Second)
		return false, nil
	}

	return true, nil
}

func hasSameKeys(map1, map2 map[string]*longhorn.SnapshotInfo) bool {
	if len(map1) != len(map2) {
		return false
	}

	for key := range map1 {
		if _, ok := map2[key]; !ok {
			return false
		}
	}

	return true
}

func (vac *VolumeAttachmentController) handleVolumeMigrationRollback(va *longhorn.VolumeAttachment, vol *longhorn.Volume) {
	// Nothing to rollback
	if vol.Spec.MigrationNodeID == "" {
		return
	}

	hasCSIAttachmentTicketRequestingMigratingNode := false
	for _, attachmentTicket := range va.Spec.AttachmentTickets {
		if attachmentTicket.Type != longhorn.AttacherTypeCSIAttacher {
			continue
		}
		// Found one csi attachmentTicket that is requesting volume to attach to the current node
		if attachmentTicket.NodeID == vol.Spec.MigrationNodeID {
			hasCSIAttachmentTicketRequestingMigratingNode = true
		}
	}
	if !hasCSIAttachmentTicketRequestingMigratingNode {
		vol.Spec.MigrationNodeID = ""
	}
}

func (vac *VolumeAttachmentController) handleVolumeDetachment(va *longhorn.VolumeAttachment, vol *longhorn.Volume) {
	log := getLoggerForLHVolumeAttachment(vac.logger, va)

	// Volume is already trying to detach
	if vol.Spec.NodeID == "" {
		return
	}

	if !vac.shouldDoDetach(va, vol) {
		return
	}

	log.Infof("Volume %v is selected to detach from node %v", vol.Name, vol.Spec.NodeID)

	// There is no attachment ticket that request the current vol.Spec.NodeID.
	// Therefore, set desire state of volume to empty
	vol.Spec.NodeID = ""
	// reset the attachment parameter for vol
	setAttachmentParameter(map[string]string{}, vol)
}

func (vac *VolumeAttachmentController) shouldDoDetach(va *longhorn.VolumeAttachment, vol *longhorn.Volume) bool {
	log := getLoggerForLHVolumeAttachment(vac.logger, va)
	// For auto salvage logic
	// TODO: create Auto Salvage controller to handle this logic instead of AD controller
	if vol.Status.Robustness == longhorn.VolumeRobustnessFaulted {
		return true
	}
	if isMigratableVolume(vol) && isVolumeMigrating(vol) {
		// if the volume is migrating, the detachment will be handled by handleVolumeMigration()
		return false
	}

	currentAttachmentTickets := map[string]*longhorn.AttachmentTicket{}
	attachmentTicketsOnOtherNodes := map[string]*longhorn.AttachmentTicket{}
	for _, attachmentTicket := range va.Spec.AttachmentTickets {
		// For the RWX volume attachment, VolumeAttachment controller will not directly handle
		// the tickets from the CSI plugin. Instead, ShareManager controller will add a
		// AttacherTypeShareManagerController ticket (as the summarization of CSI tickets) then
		// the VolumeAttachment controller is responsible for handling the AttacherTypeShareManagerController
		// tickets only. See more at https://github.com/longhorn/longhorn-manager/pull/1541#issuecomment-1429044946
		if isCSIAttacherTicketOfRegularRWXVolume(attachmentTicket, vol) {
			continue
		}
		if attachmentTicket.NodeID == vol.Spec.NodeID && verifyAttachmentParameters(attachmentTicket.Parameters, vol) {
			currentAttachmentTickets[attachmentTicket.ID] = attachmentTicket
		}
		if attachmentTicket.NodeID != vol.Spec.NodeID {
			attachmentTicketsOnOtherNodes[attachmentTicket.ID] = attachmentTicket
		}
	}

	if len(currentAttachmentTickets) == 0 {
		log.Infof("Should detach volume %v because there is no matching attachment ticket", vol.Spec.NodeID)
		return true
	}

	// Check if there is any workload ticket regardless of frontend on other nodes
	// If exist, detach and interrupt the current ticket.
	if !hasUninterruptibleTicket(currentAttachmentTickets) && hasWorkloadTicket(attachmentTicketsOnOtherNodes, longhorn.AnyValue) {
		log.Info("Workload attachment ticket interrupted snapshot/backup/rebuilding-controller attachment tickets")
		return true
	}

	// If there is an interruptible ticket and frontend disabled ticket on the current node (currently, only offline rebuilding ticket)
	// need to check if there is any workload ticket with frontend enabled (disableFrontend=false) on the same node.
	// If exist, detach and interrupt the rebuilding ticket.
	if hasInterruptibleAndFrontendDisabledTicket(currentAttachmentTickets) && hasWorkloadTicket(currentAttachmentTickets, longhorn.FalseValue) {
		log.Info("Workload attachment ticket interrupted rebuilding-controller attachment tickets")
		return true
	}

	return false
}

func hasUninterruptibleTicket(attachmentTickets map[string]*longhorn.AttachmentTicket) bool {
	for _, ticket := range attachmentTickets {
		if ticket.Type != longhorn.AttacherTypeSnapshotController &&
			ticket.Type != longhorn.AttacherTypeBackupController &&
			ticket.Type != longhorn.AttacherTypeVolumeRebuildingController {
			return true
		}
	}
	return false
}

func hasInterruptibleAndFrontendDisabledTicket(attachmentTickets map[string]*longhorn.AttachmentTicket) bool {
	for _, ticket := range attachmentTickets {
		if ticket.Type == longhorn.AttacherTypeVolumeRebuildingController {
			return true
		}
	}
	return false
}

func hasWorkloadTicket(attachmentTickets map[string]*longhorn.AttachmentTicket, disableFrontend string) bool {
	for _, ticket := range attachmentTickets {
		if ticket.Type == longhorn.AttacherTypeCSIAttacher ||
			ticket.Type == longhorn.AttacherTypeLonghornAPI ||
			ticket.Type == longhorn.AttacherTypeShareManagerController {
			if disableFrontend == longhorn.AnyValue {
				return true
			}
			if ticket.Parameters != nil {
				value, ok := ticket.Parameters[longhorn.AttachmentParameterDisableFrontend]
				if ok && value == disableFrontend {
					return true
				}
			}
		}
	}
	return false
}

func (vac *VolumeAttachmentController) handleVolumeAttachment(va *longhorn.VolumeAttachment, vol *longhorn.Volume) {
	log := getLoggerForLHVolumeAttachment(vac.logger, va)

	// Wait for volume to be fully detached
	if !isVolumeFullyDetached(vol) {
		return
	}

	// For auto salvage logic
	// TODO: create Auto Salvage controller to handle this logic instead of AD controller
	if vol.Status.Robustness == longhorn.VolumeRobustnessFaulted {
		return
	}

	attachmentTicket := selectAttachmentTicketToAttach(va, vol)
	if attachmentTicket == nil {
		return
	}

	log.Infof("Volume %v is selected to attach to node %v, ticket +%v", vol.Name, attachmentTicket.NodeID, attachmentTicket)

	vol.Spec.NodeID = attachmentTicket.NodeID
	setAttachmentParameter(attachmentTicket.Parameters, vol)
}

func selectAttachmentTicketToAttach(va *longhorn.VolumeAttachment, vol *longhorn.Volume) *longhorn.AttachmentTicket {
	ticketCandidates := []*longhorn.AttachmentTicket{}
	for _, attachmentTicket := range va.Spec.AttachmentTickets {
		if isCSIAttacherTicketOfRegularRWXVolume(attachmentTicket, vol) {
			continue
		}
		ticketCandidates = append(ticketCandidates, attachmentTicket)
	}

	maxAttacherPriorityLevel := 0
	for _, attachmentTicket := range ticketCandidates {
		priorityLevel := longhorn.GetAttacherPriorityLevel(attachmentTicket.Type)
		if priorityLevel > maxAttacherPriorityLevel {
			maxAttacherPriorityLevel = priorityLevel
		}
	}

	highPriorityTicketCandidates := []*longhorn.AttachmentTicket{}
	for _, attachmentTicket := range ticketCandidates {
		priorityLevel := longhorn.GetAttacherPriorityLevel(attachmentTicket.Type)
		if priorityLevel == maxAttacherPriorityLevel {
			highPriorityTicketCandidates = append(highPriorityTicketCandidates, attachmentTicket)
		}
	}

	// TODO: sort by time

	// sort by name
	if len(highPriorityTicketCandidates) == 0 {
		return nil
	}
	shortestNameAttachmentTicket := highPriorityTicketCandidates[0]
	for _, attachmentTicket := range highPriorityTicketCandidates {
		if attachmentTicket.ID < shortestNameAttachmentTicket.ID {
			shortestNameAttachmentTicket = attachmentTicket
		}
	}

	return shortestNameAttachmentTicket
}

func (vac *VolumeAttachmentController) handleVAStatusUpdate(va *longhorn.VolumeAttachment, vol *longhorn.Volume) error {
	if va.Status.AttachmentTicketStatuses == nil {
		va.Status.AttachmentTicketStatuses = make(map[string]*longhorn.AttachmentTicketStatus)
	}

	// Attachment ticket that desires detaching
	for _, attachmentTicketStatus := range va.Status.AttachmentTicketStatuses {
		if _, ok := va.Spec.AttachmentTickets[attachmentTicketStatus.ID]; !ok {
			vac.updateStatusForDesiredDetachingAttachmentTicket(attachmentTicketStatus.ID, va)
		}
	}

	// Attachment that requests to attach
	for _, attachmentTicket := range va.Spec.AttachmentTickets {
		vac.updateStatusForDesiredAttachingAttachmentTicket(attachmentTicket.ID, va, vol)
	}
	return nil
}

func (vac *VolumeAttachmentController) updateStatusForDesiredDetachingAttachmentTicket(attachmentTicketID string, va *longhorn.VolumeAttachment) {
	delete(va.Status.AttachmentTicketStatuses, attachmentTicketID)
}

func (vac *VolumeAttachmentController) updateStatusForDesiredAttachingAttachmentTicket(attachmentTicketID string, va *longhorn.VolumeAttachment, vol *longhorn.Volume) {
	log := getLoggerForLHVolumeAttachment(vac.logger, va)

	if _, ok := va.Status.AttachmentTicketStatuses[attachmentTicketID]; !ok {
		va.Status.AttachmentTicketStatuses[attachmentTicketID] = &longhorn.AttachmentTicketStatus{
			ID: attachmentTicketID,
			// TODO: handle condition initialization here
		}
	}

	attachmentTicket := va.Spec.AttachmentTickets[attachmentTicketID]
	attachmentTicketStatus := va.Status.AttachmentTicketStatuses[attachmentTicketID]

	defer func() {
		attachmentTicketStatus.Generation = attachmentTicket.Generation
	}()

	if isCSIAttacherTicketOfRegularRWXVolume(attachmentTicket, vol) {
		if isVolumeShareAvailable(vol) {
			attachmentTicketStatus.Satisfied = true
			attachmentTicketStatus.Conditions = types.SetCondition(
				attachmentTicketStatus.Conditions,
				longhorn.AttachmentStatusConditionTypeSatisfied,
				longhorn.ConditionStatusTrue,
				"",
				"The attachment ticket is satisfied",
			)
			return
		}
		attachmentTicketStatus.Satisfied = false
		attachmentTicketStatus.Conditions = types.SetCondition(
			attachmentTicketStatus.Conditions,
			longhorn.AttachmentStatusConditionTypeSatisfied,
			longhorn.ConditionStatusFalse,
			"",
			"Waiting for volume share to be available",
		)

		return
	}

	if isMigratingCSIAttacherTicket(attachmentTicket, vol) {
		if vac.isVolumeAvailableOnNode(vol.Name, attachmentTicket.NodeID) {
			attachmentTicketStatus.Satisfied = true
			attachmentTicketStatus.Conditions = types.SetCondition(
				attachmentTicketStatus.Conditions,
				longhorn.AttachmentStatusConditionTypeSatisfied,
				longhorn.ConditionStatusTrue,
				"",
				"The migrating attachment ticket is satisfied",
			)
			return
		}
		attachmentTicketStatus.Satisfied = false
		attachmentTicketStatus.Conditions = types.SetCondition(
			attachmentTicketStatus.Conditions,
			longhorn.AttachmentStatusConditionTypeSatisfied,
			longhorn.ConditionStatusFalse,
			"",
			fmt.Sprintf("waiting for volume to migrate to node %v", attachmentTicket.NodeID),
		)

		return
	}

	if vol.Status.CurrentNodeID == "" || vol.Status.State != longhorn.VolumeStateAttached {
		attachmentTicketStatus.Satisfied = false
		attachmentTicketStatus.Conditions = types.SetCondition(
			attachmentTicketStatus.Conditions,
			longhorn.AttachmentStatusConditionTypeSatisfied,
			longhorn.ConditionStatusFalse,
			"",
			"",
		)

		// TODO: check if the engine image is ready on the node
		// check if the node is down
		// to set the condition for the client to consume
		return
	}

	if attachmentTicket.NodeID != vol.Status.CurrentNodeID {
		attachmentTicketStatus.Satisfied = false
		attachmentTicketStatus.Conditions = types.SetCondition(
			attachmentTicketStatus.Conditions,
			longhorn.AttachmentStatusConditionTypeSatisfied,
			longhorn.ConditionStatusFalse,
			"",
			fmt.Sprintf("the volume is currently attached to different node %v ", vol.Status.CurrentNodeID),
		)
		return
	}

	if vol.Status.CurrentNodeID == attachmentTicket.NodeID && vol.Status.State == longhorn.VolumeStateAttached {
		if !verifyAttachmentParameters(attachmentTicket.Parameters, vol) {
			attachmentTicketStatus.Satisfied = false
			cond := types.GetCondition(attachmentTicketStatus.Conditions, longhorn.AttachmentStatusConditionTypeSatisfied)
			if cond.Reason != longhorn.AttachmentStatusConditionReasonAttachedWithIncompatibleParameters {
				log.Warnf("Volume %v has already attached to node %v with incompatible parameters", vol.Name, vol.Status.CurrentNodeID)
			}
			attachmentTicketStatus.Conditions = types.SetCondition(
				attachmentTicketStatus.Conditions,
				longhorn.AttachmentStatusConditionTypeSatisfied,
				longhorn.ConditionStatusFalse,
				longhorn.AttachmentStatusConditionReasonAttachedWithIncompatibleParameters,
				fmt.Sprintf("volume %v has already attached to node %v with incompatible parameters", vol.Name, vol.Status.CurrentNodeID),
			)
			return
		}
		attachmentTicketStatus.Satisfied = true
		attachmentTicketStatus.Conditions = types.SetCondition(
			attachmentTicketStatus.Conditions,
			longhorn.AttachmentStatusConditionTypeSatisfied,
			longhorn.ConditionStatusTrue,
			"",
			"",
		)
	}
}

func verifyAttachmentParameters(parameters map[string]string, vol *longhorn.Volume) bool {
	disableFrontendString, ok := parameters["disableFrontend"]
	if !ok || disableFrontendString == longhorn.FalseValue {
		return vol.Spec.DisableFrontend == false
	} else if disableFrontendString == longhorn.TrueValue {
		return vol.Spec.DisableFrontend == true
	}
	return true
}

func setAttachmentParameter(parameters map[string]string, vol *longhorn.Volume) {
	disableFrontendString, ok := parameters["disableFrontend"]
	if !ok || disableFrontendString == longhorn.FalseValue {
		vol.Spec.DisableFrontend = false
	} else if disableFrontendString == longhorn.TrueValue {
		vol.Spec.DisableFrontend = true
	}
	vol.Spec.LastAttachedBy = parameters["lastAttachedBy"]
}

func (vac *VolumeAttachmentController) isResponsibleFor(va *longhorn.VolumeAttachment) (bool, error) {
	var err error
	defer func() {
		err = errors.Wrap(err, "error while checking isResponsibleFor")
	}()

	volume, err := vac.ds.GetVolumeRO(va.Spec.Volume)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return vac.controllerID == volume.Status.OwnerID, nil
}

func getLoggerForLHVolumeAttachment(logger logrus.FieldLogger, va *longhorn.VolumeAttachment) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"longhornVolumeAttachment": va.Name,
		},
	)
}

func isCSIAttacherTicketOfRegularRWXVolume(attachmentTicket *longhorn.AttachmentTicket, v *longhorn.Volume) bool {
	return isRegularRWXVolume(v) && isCSIAttacherTicket(attachmentTicket)
}

func isRegularRWXVolume(v *longhorn.Volume) bool {
	if v == nil {
		return false
	}
	return v.Spec.AccessMode == longhorn.AccessModeReadWriteMany && !v.Spec.Migratable
}

func isCSIAttacherTicket(ticket *longhorn.AttachmentTicket) bool {
	if ticket == nil {
		return false
	}
	return ticket.Type == longhorn.AttacherTypeCSIAttacher
}

func isMigratingCSIAttacherTicket(attachmentTicket *longhorn.AttachmentTicket, vol *longhorn.Volume) bool {
	if attachmentTicket == nil || vol == nil {
		return false
	}
	isCSIAttacherTicket := attachmentTicket.Type == longhorn.AttacherTypeCSIAttacher
	isMigratingTicket := attachmentTicket.NodeID == vol.Status.CurrentMigrationNodeID
	return isMigratableVolume(vol) && isVolumeMigrating(vol) && isCSIAttacherTicket && isMigratingTicket
}

func isVolumeShareAvailable(vol *longhorn.Volume) bool {
	return vol.Spec.AccessMode == longhorn.AccessModeReadWriteMany &&
		vol.Status.ShareState == longhorn.ShareManagerStateRunning &&
		vol.Status.ShareEndpoint != ""
}

func (vac *VolumeAttachmentController) isVolumeAvailableOnNode(volumeName, node string) bool {
	es, _ := vac.ds.ListVolumeEngines(volumeName)
	for _, e := range es {
		if e.Spec.NodeID != node {
			continue
		}
		if e.DeletionTimestamp != nil {
			continue
		}
		if e.Spec.DesireState != longhorn.InstanceStateRunning || e.Status.CurrentState != longhorn.InstanceStateRunning {
			continue
		}
		hasAvailableReplica := false
		for _, mode := range e.Status.ReplicaModeMap {
			hasAvailableReplica = hasAvailableReplica || mode == longhorn.ReplicaModeRW
		}
		if !hasAvailableReplica {
			continue
		}
		return true
	}

	return false
}
