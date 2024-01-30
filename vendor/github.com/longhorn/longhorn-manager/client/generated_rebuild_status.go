package client

const (
	REBUILD_STATUS_TYPE = "rebuildStatus"
)

type RebuildStatus struct {
	Resource `yaml:"-"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	FromReplica string `json:"fromReplica,omitempty" yaml:"from_replica,omitempty"`

	IsRebuilding bool `json:"isRebuilding,omitempty" yaml:"is_rebuilding,omitempty"`

	Progress int64 `json:"progress,omitempty" yaml:"progress,omitempty"`

	Replica string `json:"replica,omitempty" yaml:"replica,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`
}

type RebuildStatusCollection struct {
	Collection
	Data   []RebuildStatus `json:"data,omitempty"`
	client *RebuildStatusClient
}

type RebuildStatusClient struct {
	rancherClient *RancherClient
}

type RebuildStatusOperations interface {
	List(opts *ListOpts) (*RebuildStatusCollection, error)
	Create(opts *RebuildStatus) (*RebuildStatus, error)
	Update(existing *RebuildStatus, updates interface{}) (*RebuildStatus, error)
	ById(id string) (*RebuildStatus, error)
	Delete(container *RebuildStatus) error
}

func newRebuildStatusClient(rancherClient *RancherClient) *RebuildStatusClient {
	return &RebuildStatusClient{
		rancherClient: rancherClient,
	}
}

func (c *RebuildStatusClient) Create(container *RebuildStatus) (*RebuildStatus, error) {
	resp := &RebuildStatus{}
	err := c.rancherClient.doCreate(REBUILD_STATUS_TYPE, container, resp)
	return resp, err
}

func (c *RebuildStatusClient) Update(existing *RebuildStatus, updates interface{}) (*RebuildStatus, error) {
	resp := &RebuildStatus{}
	err := c.rancherClient.doUpdate(REBUILD_STATUS_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *RebuildStatusClient) List(opts *ListOpts) (*RebuildStatusCollection, error) {
	resp := &RebuildStatusCollection{}
	err := c.rancherClient.doList(REBUILD_STATUS_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *RebuildStatusCollection) Next() (*RebuildStatusCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &RebuildStatusCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *RebuildStatusClient) ById(id string) (*RebuildStatus, error) {
	resp := &RebuildStatus{}
	err := c.rancherClient.doById(REBUILD_STATUS_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *RebuildStatusClient) Delete(container *RebuildStatus) error {
	return c.rancherClient.doResourceDelete(REBUILD_STATUS_TYPE, &container.Resource)
}
