package client

const (
	SNAPSHOT_CRLIST_OUTPUT_TYPE = "snapshotCRListOutput"
)

type SnapshotCRListOutput struct {
	Resource `yaml:"-"`

	Data []SnapshotCR `json:"data,omitempty" yaml:"data,omitempty"`
}

type SnapshotCRListOutputCollection struct {
	Collection
	Data   []SnapshotCRListOutput `json:"data,omitempty"`
	client *SnapshotCRListOutputClient
}

type SnapshotCRListOutputClient struct {
	rancherClient *RancherClient
}

type SnapshotCRListOutputOperations interface {
	List(opts *ListOpts) (*SnapshotCRListOutputCollection, error)
	Create(opts *SnapshotCRListOutput) (*SnapshotCRListOutput, error)
	Update(existing *SnapshotCRListOutput, updates interface{}) (*SnapshotCRListOutput, error)
	ById(id string) (*SnapshotCRListOutput, error)
	Delete(container *SnapshotCRListOutput) error
}

func newSnapshotCRListOutputClient(rancherClient *RancherClient) *SnapshotCRListOutputClient {
	return &SnapshotCRListOutputClient{
		rancherClient: rancherClient,
	}
}

func (c *SnapshotCRListOutputClient) Create(container *SnapshotCRListOutput) (*SnapshotCRListOutput, error) {
	resp := &SnapshotCRListOutput{}
	err := c.rancherClient.doCreate(SNAPSHOT_CRLIST_OUTPUT_TYPE, container, resp)
	return resp, err
}

func (c *SnapshotCRListOutputClient) Update(existing *SnapshotCRListOutput, updates interface{}) (*SnapshotCRListOutput, error) {
	resp := &SnapshotCRListOutput{}
	err := c.rancherClient.doUpdate(SNAPSHOT_CRLIST_OUTPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SnapshotCRListOutputClient) List(opts *ListOpts) (*SnapshotCRListOutputCollection, error) {
	resp := &SnapshotCRListOutputCollection{}
	err := c.rancherClient.doList(SNAPSHOT_CRLIST_OUTPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SnapshotCRListOutputCollection) Next() (*SnapshotCRListOutputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SnapshotCRListOutputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SnapshotCRListOutputClient) ById(id string) (*SnapshotCRListOutput, error) {
	resp := &SnapshotCRListOutput{}
	err := c.rancherClient.doById(SNAPSHOT_CRLIST_OUTPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SnapshotCRListOutputClient) Delete(container *SnapshotCRListOutput) error {
	return c.rancherClient.doResourceDelete(SNAPSHOT_CRLIST_OUTPUT_TYPE, &container.Resource)
}
