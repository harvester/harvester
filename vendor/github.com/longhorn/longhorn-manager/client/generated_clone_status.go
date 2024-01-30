package client

const (
	CLONE_STATUS_TYPE = "cloneStatus"
)

type CloneStatus struct {
	Resource `yaml:"-"`

	Snapshot string `json:"snapshot,omitempty" yaml:"snapshot,omitempty"`

	SourceVolume string `json:"sourceVolume,omitempty" yaml:"source_volume,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`
}

type CloneStatusCollection struct {
	Collection
	Data   []CloneStatus `json:"data,omitempty"`
	client *CloneStatusClient
}

type CloneStatusClient struct {
	rancherClient *RancherClient
}

type CloneStatusOperations interface {
	List(opts *ListOpts) (*CloneStatusCollection, error)
	Create(opts *CloneStatus) (*CloneStatus, error)
	Update(existing *CloneStatus, updates interface{}) (*CloneStatus, error)
	ById(id string) (*CloneStatus, error)
	Delete(container *CloneStatus) error
}

func newCloneStatusClient(rancherClient *RancherClient) *CloneStatusClient {
	return &CloneStatusClient{
		rancherClient: rancherClient,
	}
}

func (c *CloneStatusClient) Create(container *CloneStatus) (*CloneStatus, error) {
	resp := &CloneStatus{}
	err := c.rancherClient.doCreate(CLONE_STATUS_TYPE, container, resp)
	return resp, err
}

func (c *CloneStatusClient) Update(existing *CloneStatus, updates interface{}) (*CloneStatus, error) {
	resp := &CloneStatus{}
	err := c.rancherClient.doUpdate(CLONE_STATUS_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *CloneStatusClient) List(opts *ListOpts) (*CloneStatusCollection, error) {
	resp := &CloneStatusCollection{}
	err := c.rancherClient.doList(CLONE_STATUS_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *CloneStatusCollection) Next() (*CloneStatusCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &CloneStatusCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *CloneStatusClient) ById(id string) (*CloneStatus, error) {
	resp := &CloneStatus{}
	err := c.rancherClient.doById(CLONE_STATUS_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *CloneStatusClient) Delete(container *CloneStatus) error {
	return c.rancherClient.doResourceDelete(CLONE_STATUS_TYPE, &container.Resource)
}
