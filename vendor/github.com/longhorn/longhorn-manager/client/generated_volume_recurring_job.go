package client

const (
	VOLUME_RECURRING_JOB_TYPE = "volumeRecurringJob"
)

type VolumeRecurringJob struct {
	Resource `yaml:"-"`

	IsGroup bool `json:"isGroup,omitempty" yaml:"is_group,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type VolumeRecurringJobCollection struct {
	Collection
	Data   []VolumeRecurringJob `json:"data,omitempty"`
	client *VolumeRecurringJobClient
}

type VolumeRecurringJobClient struct {
	rancherClient *RancherClient
}

type VolumeRecurringJobOperations interface {
	List(opts *ListOpts) (*VolumeRecurringJobCollection, error)
	Create(opts *VolumeRecurringJob) (*VolumeRecurringJob, error)
	Update(existing *VolumeRecurringJob, updates interface{}) (*VolumeRecurringJob, error)
	ById(id string) (*VolumeRecurringJob, error)
	Delete(container *VolumeRecurringJob) error
}

func newVolumeRecurringJobClient(rancherClient *RancherClient) *VolumeRecurringJobClient {
	return &VolumeRecurringJobClient{
		rancherClient: rancherClient,
	}
}

func (c *VolumeRecurringJobClient) Create(container *VolumeRecurringJob) (*VolumeRecurringJob, error) {
	resp := &VolumeRecurringJob{}
	err := c.rancherClient.doCreate(VOLUME_RECURRING_JOB_TYPE, container, resp)
	return resp, err
}

func (c *VolumeRecurringJobClient) Update(existing *VolumeRecurringJob, updates interface{}) (*VolumeRecurringJob, error) {
	resp := &VolumeRecurringJob{}
	err := c.rancherClient.doUpdate(VOLUME_RECURRING_JOB_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *VolumeRecurringJobClient) List(opts *ListOpts) (*VolumeRecurringJobCollection, error) {
	resp := &VolumeRecurringJobCollection{}
	err := c.rancherClient.doList(VOLUME_RECURRING_JOB_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *VolumeRecurringJobCollection) Next() (*VolumeRecurringJobCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &VolumeRecurringJobCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *VolumeRecurringJobClient) ById(id string) (*VolumeRecurringJob, error) {
	resp := &VolumeRecurringJob{}
	err := c.rancherClient.doById(VOLUME_RECURRING_JOB_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *VolumeRecurringJobClient) Delete(container *VolumeRecurringJob) error {
	return c.rancherClient.doResourceDelete(VOLUME_RECURRING_JOB_TYPE, &container.Resource)
}
