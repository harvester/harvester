package client

const (
	UPDATE_REPLICA_SOFT_ANTI_AFFINITY_INPUT_TYPE = "UpdateReplicaSoftAntiAffinityInput"
)

type UpdateReplicaSoftAntiAffinityInput struct {
	Resource `yaml:"-"`

	ReplicaSoftAntiAffinity string `json:"replicaSoftAntiAffinity,omitempty" yaml:"replica_soft_anti_affinity,omitempty"`
}

type UpdateReplicaSoftAntiAffinityInputCollection struct {
	Collection
	Data   []UpdateReplicaSoftAntiAffinityInput `json:"data,omitempty"`
	client *UpdateReplicaSoftAntiAffinityInputClient
}

type UpdateReplicaSoftAntiAffinityInputClient struct {
	rancherClient *RancherClient
}

type UpdateReplicaSoftAntiAffinityInputOperations interface {
	List(opts *ListOpts) (*UpdateReplicaSoftAntiAffinityInputCollection, error)
	Create(opts *UpdateReplicaSoftAntiAffinityInput) (*UpdateReplicaSoftAntiAffinityInput, error)
	Update(existing *UpdateReplicaSoftAntiAffinityInput, updates interface{}) (*UpdateReplicaSoftAntiAffinityInput, error)
	ById(id string) (*UpdateReplicaSoftAntiAffinityInput, error)
	Delete(container *UpdateReplicaSoftAntiAffinityInput) error
}

func newUpdateReplicaSoftAntiAffinityInputClient(rancherClient *RancherClient) *UpdateReplicaSoftAntiAffinityInputClient {
	return &UpdateReplicaSoftAntiAffinityInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateReplicaSoftAntiAffinityInputClient) Create(container *UpdateReplicaSoftAntiAffinityInput) (*UpdateReplicaSoftAntiAffinityInput, error) {
	resp := &UpdateReplicaSoftAntiAffinityInput{}
	err := c.rancherClient.doCreate(UPDATE_REPLICA_SOFT_ANTI_AFFINITY_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateReplicaSoftAntiAffinityInputClient) Update(existing *UpdateReplicaSoftAntiAffinityInput, updates interface{}) (*UpdateReplicaSoftAntiAffinityInput, error) {
	resp := &UpdateReplicaSoftAntiAffinityInput{}
	err := c.rancherClient.doUpdate(UPDATE_REPLICA_SOFT_ANTI_AFFINITY_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateReplicaSoftAntiAffinityInputClient) List(opts *ListOpts) (*UpdateReplicaSoftAntiAffinityInputCollection, error) {
	resp := &UpdateReplicaSoftAntiAffinityInputCollection{}
	err := c.rancherClient.doList(UPDATE_REPLICA_SOFT_ANTI_AFFINITY_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateReplicaSoftAntiAffinityInputCollection) Next() (*UpdateReplicaSoftAntiAffinityInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateReplicaSoftAntiAffinityInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateReplicaSoftAntiAffinityInputClient) ById(id string) (*UpdateReplicaSoftAntiAffinityInput, error) {
	resp := &UpdateReplicaSoftAntiAffinityInput{}
	err := c.rancherClient.doById(UPDATE_REPLICA_SOFT_ANTI_AFFINITY_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateReplicaSoftAntiAffinityInputClient) Delete(container *UpdateReplicaSoftAntiAffinityInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_REPLICA_SOFT_ANTI_AFFINITY_INPUT_TYPE, &container.Resource)
}
