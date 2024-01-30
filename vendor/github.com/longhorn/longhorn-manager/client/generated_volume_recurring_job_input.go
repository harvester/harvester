package client

const (
	VOLUME_RECURRING_JOB_INPUT_TYPE = "volumeRecurringJobInput"
)

type VolumeRecurringJobInput struct {
	Resource `yaml:"-"`

	IsGroup bool `json:"isGroup,omitempty" yaml:"is_group,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type VolumeRecurringJobInputCollection struct {
	Collection
	Data   []VolumeRecurringJobInput `json:"data,omitempty"`
	client *VolumeRecurringJobInputClient
}

type VolumeRecurringJobInputClient struct {
	rancherClient *RancherClient
}

type VolumeRecurringJobInputOperations interface {
	List(opts *ListOpts) (*VolumeRecurringJobInputCollection, error)
	Create(opts *VolumeRecurringJobInput) (*VolumeRecurringJobInput, error)
	Update(existing *VolumeRecurringJobInput, updates interface{}) (*VolumeRecurringJobInput, error)
	ById(id string) (*VolumeRecurringJobInput, error)
	Delete(container *VolumeRecurringJobInput) error
}

func newVolumeRecurringJobInputClient(rancherClient *RancherClient) *VolumeRecurringJobInputClient {
	return &VolumeRecurringJobInputClient{
		rancherClient: rancherClient,
	}
}

func (c *VolumeRecurringJobInputClient) Create(container *VolumeRecurringJobInput) (*VolumeRecurringJobInput, error) {
	resp := &VolumeRecurringJobInput{}
	err := c.rancherClient.doCreate(VOLUME_RECURRING_JOB_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *VolumeRecurringJobInputClient) Update(existing *VolumeRecurringJobInput, updates interface{}) (*VolumeRecurringJobInput, error) {
	resp := &VolumeRecurringJobInput{}
	err := c.rancherClient.doUpdate(VOLUME_RECURRING_JOB_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *VolumeRecurringJobInputClient) List(opts *ListOpts) (*VolumeRecurringJobInputCollection, error) {
	resp := &VolumeRecurringJobInputCollection{}
	err := c.rancherClient.doList(VOLUME_RECURRING_JOB_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *VolumeRecurringJobInputCollection) Next() (*VolumeRecurringJobInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &VolumeRecurringJobInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *VolumeRecurringJobInputClient) ById(id string) (*VolumeRecurringJobInput, error) {
	resp := &VolumeRecurringJobInput{}
	err := c.rancherClient.doById(VOLUME_RECURRING_JOB_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *VolumeRecurringJobInputClient) Delete(container *VolumeRecurringJobInput) error {
	return c.rancherClient.doResourceDelete(VOLUME_RECURRING_JOB_INPUT_TYPE, &container.Resource)
}
