package client

const (
	BACKUP_VOLUME_TYPE = "backupVolume"
)

type BackupVolume struct {
	Resource `yaml:"-"`

	BackingImageChecksum string `json:"backingImageChecksum,omitempty" yaml:"backing_image_checksum,omitempty"`

	BackingImageName string `json:"backingImageName,omitempty" yaml:"backing_image_name,omitempty"`

	Created string `json:"created,omitempty" yaml:"created,omitempty"`

	DataStored string `json:"dataStored,omitempty" yaml:"data_stored,omitempty"`

	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	LastBackupAt string `json:"lastBackupAt,omitempty" yaml:"last_backup_at,omitempty"`

	LastBackupName string `json:"lastBackupName,omitempty" yaml:"last_backup_name,omitempty"`

	Messages map[string]string `json:"messages,omitempty" yaml:"messages,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	Size string `json:"size,omitempty" yaml:"size,omitempty"`

	StorageClassName string `json:"storageClassName,omitempty" yaml:"storage_class_name,omitempty"`
}

type BackupVolumeCollection struct {
	Collection
	Data   []BackupVolume `json:"data,omitempty"`
	client *BackupVolumeClient
}

type BackupVolumeClient struct {
	rancherClient *RancherClient
}

type BackupVolumeOperations interface {
	List(opts *ListOpts) (*BackupVolumeCollection, error)
	Create(opts *BackupVolume) (*BackupVolume, error)
	Update(existing *BackupVolume, updates interface{}) (*BackupVolume, error)
	ById(id string) (*BackupVolume, error)
	Delete(container *BackupVolume) error

	ActionBackupDelete(*BackupVolume, *BackupInput) (*BackupVolume, error)

	ActionBackupGet(*BackupVolume, *BackupInput) (*Backup, error)

	ActionBackupList(*BackupVolume) (*BackupListOutput, error)
}

func newBackupVolumeClient(rancherClient *RancherClient) *BackupVolumeClient {
	return &BackupVolumeClient{
		rancherClient: rancherClient,
	}
}

func (c *BackupVolumeClient) Create(container *BackupVolume) (*BackupVolume, error) {
	resp := &BackupVolume{}
	err := c.rancherClient.doCreate(BACKUP_VOLUME_TYPE, container, resp)
	return resp, err
}

func (c *BackupVolumeClient) Update(existing *BackupVolume, updates interface{}) (*BackupVolume, error) {
	resp := &BackupVolume{}
	err := c.rancherClient.doUpdate(BACKUP_VOLUME_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *BackupVolumeClient) List(opts *ListOpts) (*BackupVolumeCollection, error) {
	resp := &BackupVolumeCollection{}
	err := c.rancherClient.doList(BACKUP_VOLUME_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *BackupVolumeCollection) Next() (*BackupVolumeCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &BackupVolumeCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *BackupVolumeClient) ById(id string) (*BackupVolume, error) {
	resp := &BackupVolume{}
	err := c.rancherClient.doById(BACKUP_VOLUME_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *BackupVolumeClient) Delete(container *BackupVolume) error {
	return c.rancherClient.doResourceDelete(BACKUP_VOLUME_TYPE, &container.Resource)
}

func (c *BackupVolumeClient) ActionBackupDelete(resource *BackupVolume, input *BackupInput) (*BackupVolume, error) {

	resp := &BackupVolume{}

	err := c.rancherClient.doAction(BACKUP_VOLUME_TYPE, "backupDelete", &resource.Resource, input, resp)

	return resp, err
}

func (c *BackupVolumeClient) ActionBackupGet(resource *BackupVolume, input *BackupInput) (*Backup, error) {

	resp := &Backup{}

	err := c.rancherClient.doAction(BACKUP_VOLUME_TYPE, "backupGet", &resource.Resource, input, resp)

	return resp, err
}

func (c *BackupVolumeClient) ActionBackupList(resource *BackupVolume) (*BackupListOutput, error) {

	resp := &BackupListOutput{}

	err := c.rancherClient.doAction(BACKUP_VOLUME_TYPE, "backupList", &resource.Resource, nil, resp)

	return resp, err
}
