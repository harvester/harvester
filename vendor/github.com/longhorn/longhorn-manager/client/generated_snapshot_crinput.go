package client

const (
	SNAPSHOT_CRINPUT_TYPE = "snapshotCRInput"
)

type SnapshotCRInput struct {
	Resource `yaml:"-"`

	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type SnapshotCRInputCollection struct {
	Collection
	Data   []SnapshotCRInput `json:"data,omitempty"`
	client *SnapshotCRInputClient
}

type SnapshotCRInputClient struct {
	rancherClient *RancherClient
}

type SnapshotCRInputOperations interface {
	List(opts *ListOpts) (*SnapshotCRInputCollection, error)
	Create(opts *SnapshotCRInput) (*SnapshotCRInput, error)
	Update(existing *SnapshotCRInput, updates interface{}) (*SnapshotCRInput, error)
	ById(id string) (*SnapshotCRInput, error)
	Delete(container *SnapshotCRInput) error
}

func newSnapshotCRInputClient(rancherClient *RancherClient) *SnapshotCRInputClient {
	return &SnapshotCRInputClient{
		rancherClient: rancherClient,
	}
}

func (c *SnapshotCRInputClient) Create(container *SnapshotCRInput) (*SnapshotCRInput, error) {
	resp := &SnapshotCRInput{}
	err := c.rancherClient.doCreate(SNAPSHOT_CRINPUT_TYPE, container, resp)
	return resp, err
}

func (c *SnapshotCRInputClient) Update(existing *SnapshotCRInput, updates interface{}) (*SnapshotCRInput, error) {
	resp := &SnapshotCRInput{}
	err := c.rancherClient.doUpdate(SNAPSHOT_CRINPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SnapshotCRInputClient) List(opts *ListOpts) (*SnapshotCRInputCollection, error) {
	resp := &SnapshotCRInputCollection{}
	err := c.rancherClient.doList(SNAPSHOT_CRINPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SnapshotCRInputCollection) Next() (*SnapshotCRInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SnapshotCRInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SnapshotCRInputClient) ById(id string) (*SnapshotCRInput, error) {
	resp := &SnapshotCRInput{}
	err := c.rancherClient.doById(SNAPSHOT_CRINPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SnapshotCRInputClient) Delete(container *SnapshotCRInput) error {
	return c.rancherClient.doResourceDelete(SNAPSHOT_CRINPUT_TYPE, &container.Resource)
}
