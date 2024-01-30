package client

const (
	BACKUP_STATUS_TYPE = "backupStatus"
)

type BackupStatus struct {
	Resource `yaml:"-"`

	BackupURL string `json:"backupURL,omitempty" yaml:"backup_url,omitempty"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	Progress int64 `json:"progress,omitempty" yaml:"progress,omitempty"`

	Replica string `json:"replica,omitempty" yaml:"replica,omitempty"`

	Size string `json:"size,omitempty" yaml:"size,omitempty"`

	Snapshot string `json:"snapshot,omitempty" yaml:"snapshot,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`
}

type BackupStatusCollection struct {
	Collection
	Data   []BackupStatus `json:"data,omitempty"`
	client *BackupStatusClient
}

type BackupStatusClient struct {
	rancherClient *RancherClient
}

type BackupStatusOperations interface {
	List(opts *ListOpts) (*BackupStatusCollection, error)
	Create(opts *BackupStatus) (*BackupStatus, error)
	Update(existing *BackupStatus, updates interface{}) (*BackupStatus, error)
	ById(id string) (*BackupStatus, error)
	Delete(container *BackupStatus) error
}

func newBackupStatusClient(rancherClient *RancherClient) *BackupStatusClient {
	return &BackupStatusClient{
		rancherClient: rancherClient,
	}
}

func (c *BackupStatusClient) Create(container *BackupStatus) (*BackupStatus, error) {
	resp := &BackupStatus{}
	err := c.rancherClient.doCreate(BACKUP_STATUS_TYPE, container, resp)
	return resp, err
}

func (c *BackupStatusClient) Update(existing *BackupStatus, updates interface{}) (*BackupStatus, error) {
	resp := &BackupStatus{}
	err := c.rancherClient.doUpdate(BACKUP_STATUS_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *BackupStatusClient) List(opts *ListOpts) (*BackupStatusCollection, error) {
	resp := &BackupStatusCollection{}
	err := c.rancherClient.doList(BACKUP_STATUS_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *BackupStatusCollection) Next() (*BackupStatusCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &BackupStatusCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *BackupStatusClient) ById(id string) (*BackupStatus, error) {
	resp := &BackupStatus{}
	err := c.rancherClient.doById(BACKUP_STATUS_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *BackupStatusClient) Delete(container *BackupStatus) error {
	return c.rancherClient.doResourceDelete(BACKUP_STATUS_TYPE, &container.Resource)
}
