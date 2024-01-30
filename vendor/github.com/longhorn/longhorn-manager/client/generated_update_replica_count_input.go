package client

const (
	UPDATE_REPLICA_COUNT_INPUT_TYPE = "UpdateReplicaCountInput"
)

type UpdateReplicaCountInput struct {
	Resource `yaml:"-"`

	ReplicaCount int64 `json:"replicaCount,omitempty" yaml:"replica_count,omitempty"`
}

type UpdateReplicaCountInputCollection struct {
	Collection
	Data   []UpdateReplicaCountInput `json:"data,omitempty"`
	client *UpdateReplicaCountInputClient
}

type UpdateReplicaCountInputClient struct {
	rancherClient *RancherClient
}

type UpdateReplicaCountInputOperations interface {
	List(opts *ListOpts) (*UpdateReplicaCountInputCollection, error)
	Create(opts *UpdateReplicaCountInput) (*UpdateReplicaCountInput, error)
	Update(existing *UpdateReplicaCountInput, updates interface{}) (*UpdateReplicaCountInput, error)
	ById(id string) (*UpdateReplicaCountInput, error)
	Delete(container *UpdateReplicaCountInput) error
}

func newUpdateReplicaCountInputClient(rancherClient *RancherClient) *UpdateReplicaCountInputClient {
	return &UpdateReplicaCountInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateReplicaCountInputClient) Create(container *UpdateReplicaCountInput) (*UpdateReplicaCountInput, error) {
	resp := &UpdateReplicaCountInput{}
	err := c.rancherClient.doCreate(UPDATE_REPLICA_COUNT_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateReplicaCountInputClient) Update(existing *UpdateReplicaCountInput, updates interface{}) (*UpdateReplicaCountInput, error) {
	resp := &UpdateReplicaCountInput{}
	err := c.rancherClient.doUpdate(UPDATE_REPLICA_COUNT_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateReplicaCountInputClient) List(opts *ListOpts) (*UpdateReplicaCountInputCollection, error) {
	resp := &UpdateReplicaCountInputCollection{}
	err := c.rancherClient.doList(UPDATE_REPLICA_COUNT_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateReplicaCountInputCollection) Next() (*UpdateReplicaCountInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateReplicaCountInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateReplicaCountInputClient) ById(id string) (*UpdateReplicaCountInput, error) {
	resp := &UpdateReplicaCountInput{}
	err := c.rancherClient.doById(UPDATE_REPLICA_COUNT_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateReplicaCountInputClient) Delete(container *UpdateReplicaCountInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_REPLICA_COUNT_INPUT_TYPE, &container.Resource)
}
