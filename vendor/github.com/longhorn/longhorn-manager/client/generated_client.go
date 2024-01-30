package client

type RancherClient struct {
	RancherBaseClient

	ApiVersion                             ApiVersionOperations
	Error                                  ErrorOperations
	AttachInput                            AttachInputOperations
	DetachInput                            DetachInputOperations
	SnapshotInput                          SnapshotInputOperations
	SnapshotCRInput                        SnapshotCRInputOperations
	BackupTarget                           BackupTargetOperations
	Backup                                 BackupOperations
	BackupInput                            BackupInputOperations
	BackupStatus                           BackupStatusOperations
	Orphan                                 OrphanOperations
	RestoreStatus                          RestoreStatusOperations
	PurgeStatus                            PurgeStatusOperations
	RebuildStatus                          RebuildStatusOperations
	ReplicaRemoveInput                     ReplicaRemoveInputOperations
	SalvageInput                           SalvageInputOperations
	ActivateInput                          ActivateInputOperations
	ExpandInput                            ExpandInputOperations
	EngineUpgradeInput                     EngineUpgradeInputOperations
	Replica                                ReplicaOperations
	Controller                             ControllerOperations
	DiskUpdate                             DiskUpdateOperations
	UpdateReplicaCountInput                UpdateReplicaCountInputOperations
	UpdateReplicaAutoBalanceInput          UpdateReplicaAutoBalanceInputOperations
	UpdateDataLocalityInput                UpdateDataLocalityInputOperations
	UpdateAccessModeInput                  UpdateAccessModeInputOperations
	UpdateSnapshotDataIntegrityInput       UpdateSnapshotDataIntegrityInputOperations
	UpdateOfflineReplicaRebuildingInput    UpdateOfflineReplicaRebuildingInputOperations
	UpdateBackupCompressionInput           UpdateBackupCompressionInputOperations
	UpdateUnmapMarkSnapChainRemovedInput   UpdateUnmapMarkSnapChainRemovedInputOperations
	UpdateReplicaSoftAntiAffinityInput     UpdateReplicaSoftAntiAffinityInputOperations
	UpdateReplicaZoneSoftAntiAffinityInput UpdateReplicaZoneSoftAntiAffinityInputOperations
	WorkloadStatus                         WorkloadStatusOperations
	CloneStatus                            CloneStatusOperations
	Empty                                  EmptyOperations
	VolumeRecurringJob                     VolumeRecurringJobOperations
	VolumeRecurringJobInput                VolumeRecurringJobInputOperations
	PVCreateInput                          PVCreateInputOperations
	PVCCreateInput                         PVCCreateInputOperations
	SettingDefinition                      SettingDefinitionOperations
	VolumeCondition                        VolumeConditionOperations
	NodeCondition                          NodeConditionOperations
	DiskCondition                          DiskConditionOperations
	LonghornCondition                      LonghornConditionOperations
	SupportBundle                          SupportBundleOperations
	SupportBundleInitateInput              SupportBundleInitateInputOperations
	Tag                                    TagOperations
	InstanceManager                        InstanceManagerOperations
	BackingImageDiskFileStatus             BackingImageDiskFileStatusOperations
	BackingImageCleanupInput               BackingImageCleanupInputOperations
	Attachment                             AttachmentOperations
	VolumeAttachment                       VolumeAttachmentOperations
	Volume                                 VolumeOperations
	Snapshot                               SnapshotOperations
	SnapshotCR                             SnapshotCROperations
	BackupVolume                           BackupVolumeOperations
	Setting                                SettingOperations
	RecurringJob                           RecurringJobOperations
	EngineImage                            EngineImageOperations
	BackingImage                           BackingImageOperations
	Node                                   NodeOperations
	DiskUpdateInput                        DiskUpdateInputOperations
	DiskInfo                               DiskInfoOperations
	KubernetesStatus                       KubernetesStatusOperations
	BackupListOutput                       BackupListOutputOperations
	SnapshotListOutput                     SnapshotListOutputOperations
	SystemBackup                           SystemBackupOperations
	SystemRestore                          SystemRestoreOperations
	SnapshotCRListOutput                   SnapshotCRListOutputOperations
}

func constructClient(rancherBaseClient *RancherBaseClientImpl) *RancherClient {
	client := &RancherClient{
		RancherBaseClient: rancherBaseClient,
	}

	client.ApiVersion = newApiVersionClient(client)
	client.Error = newErrorClient(client)
	client.AttachInput = newAttachInputClient(client)
	client.DetachInput = newDetachInputClient(client)
	client.SnapshotInput = newSnapshotInputClient(client)
	client.SnapshotCRInput = newSnapshotCRInputClient(client)
	client.BackupTarget = newBackupTargetClient(client)
	client.Backup = newBackupClient(client)
	client.BackupInput = newBackupInputClient(client)
	client.BackupStatus = newBackupStatusClient(client)
	client.Orphan = newOrphanClient(client)
	client.RestoreStatus = newRestoreStatusClient(client)
	client.PurgeStatus = newPurgeStatusClient(client)
	client.RebuildStatus = newRebuildStatusClient(client)
	client.ReplicaRemoveInput = newReplicaRemoveInputClient(client)
	client.SalvageInput = newSalvageInputClient(client)
	client.ActivateInput = newActivateInputClient(client)
	client.ExpandInput = newExpandInputClient(client)
	client.EngineUpgradeInput = newEngineUpgradeInputClient(client)
	client.Replica = newReplicaClient(client)
	client.Controller = newControllerClient(client)
	client.DiskUpdate = newDiskUpdateClient(client)
	client.UpdateReplicaCountInput = newUpdateReplicaCountInputClient(client)
	client.UpdateReplicaAutoBalanceInput = newUpdateReplicaAutoBalanceInputClient(client)
	client.UpdateDataLocalityInput = newUpdateDataLocalityInputClient(client)
	client.UpdateAccessModeInput = newUpdateAccessModeInputClient(client)
	client.UpdateSnapshotDataIntegrityInput = newUpdateSnapshotDataIntegrityInputClient(client)
	client.UpdateOfflineReplicaRebuildingInput = newUpdateOfflineReplicaRebuildingInputClient(client)
	client.UpdateBackupCompressionInput = newUpdateBackupCompressionInputClient(client)
	client.UpdateUnmapMarkSnapChainRemovedInput = newUpdateUnmapMarkSnapChainRemovedInputClient(client)
	client.UpdateReplicaSoftAntiAffinityInput = newUpdateReplicaSoftAntiAffinityInputClient(client)
	client.UpdateReplicaZoneSoftAntiAffinityInput = newUpdateReplicaZoneSoftAntiAffinityInputClient(client)
	client.WorkloadStatus = newWorkloadStatusClient(client)
	client.CloneStatus = newCloneStatusClient(client)
	client.Empty = newEmptyClient(client)
	client.VolumeRecurringJob = newVolumeRecurringJobClient(client)
	client.VolumeRecurringJobInput = newVolumeRecurringJobInputClient(client)
	client.PVCreateInput = newPVCreateInputClient(client)
	client.PVCCreateInput = newPVCCreateInputClient(client)
	client.SettingDefinition = newSettingDefinitionClient(client)
	client.VolumeCondition = newVolumeConditionClient(client)
	client.NodeCondition = newNodeConditionClient(client)
	client.DiskCondition = newDiskConditionClient(client)
	client.LonghornCondition = newLonghornConditionClient(client)
	client.SupportBundle = newSupportBundleClient(client)
	client.SupportBundleInitateInput = newSupportBundleInitateInputClient(client)
	client.Tag = newTagClient(client)
	client.InstanceManager = newInstanceManagerClient(client)
	client.BackingImageDiskFileStatus = newBackingImageDiskFileStatusClient(client)
	client.BackingImageCleanupInput = newBackingImageCleanupInputClient(client)
	client.Attachment = newAttachmentClient(client)
	client.VolumeAttachment = newVolumeAttachmentClient(client)
	client.Volume = newVolumeClient(client)
	client.Snapshot = newSnapshotClient(client)
	client.SnapshotCR = newSnapshotCRClient(client)
	client.BackupVolume = newBackupVolumeClient(client)
	client.Setting = newSettingClient(client)
	client.RecurringJob = newRecurringJobClient(client)
	client.EngineImage = newEngineImageClient(client)
	client.BackingImage = newBackingImageClient(client)
	client.Node = newNodeClient(client)
	client.DiskUpdateInput = newDiskUpdateInputClient(client)
	client.DiskInfo = newDiskInfoClient(client)
	client.KubernetesStatus = newKubernetesStatusClient(client)
	client.BackupListOutput = newBackupListOutputClient(client)
	client.SnapshotListOutput = newSnapshotListOutputClient(client)
	client.SystemBackup = newSystemBackupClient(client)
	client.SystemRestore = newSystemRestoreClient(client)
	client.SnapshotCRListOutput = newSnapshotCRListOutputClient(client)

	return client
}

func NewRancherClient(opts *ClientOpts) (*RancherClient, error) {
	rancherBaseClient := &RancherBaseClientImpl{
		Types: map[string]Schema{},
	}
	client := constructClient(rancherBaseClient)

	err := setupRancherBaseClient(rancherBaseClient, opts)
	if err != nil {
		return nil, err
	}

	return client, nil
}
