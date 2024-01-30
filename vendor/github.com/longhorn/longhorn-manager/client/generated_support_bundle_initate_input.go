package client

const (
	SUPPORT_BUNDLE_INITATE_INPUT_TYPE = "supportBundleInitateInput"
)

type SupportBundleInitateInput struct {
	Resource `yaml:"-"`

	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	IssueURL string `json:"issueURL,omitempty" yaml:"issue_url,omitempty"`
}

type SupportBundleInitateInputCollection struct {
	Collection
	Data   []SupportBundleInitateInput `json:"data,omitempty"`
	client *SupportBundleInitateInputClient
}

type SupportBundleInitateInputClient struct {
	rancherClient *RancherClient
}

type SupportBundleInitateInputOperations interface {
	List(opts *ListOpts) (*SupportBundleInitateInputCollection, error)
	Create(opts *SupportBundleInitateInput) (*SupportBundleInitateInput, error)
	Update(existing *SupportBundleInitateInput, updates interface{}) (*SupportBundleInitateInput, error)
	ById(id string) (*SupportBundleInitateInput, error)
	Delete(container *SupportBundleInitateInput) error
}

func newSupportBundleInitateInputClient(rancherClient *RancherClient) *SupportBundleInitateInputClient {
	return &SupportBundleInitateInputClient{
		rancherClient: rancherClient,
	}
}

func (c *SupportBundleInitateInputClient) Create(container *SupportBundleInitateInput) (*SupportBundleInitateInput, error) {
	resp := &SupportBundleInitateInput{}
	err := c.rancherClient.doCreate(SUPPORT_BUNDLE_INITATE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *SupportBundleInitateInputClient) Update(existing *SupportBundleInitateInput, updates interface{}) (*SupportBundleInitateInput, error) {
	resp := &SupportBundleInitateInput{}
	err := c.rancherClient.doUpdate(SUPPORT_BUNDLE_INITATE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SupportBundleInitateInputClient) List(opts *ListOpts) (*SupportBundleInitateInputCollection, error) {
	resp := &SupportBundleInitateInputCollection{}
	err := c.rancherClient.doList(SUPPORT_BUNDLE_INITATE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SupportBundleInitateInputCollection) Next() (*SupportBundleInitateInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SupportBundleInitateInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SupportBundleInitateInputClient) ById(id string) (*SupportBundleInitateInput, error) {
	resp := &SupportBundleInitateInput{}
	err := c.rancherClient.doById(SUPPORT_BUNDLE_INITATE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SupportBundleInitateInputClient) Delete(container *SupportBundleInitateInput) error {
	return c.rancherClient.doResourceDelete(SUPPORT_BUNDLE_INITATE_INPUT_TYPE, &container.Resource)
}
