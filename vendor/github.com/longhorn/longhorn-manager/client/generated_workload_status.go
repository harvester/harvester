package client

const (
	WORKLOAD_STATUS_TYPE = "workloadStatus"
)

type WorkloadStatus struct {
	Resource `yaml:"-"`

	PodName string `json:"podName,omitempty" yaml:"pod_name,omitempty"`

	PodStatus string `json:"podStatus,omitempty" yaml:"pod_status,omitempty"`

	WorkloadName string `json:"workloadName,omitempty" yaml:"workload_name,omitempty"`

	WorkloadType string `json:"workloadType,omitempty" yaml:"workload_type,omitempty"`
}

type WorkloadStatusCollection struct {
	Collection
	Data   []WorkloadStatus `json:"data,omitempty"`
	client *WorkloadStatusClient
}

type WorkloadStatusClient struct {
	rancherClient *RancherClient
}

type WorkloadStatusOperations interface {
	List(opts *ListOpts) (*WorkloadStatusCollection, error)
	Create(opts *WorkloadStatus) (*WorkloadStatus, error)
	Update(existing *WorkloadStatus, updates interface{}) (*WorkloadStatus, error)
	ById(id string) (*WorkloadStatus, error)
	Delete(container *WorkloadStatus) error
}

func newWorkloadStatusClient(rancherClient *RancherClient) *WorkloadStatusClient {
	return &WorkloadStatusClient{
		rancherClient: rancherClient,
	}
}

func (c *WorkloadStatusClient) Create(container *WorkloadStatus) (*WorkloadStatus, error) {
	resp := &WorkloadStatus{}
	err := c.rancherClient.doCreate(WORKLOAD_STATUS_TYPE, container, resp)
	return resp, err
}

func (c *WorkloadStatusClient) Update(existing *WorkloadStatus, updates interface{}) (*WorkloadStatus, error) {
	resp := &WorkloadStatus{}
	err := c.rancherClient.doUpdate(WORKLOAD_STATUS_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *WorkloadStatusClient) List(opts *ListOpts) (*WorkloadStatusCollection, error) {
	resp := &WorkloadStatusCollection{}
	err := c.rancherClient.doList(WORKLOAD_STATUS_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *WorkloadStatusCollection) Next() (*WorkloadStatusCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &WorkloadStatusCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *WorkloadStatusClient) ById(id string) (*WorkloadStatus, error) {
	resp := &WorkloadStatus{}
	err := c.rancherClient.doById(WORKLOAD_STATUS_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *WorkloadStatusClient) Delete(container *WorkloadStatus) error {
	return c.rancherClient.doResourceDelete(WORKLOAD_STATUS_TYPE, &container.Resource)
}
