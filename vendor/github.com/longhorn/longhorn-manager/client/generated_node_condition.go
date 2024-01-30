package client

const (
	NODE_CONDITION_TYPE = "nodeCondition"
)

type NodeCondition struct {
	Resource `yaml:"-"`

	LastProbeTime string `json:"lastProbeTime,omitempty" yaml:"last_probe_time,omitempty"`

	LastTransitionTime string `json:"lastTransitionTime,omitempty" yaml:"last_transition_time,omitempty"`

	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`

	Status string `json:"status,omitempty" yaml:"status,omitempty"`
}

type NodeConditionCollection struct {
	Collection
	Data   []NodeCondition `json:"data,omitempty"`
	client *NodeConditionClient
}

type NodeConditionClient struct {
	rancherClient *RancherClient
}

type NodeConditionOperations interface {
	List(opts *ListOpts) (*NodeConditionCollection, error)
	Create(opts *NodeCondition) (*NodeCondition, error)
	Update(existing *NodeCondition, updates interface{}) (*NodeCondition, error)
	ById(id string) (*NodeCondition, error)
	Delete(container *NodeCondition) error
}

func newNodeConditionClient(rancherClient *RancherClient) *NodeConditionClient {
	return &NodeConditionClient{
		rancherClient: rancherClient,
	}
}

func (c *NodeConditionClient) Create(container *NodeCondition) (*NodeCondition, error) {
	resp := &NodeCondition{}
	err := c.rancherClient.doCreate(NODE_CONDITION_TYPE, container, resp)
	return resp, err
}

func (c *NodeConditionClient) Update(existing *NodeCondition, updates interface{}) (*NodeCondition, error) {
	resp := &NodeCondition{}
	err := c.rancherClient.doUpdate(NODE_CONDITION_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *NodeConditionClient) List(opts *ListOpts) (*NodeConditionCollection, error) {
	resp := &NodeConditionCollection{}
	err := c.rancherClient.doList(NODE_CONDITION_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *NodeConditionCollection) Next() (*NodeConditionCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &NodeConditionCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *NodeConditionClient) ById(id string) (*NodeCondition, error) {
	resp := &NodeCondition{}
	err := c.rancherClient.doById(NODE_CONDITION_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *NodeConditionClient) Delete(container *NodeCondition) error {
	return c.rancherClient.doResourceDelete(NODE_CONDITION_TYPE, &container.Resource)
}
