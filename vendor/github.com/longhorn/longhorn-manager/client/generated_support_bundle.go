package client

const (
	SUPPORT_BUNDLE_TYPE = "supportBundle"
)

type SupportBundle struct {
	Resource `yaml:"-"`

	ErrorMessage string `json:"errorMessage,omitempty" yaml:"error_message,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	NodeID string `json:"nodeID,omitempty" yaml:"node_id,omitempty"`

	ProgressPercentage int64 `json:"progressPercentage,omitempty" yaml:"progress_percentage,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`
}

type SupportBundleCollection struct {
	Collection
	Data   []SupportBundle `json:"data,omitempty"`
	client *SupportBundleClient
}

type SupportBundleClient struct {
	rancherClient *RancherClient
}

type SupportBundleOperations interface {
	List(opts *ListOpts) (*SupportBundleCollection, error)
	Create(opts *SupportBundle) (*SupportBundle, error)
	Update(existing *SupportBundle, updates interface{}) (*SupportBundle, error)
	ById(id string) (*SupportBundle, error)
	Delete(container *SupportBundle) error
}

func newSupportBundleClient(rancherClient *RancherClient) *SupportBundleClient {
	return &SupportBundleClient{
		rancherClient: rancherClient,
	}
}

func (c *SupportBundleClient) Create(container *SupportBundle) (*SupportBundle, error) {
	resp := &SupportBundle{}
	err := c.rancherClient.doCreate(SUPPORT_BUNDLE_TYPE, container, resp)
	return resp, err
}

func (c *SupportBundleClient) Update(existing *SupportBundle, updates interface{}) (*SupportBundle, error) {
	resp := &SupportBundle{}
	err := c.rancherClient.doUpdate(SUPPORT_BUNDLE_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SupportBundleClient) List(opts *ListOpts) (*SupportBundleCollection, error) {
	resp := &SupportBundleCollection{}
	err := c.rancherClient.doList(SUPPORT_BUNDLE_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SupportBundleCollection) Next() (*SupportBundleCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SupportBundleCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SupportBundleClient) ById(id string) (*SupportBundle, error) {
	resp := &SupportBundle{}
	err := c.rancherClient.doById(SUPPORT_BUNDLE_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SupportBundleClient) Delete(container *SupportBundle) error {
	return c.rancherClient.doResourceDelete(SUPPORT_BUNDLE_TYPE, &container.Resource)
}
