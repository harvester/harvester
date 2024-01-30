package client

const (
	UPDATE_SNAPSHOT_DATA_INTEGRITY_INPUT_TYPE = "UpdateSnapshotDataIntegrityInput"
)

type UpdateSnapshotDataIntegrityInput struct {
	Resource `yaml:"-"`

	SnapshotDataIntegrity string `json:"snapshotDataIntegrity,omitempty" yaml:"snapshot_data_integrity,omitempty"`
}

type UpdateSnapshotDataIntegrityInputCollection struct {
	Collection
	Data   []UpdateSnapshotDataIntegrityInput `json:"data,omitempty"`
	client *UpdateSnapshotDataIntegrityInputClient
}

type UpdateSnapshotDataIntegrityInputClient struct {
	rancherClient *RancherClient
}

type UpdateSnapshotDataIntegrityInputOperations interface {
	List(opts *ListOpts) (*UpdateSnapshotDataIntegrityInputCollection, error)
	Create(opts *UpdateSnapshotDataIntegrityInput) (*UpdateSnapshotDataIntegrityInput, error)
	Update(existing *UpdateSnapshotDataIntegrityInput, updates interface{}) (*UpdateSnapshotDataIntegrityInput, error)
	ById(id string) (*UpdateSnapshotDataIntegrityInput, error)
	Delete(container *UpdateSnapshotDataIntegrityInput) error
}

func newUpdateSnapshotDataIntegrityInputClient(rancherClient *RancherClient) *UpdateSnapshotDataIntegrityInputClient {
	return &UpdateSnapshotDataIntegrityInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateSnapshotDataIntegrityInputClient) Create(container *UpdateSnapshotDataIntegrityInput) (*UpdateSnapshotDataIntegrityInput, error) {
	resp := &UpdateSnapshotDataIntegrityInput{}
	err := c.rancherClient.doCreate(UPDATE_SNAPSHOT_DATA_INTEGRITY_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateSnapshotDataIntegrityInputClient) Update(existing *UpdateSnapshotDataIntegrityInput, updates interface{}) (*UpdateSnapshotDataIntegrityInput, error) {
	resp := &UpdateSnapshotDataIntegrityInput{}
	err := c.rancherClient.doUpdate(UPDATE_SNAPSHOT_DATA_INTEGRITY_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateSnapshotDataIntegrityInputClient) List(opts *ListOpts) (*UpdateSnapshotDataIntegrityInputCollection, error) {
	resp := &UpdateSnapshotDataIntegrityInputCollection{}
	err := c.rancherClient.doList(UPDATE_SNAPSHOT_DATA_INTEGRITY_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateSnapshotDataIntegrityInputCollection) Next() (*UpdateSnapshotDataIntegrityInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateSnapshotDataIntegrityInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateSnapshotDataIntegrityInputClient) ById(id string) (*UpdateSnapshotDataIntegrityInput, error) {
	resp := &UpdateSnapshotDataIntegrityInput{}
	err := c.rancherClient.doById(UPDATE_SNAPSHOT_DATA_INTEGRITY_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateSnapshotDataIntegrityInputClient) Delete(container *UpdateSnapshotDataIntegrityInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_SNAPSHOT_DATA_INTEGRITY_INPUT_TYPE, &container.Resource)
}
