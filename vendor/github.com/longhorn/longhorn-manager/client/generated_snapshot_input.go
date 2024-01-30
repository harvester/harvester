package client

const (
	SNAPSHOT_INPUT_TYPE = "snapshotInput"
)

type SnapshotInput struct {
	Resource `yaml:"-"`

	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type SnapshotInputCollection struct {
	Collection
	Data   []SnapshotInput `json:"data,omitempty"`
	client *SnapshotInputClient
}

type SnapshotInputClient struct {
	rancherClient *RancherClient
}

type SnapshotInputOperations interface {
	List(opts *ListOpts) (*SnapshotInputCollection, error)
	Create(opts *SnapshotInput) (*SnapshotInput, error)
	Update(existing *SnapshotInput, updates interface{}) (*SnapshotInput, error)
	ById(id string) (*SnapshotInput, error)
	Delete(container *SnapshotInput) error
}

func newSnapshotInputClient(rancherClient *RancherClient) *SnapshotInputClient {
	return &SnapshotInputClient{
		rancherClient: rancherClient,
	}
}

func (c *SnapshotInputClient) Create(container *SnapshotInput) (*SnapshotInput, error) {
	resp := &SnapshotInput{}
	err := c.rancherClient.doCreate(SNAPSHOT_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *SnapshotInputClient) Update(existing *SnapshotInput, updates interface{}) (*SnapshotInput, error) {
	resp := &SnapshotInput{}
	err := c.rancherClient.doUpdate(SNAPSHOT_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SnapshotInputClient) List(opts *ListOpts) (*SnapshotInputCollection, error) {
	resp := &SnapshotInputCollection{}
	err := c.rancherClient.doList(SNAPSHOT_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SnapshotInputCollection) Next() (*SnapshotInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SnapshotInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SnapshotInputClient) ById(id string) (*SnapshotInput, error) {
	resp := &SnapshotInput{}
	err := c.rancherClient.doById(SNAPSHOT_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SnapshotInputClient) Delete(container *SnapshotInput) error {
	return c.rancherClient.doResourceDelete(SNAPSHOT_INPUT_TYPE, &container.Resource)
}
