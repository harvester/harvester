package client

const (
	VOLUME_CONDITION_TYPE = "volumeCondition"
)

type VolumeCondition struct {
	Resource `yaml:"-"`

	LastProbeTime string `json:"lastProbeTime,omitempty" yaml:"last_probe_time,omitempty"`

	LastTransitionTime string `json:"lastTransitionTime,omitempty" yaml:"last_transition_time,omitempty"`

	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`

	Status string `json:"status,omitempty" yaml:"status,omitempty"`
}

type VolumeConditionCollection struct {
	Collection
	Data   []VolumeCondition `json:"data,omitempty"`
	client *VolumeConditionClient
}

type VolumeConditionClient struct {
	rancherClient *RancherClient
}

type VolumeConditionOperations interface {
	List(opts *ListOpts) (*VolumeConditionCollection, error)
	Create(opts *VolumeCondition) (*VolumeCondition, error)
	Update(existing *VolumeCondition, updates interface{}) (*VolumeCondition, error)
	ById(id string) (*VolumeCondition, error)
	Delete(container *VolumeCondition) error
}

func newVolumeConditionClient(rancherClient *RancherClient) *VolumeConditionClient {
	return &VolumeConditionClient{
		rancherClient: rancherClient,
	}
}

func (c *VolumeConditionClient) Create(container *VolumeCondition) (*VolumeCondition, error) {
	resp := &VolumeCondition{}
	err := c.rancherClient.doCreate(VOLUME_CONDITION_TYPE, container, resp)
	return resp, err
}

func (c *VolumeConditionClient) Update(existing *VolumeCondition, updates interface{}) (*VolumeCondition, error) {
	resp := &VolumeCondition{}
	err := c.rancherClient.doUpdate(VOLUME_CONDITION_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *VolumeConditionClient) List(opts *ListOpts) (*VolumeConditionCollection, error) {
	resp := &VolumeConditionCollection{}
	err := c.rancherClient.doList(VOLUME_CONDITION_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *VolumeConditionCollection) Next() (*VolumeConditionCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &VolumeConditionCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *VolumeConditionClient) ById(id string) (*VolumeCondition, error) {
	resp := &VolumeCondition{}
	err := c.rancherClient.doById(VOLUME_CONDITION_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *VolumeConditionClient) Delete(container *VolumeCondition) error {
	return c.rancherClient.doResourceDelete(VOLUME_CONDITION_TYPE, &container.Resource)
}
