package client

const (
	BACKUP_TYPE = "backup"
)

type Backup struct {
	Resource `yaml:"-"`

	CompressionMethod string `json:"compressionMethod,omitempty" yaml:"compression_method,omitempty"`

	Created string `json:"created,omitempty" yaml:"created,omitempty"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	Messages map[string]string `json:"messages,omitempty" yaml:"messages,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	Progress int64 `json:"progress,omitempty" yaml:"progress,omitempty"`

	Size string `json:"size,omitempty" yaml:"size,omitempty"`

	SnapshotCreated string `json:"snapshotCreated,omitempty" yaml:"snapshot_created,omitempty"`

	SnapshotName string `json:"snapshotName,omitempty" yaml:"snapshot_name,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`

	Url string `json:"url,omitempty" yaml:"url,omitempty"`

	VolumeBackingImageName string `json:"volumeBackingImageName,omitempty" yaml:"volume_backing_image_name,omitempty"`

	VolumeCreated string `json:"volumeCreated,omitempty" yaml:"volume_created,omitempty"`

	VolumeName string `json:"volumeName,omitempty" yaml:"volume_name,omitempty"`

	VolumeSize string `json:"volumeSize,omitempty" yaml:"volume_size,omitempty"`
}

type BackupCollection struct {
	Collection
	Data   []Backup `json:"data,omitempty"`
	client *BackupClient
}

type BackupClient struct {
	rancherClient *RancherClient
}

type BackupOperations interface {
	List(opts *ListOpts) (*BackupCollection, error)
	Create(opts *Backup) (*Backup, error)
	Update(existing *Backup, updates interface{}) (*Backup, error)
	ById(id string) (*Backup, error)
	Delete(container *Backup) error
}

func newBackupClient(rancherClient *RancherClient) *BackupClient {
	return &BackupClient{
		rancherClient: rancherClient,
	}
}

func (c *BackupClient) Create(container *Backup) (*Backup, error) {
	resp := &Backup{}
	err := c.rancherClient.doCreate(BACKUP_TYPE, container, resp)
	return resp, err
}

func (c *BackupClient) Update(existing *Backup, updates interface{}) (*Backup, error) {
	resp := &Backup{}
	err := c.rancherClient.doUpdate(BACKUP_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *BackupClient) List(opts *ListOpts) (*BackupCollection, error) {
	resp := &BackupCollection{}
	err := c.rancherClient.doList(BACKUP_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *BackupCollection) Next() (*BackupCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &BackupCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *BackupClient) ById(id string) (*Backup, error) {
	resp := &Backup{}
	err := c.rancherClient.doById(BACKUP_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *BackupClient) Delete(container *Backup) error {
	return c.rancherClient.doResourceDelete(BACKUP_TYPE, &container.Resource)
}
