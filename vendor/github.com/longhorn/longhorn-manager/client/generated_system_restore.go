package client

const (
	SYSTEM_RESTORE_TYPE = "systemRestore"
)

type SystemRestore struct {
	Resource `yaml:"-"`

	CreatedAt string `json:"createdAt,omitempty" yaml:"created_at,omitempty"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`

	SystemBackup string `json:"systemBackup,omitempty" yaml:"system_backup,omitempty"`
}

type SystemRestoreCollection struct {
	Collection
	Data   []SystemRestore `json:"data,omitempty"`
	client *SystemRestoreClient
}

type SystemRestoreClient struct {
	rancherClient *RancherClient
}

type SystemRestoreOperations interface {
	List(opts *ListOpts) (*SystemRestoreCollection, error)
	Create(opts *SystemRestore) (*SystemRestore, error)
	Update(existing *SystemRestore, updates interface{}) (*SystemRestore, error)
	ById(id string) (*SystemRestore, error)
	Delete(container *SystemRestore) error
}

func newSystemRestoreClient(rancherClient *RancherClient) *SystemRestoreClient {
	return &SystemRestoreClient{
		rancherClient: rancherClient,
	}
}

func (c *SystemRestoreClient) Create(container *SystemRestore) (*SystemRestore, error) {
	resp := &SystemRestore{}
	err := c.rancherClient.doCreate(SYSTEM_RESTORE_TYPE, container, resp)
	return resp, err
}

func (c *SystemRestoreClient) Update(existing *SystemRestore, updates interface{}) (*SystemRestore, error) {
	resp := &SystemRestore{}
	err := c.rancherClient.doUpdate(SYSTEM_RESTORE_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SystemRestoreClient) List(opts *ListOpts) (*SystemRestoreCollection, error) {
	resp := &SystemRestoreCollection{}
	err := c.rancherClient.doList(SYSTEM_RESTORE_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SystemRestoreCollection) Next() (*SystemRestoreCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SystemRestoreCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SystemRestoreClient) ById(id string) (*SystemRestore, error) {
	resp := &SystemRestore{}
	err := c.rancherClient.doById(SYSTEM_RESTORE_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SystemRestoreClient) Delete(container *SystemRestore) error {
	return c.rancherClient.doResourceDelete(SYSTEM_RESTORE_TYPE, &container.Resource)
}
