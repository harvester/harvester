package client

const (
	VOLUME_TYPE = "volume"
)

type Volume struct {
	Resource `yaml:"-"`

	AccessMode string `json:"accessMode,omitempty" yaml:"access_mode,omitempty"`

	BackendStoreDriver string `json:"backendStoreDriver,omitempty" yaml:"backend_store_driver,omitempty"`

	BackingImage string `json:"backingImage,omitempty" yaml:"backing_image,omitempty"`

	BackupCompressionMethod string `json:"backupCompressionMethod,omitempty" yaml:"backup_compression_method,omitempty"`

	BackupStatus []BackupStatus `json:"backupStatus,omitempty" yaml:"backup_status,omitempty"`

	CloneStatus CloneStatus `json:"cloneStatus,omitempty" yaml:"clone_status,omitempty"`

	Conditions map[string]interface{} `json:"conditions,omitempty" yaml:"conditions,omitempty"`

	Controllers []Controller `json:"controllers,omitempty" yaml:"controllers,omitempty"`

	Created string `json:"created,omitempty" yaml:"created,omitempty"`

	CurrentImage string `json:"currentImage,omitempty" yaml:"current_image,omitempty"`

	DataLocality string `json:"dataLocality,omitempty" yaml:"data_locality,omitempty"`

	DataSource string `json:"dataSource,omitempty" yaml:"data_source,omitempty"`

	DisableFrontend bool `json:"disableFrontend,omitempty" yaml:"disable_frontend,omitempty"`

	DiskSelector []string `json:"diskSelector,omitempty" yaml:"disk_selector,omitempty"`

	Encrypted bool `json:"encrypted,omitempty" yaml:"encrypted,omitempty"`

	EngineImage string `json:"engineImage,omitempty" yaml:"engine_image,omitempty"`

	FromBackup string `json:"fromBackup,omitempty" yaml:"from_backup,omitempty"`

	Frontend string `json:"frontend,omitempty" yaml:"frontend,omitempty"`

	KubernetesStatus KubernetesStatus `json:"kubernetesStatus,omitempty" yaml:"kubernetes_status,omitempty"`

	LastAttachedBy string `json:"lastAttachedBy,omitempty" yaml:"last_attached_by,omitempty"`

	LastBackup string `json:"lastBackup,omitempty" yaml:"last_backup,omitempty"`

	LastBackupAt string `json:"lastBackupAt,omitempty" yaml:"last_backup_at,omitempty"`

	Migratable bool `json:"migratable,omitempty" yaml:"migratable,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	NodeSelector []string `json:"nodeSelector,omitempty" yaml:"node_selector,omitempty"`

	NumberOfReplicas int64 `json:"numberOfReplicas,omitempty" yaml:"number_of_replicas,omitempty"`

	OfflineReplicaRebuilding string `json:"offlineReplicaRebuilding,omitempty" yaml:"offline_replica_rebuilding,omitempty"`

	OfflineReplicaRebuildingRequired bool `json:"offlineReplicaRebuildingRequired,omitempty" yaml:"offline_replica_rebuilding_required,omitempty"`

	PurgeStatus []PurgeStatus `json:"purgeStatus,omitempty" yaml:"purge_status,omitempty"`

	Ready bool `json:"ready,omitempty" yaml:"ready,omitempty"`

	RebuildStatus []RebuildStatus `json:"rebuildStatus,omitempty" yaml:"rebuild_status,omitempty"`

	RecurringJobSelector []VolumeRecurringJob `json:"recurringJobSelector,omitempty" yaml:"recurring_job_selector,omitempty"`

	ReplicaAutoBalance string `json:"replicaAutoBalance,omitempty" yaml:"replica_auto_balance,omitempty"`

	ReplicaSoftAntiAffinity string `json:"replicaSoftAntiAffinity,omitempty" yaml:"replica_soft_anti_affinity,omitempty"`

	ReplicaZoneSoftAntiAffinity string `json:"replicaZoneSoftAntiAffinity,omitempty" yaml:"replica_zone_soft_anti_affinity,omitempty"`

	Replicas []Replica `json:"replicas,omitempty" yaml:"replicas,omitempty"`

	RestoreInitiated bool `json:"restoreInitiated,omitempty" yaml:"restore_initiated,omitempty"`

	RestoreRequired bool `json:"restoreRequired,omitempty" yaml:"restore_required,omitempty"`

	RestoreStatus []RestoreStatus `json:"restoreStatus,omitempty" yaml:"restore_status,omitempty"`

	RestoreVolumeRecurringJob string `json:"restoreVolumeRecurringJob,omitempty" yaml:"restore_volume_recurring_job,omitempty"`

	RevisionCounterDisabled bool `json:"revisionCounterDisabled,omitempty" yaml:"revision_counter_disabled,omitempty"`

	Robustness string `json:"robustness,omitempty" yaml:"robustness,omitempty"`

	ShareEndpoint string `json:"shareEndpoint,omitempty" yaml:"share_endpoint,omitempty"`

	ShareState string `json:"shareState,omitempty" yaml:"share_state,omitempty"`

	Size string `json:"size,omitempty" yaml:"size,omitempty"`

	SnapshotDataIntegrity string `json:"snapshotDataIntegrity,omitempty" yaml:"snapshot_data_integrity,omitempty"`

	StaleReplicaTimeout int64 `json:"staleReplicaTimeout,omitempty" yaml:"stale_replica_timeout,omitempty"`

	Standby bool `json:"standby,omitempty" yaml:"standby,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`

	UnmapMarkSnapChainRemoved string `json:"unmapMarkSnapChainRemoved,omitempty" yaml:"unmap_mark_snap_chain_removed,omitempty"`

	VolumeAttachment VolumeAttachment `json:"volumeAttachment,omitempty" yaml:"volume_attachment,omitempty"`
}

type VolumeCollection struct {
	Collection
	Data   []Volume `json:"data,omitempty"`
	client *VolumeClient
}

type VolumeClient struct {
	rancherClient *RancherClient
}

type VolumeOperations interface {
	List(opts *ListOpts) (*VolumeCollection, error)
	Create(opts *Volume) (*Volume, error)
	Update(existing *Volume, updates interface{}) (*Volume, error)
	ById(id string) (*Volume, error)
	Delete(container *Volume) error

	ActionActivate(*Volume, *ActivateInput) (*Volume, error)

	ActionAttach(*Volume, *AttachInput) (*Volume, error)

	ActionCancelExpansion(*Volume) (*Volume, error)

	ActionDetach(*Volume, *DetachInput) (*Volume, error)

	ActionExpand(*Volume, *ExpandInput) (*Volume, error)

	ActionPvCreate(*Volume, *PVCreateInput) (*Volume, error)

	ActionPvcCreate(*Volume, *PVCCreateInput) (*Volume, error)

	ActionRecurringJobAdd(*Volume, *VolumeRecurringJobInput) (*VolumeRecurringJob, error)

	ActionRecurringJobDelete(*Volume, *VolumeRecurringJobInput) (*VolumeRecurringJob, error)

	ActionRecurringJobList(*Volume) (*VolumeRecurringJob, error)

	ActionReplicaRemove(*Volume, *ReplicaRemoveInput) (*Volume, error)

	ActionSalvage(*Volume, *SalvageInput) (*Volume, error)

	ActionSnapshotBackup(*Volume, *SnapshotInput) (*Volume, error)

	ActionSnapshotCRCreate(*Volume, *SnapshotCRInput) (*SnapshotCR, error)

	ActionSnapshotCRDelete(*Volume, *SnapshotCRInput) (*Empty, error)

	ActionSnapshotCRGet(*Volume, *SnapshotCRInput) (*SnapshotCR, error)

	ActionSnapshotCRList(*Volume) (*SnapshotCRListOutput, error)

	ActionSnapshotCreate(*Volume, *SnapshotInput) (*Snapshot, error)

	ActionSnapshotDelete(*Volume, *SnapshotInput) (*Volume, error)

	ActionSnapshotGet(*Volume, *SnapshotInput) (*Snapshot, error)

	ActionSnapshotList(*Volume) (*SnapshotListOutput, error)

	ActionSnapshotPurge(*Volume) (*Volume, error)

	ActionSnapshotRevert(*Volume, *SnapshotInput) (*Snapshot, error)

	ActionTrimFilesystem(*Volume) (*Volume, error)

	ActionUpdateAccessMode(*Volume, *UpdateAccessModeInput) (*Volume, error)
}

func newVolumeClient(rancherClient *RancherClient) *VolumeClient {
	return &VolumeClient{
		rancherClient: rancherClient,
	}
}

func (c *VolumeClient) Create(container *Volume) (*Volume, error) {
	resp := &Volume{}
	err := c.rancherClient.doCreate(VOLUME_TYPE, container, resp)
	return resp, err
}

func (c *VolumeClient) Update(existing *Volume, updates interface{}) (*Volume, error) {
	resp := &Volume{}
	err := c.rancherClient.doUpdate(VOLUME_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *VolumeClient) List(opts *ListOpts) (*VolumeCollection, error) {
	resp := &VolumeCollection{}
	err := c.rancherClient.doList(VOLUME_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *VolumeCollection) Next() (*VolumeCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &VolumeCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *VolumeClient) ById(id string) (*Volume, error) {
	resp := &Volume{}
	err := c.rancherClient.doById(VOLUME_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *VolumeClient) Delete(container *Volume) error {
	return c.rancherClient.doResourceDelete(VOLUME_TYPE, &container.Resource)
}

func (c *VolumeClient) ActionActivate(resource *Volume, input *ActivateInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "activate", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionAttach(resource *Volume, input *AttachInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "attach", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionCancelExpansion(resource *Volume) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "cancelExpansion", &resource.Resource, nil, resp)

	return resp, err
}

func (c *VolumeClient) ActionDetach(resource *Volume, input *DetachInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "detach", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionExpand(resource *Volume, input *ExpandInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "expand", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionPvCreate(resource *Volume, input *PVCreateInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "pvCreate", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionPvcCreate(resource *Volume, input *PVCCreateInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "pvcCreate", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionRecurringJobAdd(resource *Volume, input *VolumeRecurringJobInput) (*VolumeRecurringJob, error) {

	resp := &VolumeRecurringJob{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "recurringJobAdd", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionRecurringJobDelete(resource *Volume, input *VolumeRecurringJobInput) (*VolumeRecurringJob, error) {

	resp := &VolumeRecurringJob{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "recurringJobDelete", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionRecurringJobList(resource *Volume) (*VolumeRecurringJob, error) {

	resp := &VolumeRecurringJob{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "recurringJobList", &resource.Resource, nil, resp)

	return resp, err
}

func (c *VolumeClient) ActionReplicaRemove(resource *Volume, input *ReplicaRemoveInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "replicaRemove", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionSalvage(resource *Volume, input *SalvageInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "salvage", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotBackup(resource *Volume, input *SnapshotInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotBackup", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotCRCreate(resource *Volume, input *SnapshotCRInput) (*SnapshotCR, error) {

	resp := &SnapshotCR{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotCRCreate", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotCRDelete(resource *Volume, input *SnapshotCRInput) (*Empty, error) {

	resp := &Empty{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotCRDelete", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotCRGet(resource *Volume, input *SnapshotCRInput) (*SnapshotCR, error) {

	resp := &SnapshotCR{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotCRGet", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotCRList(resource *Volume) (*SnapshotCRListOutput, error) {

	resp := &SnapshotCRListOutput{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotCRList", &resource.Resource, nil, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotCreate(resource *Volume, input *SnapshotInput) (*Snapshot, error) {

	resp := &Snapshot{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotCreate", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotDelete(resource *Volume, input *SnapshotInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotDelete", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotGet(resource *Volume, input *SnapshotInput) (*Snapshot, error) {

	resp := &Snapshot{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotGet", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotList(resource *Volume) (*SnapshotListOutput, error) {

	resp := &SnapshotListOutput{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotList", &resource.Resource, nil, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotPurge(resource *Volume) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotPurge", &resource.Resource, nil, resp)

	return resp, err
}

func (c *VolumeClient) ActionSnapshotRevert(resource *Volume, input *SnapshotInput) (*Snapshot, error) {

	resp := &Snapshot{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "snapshotRevert", &resource.Resource, input, resp)

	return resp, err
}

func (c *VolumeClient) ActionTrimFilesystem(resource *Volume) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "trimFilesystem", &resource.Resource, nil, resp)

	return resp, err
}

func (c *VolumeClient) ActionUpdateAccessMode(resource *Volume, input *UpdateAccessModeInput) (*Volume, error) {

	resp := &Volume{}

	err := c.rancherClient.doAction(VOLUME_TYPE, "updateAccessMode", &resource.Resource, input, resp)

	return resp, err
}
