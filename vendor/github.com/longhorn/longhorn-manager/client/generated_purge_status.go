package client

const (
	PURGE_STATUS_TYPE = "purgeStatus"
)

type PurgeStatus struct {
	Resource `yaml:"-"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	IsPurging bool `json:"isPurging,omitempty" yaml:"is_purging,omitempty"`

	Progress int64 `json:"progress,omitempty" yaml:"progress,omitempty"`

	Replica string `json:"replica,omitempty" yaml:"replica,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`
}

type PurgeStatusCollection struct {
	Collection
	Data   []PurgeStatus `json:"data,omitempty"`
	client *PurgeStatusClient
}

type PurgeStatusClient struct {
	rancherClient *RancherClient
}

type PurgeStatusOperations interface {
	List(opts *ListOpts) (*PurgeStatusCollection, error)
	Create(opts *PurgeStatus) (*PurgeStatus, error)
	Update(existing *PurgeStatus, updates interface{}) (*PurgeStatus, error)
	ById(id string) (*PurgeStatus, error)
	Delete(container *PurgeStatus) error
}

func newPurgeStatusClient(rancherClient *RancherClient) *PurgeStatusClient {
	return &PurgeStatusClient{
		rancherClient: rancherClient,
	}
}

func (c *PurgeStatusClient) Create(container *PurgeStatus) (*PurgeStatus, error) {
	resp := &PurgeStatus{}
	err := c.rancherClient.doCreate(PURGE_STATUS_TYPE, container, resp)
	return resp, err
}

func (c *PurgeStatusClient) Update(existing *PurgeStatus, updates interface{}) (*PurgeStatus, error) {
	resp := &PurgeStatus{}
	err := c.rancherClient.doUpdate(PURGE_STATUS_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *PurgeStatusClient) List(opts *ListOpts) (*PurgeStatusCollection, error) {
	resp := &PurgeStatusCollection{}
	err := c.rancherClient.doList(PURGE_STATUS_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *PurgeStatusCollection) Next() (*PurgeStatusCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &PurgeStatusCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *PurgeStatusClient) ById(id string) (*PurgeStatus, error) {
	resp := &PurgeStatus{}
	err := c.rancherClient.doById(PURGE_STATUS_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *PurgeStatusClient) Delete(container *PurgeStatus) error {
	return c.rancherClient.doResourceDelete(PURGE_STATUS_TYPE, &container.Resource)
}
