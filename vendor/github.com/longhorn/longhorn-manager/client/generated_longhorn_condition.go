package client

const (
	LONGHORN_CONDITION_TYPE = "longhornCondition"
)

type LonghornCondition struct {
	Resource `yaml:"-"`

	LastProbeTime string `json:"lastProbeTime,omitempty" yaml:"last_probe_time,omitempty"`

	LastTransitionTime string `json:"lastTransitionTime,omitempty" yaml:"last_transition_time,omitempty"`

	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`

	Status string `json:"status,omitempty" yaml:"status,omitempty"`
}

type LonghornConditionCollection struct {
	Collection
	Data   []LonghornCondition `json:"data,omitempty"`
	client *LonghornConditionClient
}

type LonghornConditionClient struct {
	rancherClient *RancherClient
}

type LonghornConditionOperations interface {
	List(opts *ListOpts) (*LonghornConditionCollection, error)
	Create(opts *LonghornCondition) (*LonghornCondition, error)
	Update(existing *LonghornCondition, updates interface{}) (*LonghornCondition, error)
	ById(id string) (*LonghornCondition, error)
	Delete(container *LonghornCondition) error
}

func newLonghornConditionClient(rancherClient *RancherClient) *LonghornConditionClient {
	return &LonghornConditionClient{
		rancherClient: rancherClient,
	}
}

func (c *LonghornConditionClient) Create(container *LonghornCondition) (*LonghornCondition, error) {
	resp := &LonghornCondition{}
	err := c.rancherClient.doCreate(LONGHORN_CONDITION_TYPE, container, resp)
	return resp, err
}

func (c *LonghornConditionClient) Update(existing *LonghornCondition, updates interface{}) (*LonghornCondition, error) {
	resp := &LonghornCondition{}
	err := c.rancherClient.doUpdate(LONGHORN_CONDITION_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *LonghornConditionClient) List(opts *ListOpts) (*LonghornConditionCollection, error) {
	resp := &LonghornConditionCollection{}
	err := c.rancherClient.doList(LONGHORN_CONDITION_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *LonghornConditionCollection) Next() (*LonghornConditionCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &LonghornConditionCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *LonghornConditionClient) ById(id string) (*LonghornCondition, error) {
	resp := &LonghornCondition{}
	err := c.rancherClient.doById(LONGHORN_CONDITION_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *LonghornConditionClient) Delete(container *LonghornCondition) error {
	return c.rancherClient.doResourceDelete(LONGHORN_CONDITION_TYPE, &container.Resource)
}
