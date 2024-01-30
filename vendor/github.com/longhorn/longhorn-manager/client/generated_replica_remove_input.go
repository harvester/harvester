package client

const (
	REPLICA_REMOVE_INPUT_TYPE = "replicaRemoveInput"
)

type ReplicaRemoveInput struct {
	Resource `yaml:"-"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type ReplicaRemoveInputCollection struct {
	Collection
	Data   []ReplicaRemoveInput `json:"data,omitempty"`
	client *ReplicaRemoveInputClient
}

type ReplicaRemoveInputClient struct {
	rancherClient *RancherClient
}

type ReplicaRemoveInputOperations interface {
	List(opts *ListOpts) (*ReplicaRemoveInputCollection, error)
	Create(opts *ReplicaRemoveInput) (*ReplicaRemoveInput, error)
	Update(existing *ReplicaRemoveInput, updates interface{}) (*ReplicaRemoveInput, error)
	ById(id string) (*ReplicaRemoveInput, error)
	Delete(container *ReplicaRemoveInput) error
}

func newReplicaRemoveInputClient(rancherClient *RancherClient) *ReplicaRemoveInputClient {
	return &ReplicaRemoveInputClient{
		rancherClient: rancherClient,
	}
}

func (c *ReplicaRemoveInputClient) Create(container *ReplicaRemoveInput) (*ReplicaRemoveInput, error) {
	resp := &ReplicaRemoveInput{}
	err := c.rancherClient.doCreate(REPLICA_REMOVE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *ReplicaRemoveInputClient) Update(existing *ReplicaRemoveInput, updates interface{}) (*ReplicaRemoveInput, error) {
	resp := &ReplicaRemoveInput{}
	err := c.rancherClient.doUpdate(REPLICA_REMOVE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *ReplicaRemoveInputClient) List(opts *ListOpts) (*ReplicaRemoveInputCollection, error) {
	resp := &ReplicaRemoveInputCollection{}
	err := c.rancherClient.doList(REPLICA_REMOVE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *ReplicaRemoveInputCollection) Next() (*ReplicaRemoveInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &ReplicaRemoveInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *ReplicaRemoveInputClient) ById(id string) (*ReplicaRemoveInput, error) {
	resp := &ReplicaRemoveInput{}
	err := c.rancherClient.doById(REPLICA_REMOVE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *ReplicaRemoveInputClient) Delete(container *ReplicaRemoveInput) error {
	return c.rancherClient.doResourceDelete(REPLICA_REMOVE_INPUT_TYPE, &container.Resource)
}
