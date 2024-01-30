package client

const (
	RESTORE_STATUS_TYPE = "restoreStatus"
)

type RestoreStatus struct {
	Resource `yaml:"-"`

	BackupURL string `json:"backupURL,omitempty" yaml:"backup_url,omitempty"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	Filename string `json:"filename,omitempty" yaml:"filename,omitempty"`

	IsRestoring bool `json:"isRestoring,omitempty" yaml:"is_restoring,omitempty"`

	LastRestored string `json:"lastRestored,omitempty" yaml:"last_restored,omitempty"`

	Progress int64 `json:"progress,omitempty" yaml:"progress,omitempty"`

	Replica string `json:"replica,omitempty" yaml:"replica,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`
}

type RestoreStatusCollection struct {
	Collection
	Data   []RestoreStatus `json:"data,omitempty"`
	client *RestoreStatusClient
}

type RestoreStatusClient struct {
	rancherClient *RancherClient
}

type RestoreStatusOperations interface {
	List(opts *ListOpts) (*RestoreStatusCollection, error)
	Create(opts *RestoreStatus) (*RestoreStatus, error)
	Update(existing *RestoreStatus, updates interface{}) (*RestoreStatus, error)
	ById(id string) (*RestoreStatus, error)
	Delete(container *RestoreStatus) error
}

func newRestoreStatusClient(rancherClient *RancherClient) *RestoreStatusClient {
	return &RestoreStatusClient{
		rancherClient: rancherClient,
	}
}

func (c *RestoreStatusClient) Create(container *RestoreStatus) (*RestoreStatus, error) {
	resp := &RestoreStatus{}
	err := c.rancherClient.doCreate(RESTORE_STATUS_TYPE, container, resp)
	return resp, err
}

func (c *RestoreStatusClient) Update(existing *RestoreStatus, updates interface{}) (*RestoreStatus, error) {
	resp := &RestoreStatus{}
	err := c.rancherClient.doUpdate(RESTORE_STATUS_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *RestoreStatusClient) List(opts *ListOpts) (*RestoreStatusCollection, error) {
	resp := &RestoreStatusCollection{}
	err := c.rancherClient.doList(RESTORE_STATUS_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *RestoreStatusCollection) Next() (*RestoreStatusCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &RestoreStatusCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *RestoreStatusClient) ById(id string) (*RestoreStatus, error) {
	resp := &RestoreStatus{}
	err := c.rancherClient.doById(RESTORE_STATUS_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *RestoreStatusClient) Delete(container *RestoreStatus) error {
	return c.rancherClient.doResourceDelete(RESTORE_STATUS_TYPE, &container.Resource)
}
