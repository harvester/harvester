package client

const (
	UPDATE_REPLICA_ZONE_SOFT_ANTI_AFFINITY_INPUT_TYPE = "UpdateReplicaZoneSoftAntiAffinityInput"
)

type UpdateReplicaZoneSoftAntiAffinityInput struct {
	Resource `yaml:"-"`

	ReplicaZoneSoftAntiAffinity string `json:"replicaZoneSoftAntiAffinity,omitempty" yaml:"replica_zone_soft_anti_affinity,omitempty"`
}

type UpdateReplicaZoneSoftAntiAffinityInputCollection struct {
	Collection
	Data   []UpdateReplicaZoneSoftAntiAffinityInput `json:"data,omitempty"`
	client *UpdateReplicaZoneSoftAntiAffinityInputClient
}

type UpdateReplicaZoneSoftAntiAffinityInputClient struct {
	rancherClient *RancherClient
}

type UpdateReplicaZoneSoftAntiAffinityInputOperations interface {
	List(opts *ListOpts) (*UpdateReplicaZoneSoftAntiAffinityInputCollection, error)
	Create(opts *UpdateReplicaZoneSoftAntiAffinityInput) (*UpdateReplicaZoneSoftAntiAffinityInput, error)
	Update(existing *UpdateReplicaZoneSoftAntiAffinityInput, updates interface{}) (*UpdateReplicaZoneSoftAntiAffinityInput, error)
	ById(id string) (*UpdateReplicaZoneSoftAntiAffinityInput, error)
	Delete(container *UpdateReplicaZoneSoftAntiAffinityInput) error
}

func newUpdateReplicaZoneSoftAntiAffinityInputClient(rancherClient *RancherClient) *UpdateReplicaZoneSoftAntiAffinityInputClient {
	return &UpdateReplicaZoneSoftAntiAffinityInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateReplicaZoneSoftAntiAffinityInputClient) Create(container *UpdateReplicaZoneSoftAntiAffinityInput) (*UpdateReplicaZoneSoftAntiAffinityInput, error) {
	resp := &UpdateReplicaZoneSoftAntiAffinityInput{}
	err := c.rancherClient.doCreate(UPDATE_REPLICA_ZONE_SOFT_ANTI_AFFINITY_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateReplicaZoneSoftAntiAffinityInputClient) Update(existing *UpdateReplicaZoneSoftAntiAffinityInput, updates interface{}) (*UpdateReplicaZoneSoftAntiAffinityInput, error) {
	resp := &UpdateReplicaZoneSoftAntiAffinityInput{}
	err := c.rancherClient.doUpdate(UPDATE_REPLICA_ZONE_SOFT_ANTI_AFFINITY_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateReplicaZoneSoftAntiAffinityInputClient) List(opts *ListOpts) (*UpdateReplicaZoneSoftAntiAffinityInputCollection, error) {
	resp := &UpdateReplicaZoneSoftAntiAffinityInputCollection{}
	err := c.rancherClient.doList(UPDATE_REPLICA_ZONE_SOFT_ANTI_AFFINITY_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateReplicaZoneSoftAntiAffinityInputCollection) Next() (*UpdateReplicaZoneSoftAntiAffinityInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateReplicaZoneSoftAntiAffinityInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateReplicaZoneSoftAntiAffinityInputClient) ById(id string) (*UpdateReplicaZoneSoftAntiAffinityInput, error) {
	resp := &UpdateReplicaZoneSoftAntiAffinityInput{}
	err := c.rancherClient.doById(UPDATE_REPLICA_ZONE_SOFT_ANTI_AFFINITY_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateReplicaZoneSoftAntiAffinityInputClient) Delete(container *UpdateReplicaZoneSoftAntiAffinityInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_REPLICA_ZONE_SOFT_ANTI_AFFINITY_INPUT_TYPE, &container.Resource)
}
