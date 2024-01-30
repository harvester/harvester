package client

const (
	ACTIVATE_INPUT_TYPE = "activateInput"
)

type ActivateInput struct {
	Resource `yaml:"-"`

	Frontend string `json:"frontend,omitempty" yaml:"frontend,omitempty"`
}

type ActivateInputCollection struct {
	Collection
	Data   []ActivateInput `json:"data,omitempty"`
	client *ActivateInputClient
}

type ActivateInputClient struct {
	rancherClient *RancherClient
}

type ActivateInputOperations interface {
	List(opts *ListOpts) (*ActivateInputCollection, error)
	Create(opts *ActivateInput) (*ActivateInput, error)
	Update(existing *ActivateInput, updates interface{}) (*ActivateInput, error)
	ById(id string) (*ActivateInput, error)
	Delete(container *ActivateInput) error
}

func newActivateInputClient(rancherClient *RancherClient) *ActivateInputClient {
	return &ActivateInputClient{
		rancherClient: rancherClient,
	}
}

func (c *ActivateInputClient) Create(container *ActivateInput) (*ActivateInput, error) {
	resp := &ActivateInput{}
	err := c.rancherClient.doCreate(ACTIVATE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *ActivateInputClient) Update(existing *ActivateInput, updates interface{}) (*ActivateInput, error) {
	resp := &ActivateInput{}
	err := c.rancherClient.doUpdate(ACTIVATE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *ActivateInputClient) List(opts *ListOpts) (*ActivateInputCollection, error) {
	resp := &ActivateInputCollection{}
	err := c.rancherClient.doList(ACTIVATE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *ActivateInputCollection) Next() (*ActivateInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &ActivateInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *ActivateInputClient) ById(id string) (*ActivateInput, error) {
	resp := &ActivateInput{}
	err := c.rancherClient.doById(ACTIVATE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *ActivateInputClient) Delete(container *ActivateInput) error {
	return c.rancherClient.doResourceDelete(ACTIVATE_INPUT_TYPE, &container.Resource)
}
