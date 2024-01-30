package client

const (
	INSTANCE_MANAGER_TYPE = "instanceManager"
)

type InstanceManager struct {
	Resource `yaml:"-"`

	CurrentState string `json:"currentState,omitempty" yaml:"current_state,omitempty"`

	Image string `json:"image,omitempty" yaml:"image,omitempty"`

	InstanceEngines map[string]string `json:"instanceEngines,omitempty" yaml:"instance_engines,omitempty"`

	InstanceReplicas map[string]string `json:"instanceReplicas,omitempty" yaml:"instance_replicas,omitempty"`

	Instances map[string]string `json:"instances,omitempty" yaml:"instances,omitempty"`

	ManagerType string `json:"managerType,omitempty" yaml:"manager_type,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	NodeID string `json:"nodeID,omitempty" yaml:"node_id,omitempty"`
}

type InstanceManagerCollection struct {
	Collection
	Data   []InstanceManager `json:"data,omitempty"`
	client *InstanceManagerClient
}

type InstanceManagerClient struct {
	rancherClient *RancherClient
}

type InstanceManagerOperations interface {
	List(opts *ListOpts) (*InstanceManagerCollection, error)
	Create(opts *InstanceManager) (*InstanceManager, error)
	Update(existing *InstanceManager, updates interface{}) (*InstanceManager, error)
	ById(id string) (*InstanceManager, error)
	Delete(container *InstanceManager) error
}

func newInstanceManagerClient(rancherClient *RancherClient) *InstanceManagerClient {
	return &InstanceManagerClient{
		rancherClient: rancherClient,
	}
}

func (c *InstanceManagerClient) Create(container *InstanceManager) (*InstanceManager, error) {
	resp := &InstanceManager{}
	err := c.rancherClient.doCreate(INSTANCE_MANAGER_TYPE, container, resp)
	return resp, err
}

func (c *InstanceManagerClient) Update(existing *InstanceManager, updates interface{}) (*InstanceManager, error) {
	resp := &InstanceManager{}
	err := c.rancherClient.doUpdate(INSTANCE_MANAGER_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *InstanceManagerClient) List(opts *ListOpts) (*InstanceManagerCollection, error) {
	resp := &InstanceManagerCollection{}
	err := c.rancherClient.doList(INSTANCE_MANAGER_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *InstanceManagerCollection) Next() (*InstanceManagerCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &InstanceManagerCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *InstanceManagerClient) ById(id string) (*InstanceManager, error) {
	resp := &InstanceManager{}
	err := c.rancherClient.doById(INSTANCE_MANAGER_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *InstanceManagerClient) Delete(container *InstanceManager) error {
	return c.rancherClient.doResourceDelete(INSTANCE_MANAGER_TYPE, &container.Resource)
}
