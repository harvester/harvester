package client

const (
	SNAPSHOT_LIST_OUTPUT_TYPE = "snapshotListOutput"
)

type SnapshotListOutput struct {
	Resource `yaml:"-"`

	Data []Snapshot `json:"data,omitempty" yaml:"data,omitempty"`
}

type SnapshotListOutputCollection struct {
	Collection
	Data   []SnapshotListOutput `json:"data,omitempty"`
	client *SnapshotListOutputClient
}

type SnapshotListOutputClient struct {
	rancherClient *RancherClient
}

type SnapshotListOutputOperations interface {
	List(opts *ListOpts) (*SnapshotListOutputCollection, error)
	Create(opts *SnapshotListOutput) (*SnapshotListOutput, error)
	Update(existing *SnapshotListOutput, updates interface{}) (*SnapshotListOutput, error)
	ById(id string) (*SnapshotListOutput, error)
	Delete(container *SnapshotListOutput) error
}

func newSnapshotListOutputClient(rancherClient *RancherClient) *SnapshotListOutputClient {
	return &SnapshotListOutputClient{
		rancherClient: rancherClient,
	}
}

func (c *SnapshotListOutputClient) Create(container *SnapshotListOutput) (*SnapshotListOutput, error) {
	resp := &SnapshotListOutput{}
	err := c.rancherClient.doCreate(SNAPSHOT_LIST_OUTPUT_TYPE, container, resp)
	return resp, err
}

func (c *SnapshotListOutputClient) Update(existing *SnapshotListOutput, updates interface{}) (*SnapshotListOutput, error) {
	resp := &SnapshotListOutput{}
	err := c.rancherClient.doUpdate(SNAPSHOT_LIST_OUTPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SnapshotListOutputClient) List(opts *ListOpts) (*SnapshotListOutputCollection, error) {
	resp := &SnapshotListOutputCollection{}
	err := c.rancherClient.doList(SNAPSHOT_LIST_OUTPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SnapshotListOutputCollection) Next() (*SnapshotListOutputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SnapshotListOutputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SnapshotListOutputClient) ById(id string) (*SnapshotListOutput, error) {
	resp := &SnapshotListOutput{}
	err := c.rancherClient.doById(SNAPSHOT_LIST_OUTPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SnapshotListOutputClient) Delete(container *SnapshotListOutput) error {
	return c.rancherClient.doResourceDelete(SNAPSHOT_LIST_OUTPUT_TYPE, &container.Resource)
}
