package client

const (
	SYSTEM_BACKUP_TYPE = "systemBackup"
)

type SystemBackup struct {
	Resource `yaml:"-"`

	CreatedAt string `json:"createdAt,omitempty" yaml:"created_at,omitempty"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	ManagerImage string `json:"managerImage,omitempty" yaml:"manager_image,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`

	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	VolumeBackupPolicy string `json:"volumeBackupPolicy,omitempty" yaml:"volume_backup_policy,omitempty"`
}

type SystemBackupCollection struct {
	Collection
	Data   []SystemBackup `json:"data,omitempty"`
	client *SystemBackupClient
}

type SystemBackupClient struct {
	rancherClient *RancherClient
}

type SystemBackupOperations interface {
	List(opts *ListOpts) (*SystemBackupCollection, error)
	Create(opts *SystemBackup) (*SystemBackup, error)
	Update(existing *SystemBackup, updates interface{}) (*SystemBackup, error)
	ById(id string) (*SystemBackup, error)
	Delete(container *SystemBackup) error
}

func newSystemBackupClient(rancherClient *RancherClient) *SystemBackupClient {
	return &SystemBackupClient{
		rancherClient: rancherClient,
	}
}

func (c *SystemBackupClient) Create(container *SystemBackup) (*SystemBackup, error) {
	resp := &SystemBackup{}
	err := c.rancherClient.doCreate(SYSTEM_BACKUP_TYPE, container, resp)
	return resp, err
}

func (c *SystemBackupClient) Update(existing *SystemBackup, updates interface{}) (*SystemBackup, error) {
	resp := &SystemBackup{}
	err := c.rancherClient.doUpdate(SYSTEM_BACKUP_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SystemBackupClient) List(opts *ListOpts) (*SystemBackupCollection, error) {
	resp := &SystemBackupCollection{}
	err := c.rancherClient.doList(SYSTEM_BACKUP_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SystemBackupCollection) Next() (*SystemBackupCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SystemBackupCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SystemBackupClient) ById(id string) (*SystemBackup, error) {
	resp := &SystemBackup{}
	err := c.rancherClient.doById(SYSTEM_BACKUP_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SystemBackupClient) Delete(container *SystemBackup) error {
	return c.rancherClient.doResourceDelete(SYSTEM_BACKUP_TYPE, &container.Resource)
}
