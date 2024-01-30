package client

const (
	UPDATE_REPLICA_AUTO_BALANCE_INPUT_TYPE = "UpdateReplicaAutoBalanceInput"
)

type UpdateReplicaAutoBalanceInput struct {
	Resource `yaml:"-"`

	ReplicaAutoBalance string `json:"replicaAutoBalance,omitempty" yaml:"replica_auto_balance,omitempty"`
}

type UpdateReplicaAutoBalanceInputCollection struct {
	Collection
	Data   []UpdateReplicaAutoBalanceInput `json:"data,omitempty"`
	client *UpdateReplicaAutoBalanceInputClient
}

type UpdateReplicaAutoBalanceInputClient struct {
	rancherClient *RancherClient
}

type UpdateReplicaAutoBalanceInputOperations interface {
	List(opts *ListOpts) (*UpdateReplicaAutoBalanceInputCollection, error)
	Create(opts *UpdateReplicaAutoBalanceInput) (*UpdateReplicaAutoBalanceInput, error)
	Update(existing *UpdateReplicaAutoBalanceInput, updates interface{}) (*UpdateReplicaAutoBalanceInput, error)
	ById(id string) (*UpdateReplicaAutoBalanceInput, error)
	Delete(container *UpdateReplicaAutoBalanceInput) error
}

func newUpdateReplicaAutoBalanceInputClient(rancherClient *RancherClient) *UpdateReplicaAutoBalanceInputClient {
	return &UpdateReplicaAutoBalanceInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateReplicaAutoBalanceInputClient) Create(container *UpdateReplicaAutoBalanceInput) (*UpdateReplicaAutoBalanceInput, error) {
	resp := &UpdateReplicaAutoBalanceInput{}
	err := c.rancherClient.doCreate(UPDATE_REPLICA_AUTO_BALANCE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateReplicaAutoBalanceInputClient) Update(existing *UpdateReplicaAutoBalanceInput, updates interface{}) (*UpdateReplicaAutoBalanceInput, error) {
	resp := &UpdateReplicaAutoBalanceInput{}
	err := c.rancherClient.doUpdate(UPDATE_REPLICA_AUTO_BALANCE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateReplicaAutoBalanceInputClient) List(opts *ListOpts) (*UpdateReplicaAutoBalanceInputCollection, error) {
	resp := &UpdateReplicaAutoBalanceInputCollection{}
	err := c.rancherClient.doList(UPDATE_REPLICA_AUTO_BALANCE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateReplicaAutoBalanceInputCollection) Next() (*UpdateReplicaAutoBalanceInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateReplicaAutoBalanceInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateReplicaAutoBalanceInputClient) ById(id string) (*UpdateReplicaAutoBalanceInput, error) {
	resp := &UpdateReplicaAutoBalanceInput{}
	err := c.rancherClient.doById(UPDATE_REPLICA_AUTO_BALANCE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateReplicaAutoBalanceInputClient) Delete(container *UpdateReplicaAutoBalanceInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_REPLICA_AUTO_BALANCE_INPUT_TYPE, &container.Resource)
}
