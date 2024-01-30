package client

const (
	DISK_CONDITION_TYPE = "diskCondition"
)

type DiskCondition struct {
	Resource `yaml:"-"`

	LastProbeTime string `json:"lastProbeTime,omitempty" yaml:"last_probe_time,omitempty"`

	LastTransitionTime string `json:"lastTransitionTime,omitempty" yaml:"last_transition_time,omitempty"`

	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`

	Status string `json:"status,omitempty" yaml:"status,omitempty"`
}

type DiskConditionCollection struct {
	Collection
	Data   []DiskCondition `json:"data,omitempty"`
	client *DiskConditionClient
}

type DiskConditionClient struct {
	rancherClient *RancherClient
}

type DiskConditionOperations interface {
	List(opts *ListOpts) (*DiskConditionCollection, error)
	Create(opts *DiskCondition) (*DiskCondition, error)
	Update(existing *DiskCondition, updates interface{}) (*DiskCondition, error)
	ById(id string) (*DiskCondition, error)
	Delete(container *DiskCondition) error
}

func newDiskConditionClient(rancherClient *RancherClient) *DiskConditionClient {
	return &DiskConditionClient{
		rancherClient: rancherClient,
	}
}

func (c *DiskConditionClient) Create(container *DiskCondition) (*DiskCondition, error) {
	resp := &DiskCondition{}
	err := c.rancherClient.doCreate(DISK_CONDITION_TYPE, container, resp)
	return resp, err
}

func (c *DiskConditionClient) Update(existing *DiskCondition, updates interface{}) (*DiskCondition, error) {
	resp := &DiskCondition{}
	err := c.rancherClient.doUpdate(DISK_CONDITION_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *DiskConditionClient) List(opts *ListOpts) (*DiskConditionCollection, error) {
	resp := &DiskConditionCollection{}
	err := c.rancherClient.doList(DISK_CONDITION_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *DiskConditionCollection) Next() (*DiskConditionCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &DiskConditionCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *DiskConditionClient) ById(id string) (*DiskCondition, error) {
	resp := &DiskCondition{}
	err := c.rancherClient.doById(DISK_CONDITION_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *DiskConditionClient) Delete(container *DiskCondition) error {
	return c.rancherClient.doResourceDelete(DISK_CONDITION_TYPE, &container.Resource)
}
