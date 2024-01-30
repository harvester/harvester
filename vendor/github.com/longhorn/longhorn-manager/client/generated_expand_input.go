package client

const (
	EXPAND_INPUT_TYPE = "expandInput"
)

type ExpandInput struct {
	Resource `yaml:"-"`

	Size string `json:"size,omitempty" yaml:"size,omitempty"`
}

type ExpandInputCollection struct {
	Collection
	Data   []ExpandInput `json:"data,omitempty"`
	client *ExpandInputClient
}

type ExpandInputClient struct {
	rancherClient *RancherClient
}

type ExpandInputOperations interface {
	List(opts *ListOpts) (*ExpandInputCollection, error)
	Create(opts *ExpandInput) (*ExpandInput, error)
	Update(existing *ExpandInput, updates interface{}) (*ExpandInput, error)
	ById(id string) (*ExpandInput, error)
	Delete(container *ExpandInput) error
}

func newExpandInputClient(rancherClient *RancherClient) *ExpandInputClient {
	return &ExpandInputClient{
		rancherClient: rancherClient,
	}
}

func (c *ExpandInputClient) Create(container *ExpandInput) (*ExpandInput, error) {
	resp := &ExpandInput{}
	err := c.rancherClient.doCreate(EXPAND_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *ExpandInputClient) Update(existing *ExpandInput, updates interface{}) (*ExpandInput, error) {
	resp := &ExpandInput{}
	err := c.rancherClient.doUpdate(EXPAND_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *ExpandInputClient) List(opts *ListOpts) (*ExpandInputCollection, error) {
	resp := &ExpandInputCollection{}
	err := c.rancherClient.doList(EXPAND_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *ExpandInputCollection) Next() (*ExpandInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &ExpandInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *ExpandInputClient) ById(id string) (*ExpandInput, error) {
	resp := &ExpandInput{}
	err := c.rancherClient.doById(EXPAND_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *ExpandInputClient) Delete(container *ExpandInput) error {
	return c.rancherClient.doResourceDelete(EXPAND_INPUT_TYPE, &container.Resource)
}
