package client

const (
	UPDATE_OFFLINE_REPLICA_REBUILDING_INPUT_TYPE = "UpdateOfflineReplicaRebuildingInput"
)

type UpdateOfflineReplicaRebuildingInput struct {
	Resource `yaml:"-"`

	OfflineReplicaRebuilding string `json:"offlineReplicaRebuilding,omitempty" yaml:"offline_replica_rebuilding,omitempty"`
}

type UpdateOfflineReplicaRebuildingInputCollection struct {
	Collection
	Data   []UpdateOfflineReplicaRebuildingInput `json:"data,omitempty"`
	client *UpdateOfflineReplicaRebuildingInputClient
}

type UpdateOfflineReplicaRebuildingInputClient struct {
	rancherClient *RancherClient
}

type UpdateOfflineReplicaRebuildingInputOperations interface {
	List(opts *ListOpts) (*UpdateOfflineReplicaRebuildingInputCollection, error)
	Create(opts *UpdateOfflineReplicaRebuildingInput) (*UpdateOfflineReplicaRebuildingInput, error)
	Update(existing *UpdateOfflineReplicaRebuildingInput, updates interface{}) (*UpdateOfflineReplicaRebuildingInput, error)
	ById(id string) (*UpdateOfflineReplicaRebuildingInput, error)
	Delete(container *UpdateOfflineReplicaRebuildingInput) error
}

func newUpdateOfflineReplicaRebuildingInputClient(rancherClient *RancherClient) *UpdateOfflineReplicaRebuildingInputClient {
	return &UpdateOfflineReplicaRebuildingInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateOfflineReplicaRebuildingInputClient) Create(container *UpdateOfflineReplicaRebuildingInput) (*UpdateOfflineReplicaRebuildingInput, error) {
	resp := &UpdateOfflineReplicaRebuildingInput{}
	err := c.rancherClient.doCreate(UPDATE_OFFLINE_REPLICA_REBUILDING_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateOfflineReplicaRebuildingInputClient) Update(existing *UpdateOfflineReplicaRebuildingInput, updates interface{}) (*UpdateOfflineReplicaRebuildingInput, error) {
	resp := &UpdateOfflineReplicaRebuildingInput{}
	err := c.rancherClient.doUpdate(UPDATE_OFFLINE_REPLICA_REBUILDING_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateOfflineReplicaRebuildingInputClient) List(opts *ListOpts) (*UpdateOfflineReplicaRebuildingInputCollection, error) {
	resp := &UpdateOfflineReplicaRebuildingInputCollection{}
	err := c.rancherClient.doList(UPDATE_OFFLINE_REPLICA_REBUILDING_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateOfflineReplicaRebuildingInputCollection) Next() (*UpdateOfflineReplicaRebuildingInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateOfflineReplicaRebuildingInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateOfflineReplicaRebuildingInputClient) ById(id string) (*UpdateOfflineReplicaRebuildingInput, error) {
	resp := &UpdateOfflineReplicaRebuildingInput{}
	err := c.rancherClient.doById(UPDATE_OFFLINE_REPLICA_REBUILDING_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateOfflineReplicaRebuildingInputClient) Delete(container *UpdateOfflineReplicaRebuildingInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_OFFLINE_REPLICA_REBUILDING_INPUT_TYPE, &container.Resource)
}
