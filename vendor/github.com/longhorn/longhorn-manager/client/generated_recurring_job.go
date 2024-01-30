package client

const (
	RECURRING_JOB_TYPE = "recurringJob"
)

type RecurringJob struct {
	Resource `yaml:"-"`

	Concurrency int64 `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`

	Cron string `json:"cron,omitempty" yaml:"cron,omitempty"`

	Groups []string `json:"groups,omitempty" yaml:"groups,omitempty"`

	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	Retain int64 `json:"retain,omitempty" yaml:"retain,omitempty"`

	Task string `json:"task,omitempty" yaml:"task,omitempty"`
}

type RecurringJobCollection struct {
	Collection
	Data   []RecurringJob `json:"data,omitempty"`
	client *RecurringJobClient
}

type RecurringJobClient struct {
	rancherClient *RancherClient
}

type RecurringJobOperations interface {
	List(opts *ListOpts) (*RecurringJobCollection, error)
	Create(opts *RecurringJob) (*RecurringJob, error)
	Update(existing *RecurringJob, updates interface{}) (*RecurringJob, error)
	ById(id string) (*RecurringJob, error)
	Delete(container *RecurringJob) error
}

func newRecurringJobClient(rancherClient *RancherClient) *RecurringJobClient {
	return &RecurringJobClient{
		rancherClient: rancherClient,
	}
}

func (c *RecurringJobClient) Create(container *RecurringJob) (*RecurringJob, error) {
	resp := &RecurringJob{}
	err := c.rancherClient.doCreate(RECURRING_JOB_TYPE, container, resp)
	return resp, err
}

func (c *RecurringJobClient) Update(existing *RecurringJob, updates interface{}) (*RecurringJob, error) {
	resp := &RecurringJob{}
	err := c.rancherClient.doUpdate(RECURRING_JOB_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *RecurringJobClient) List(opts *ListOpts) (*RecurringJobCollection, error) {
	resp := &RecurringJobCollection{}
	err := c.rancherClient.doList(RECURRING_JOB_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *RecurringJobCollection) Next() (*RecurringJobCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &RecurringJobCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *RecurringJobClient) ById(id string) (*RecurringJob, error) {
	resp := &RecurringJob{}
	err := c.rancherClient.doById(RECURRING_JOB_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *RecurringJobClient) Delete(container *RecurringJob) error {
	return c.rancherClient.doResourceDelete(RECURRING_JOB_TYPE, &container.Resource)
}
