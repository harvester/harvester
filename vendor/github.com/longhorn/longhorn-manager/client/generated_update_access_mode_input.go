package client

const (
	UPDATE_ACCESS_MODE_INPUT_TYPE = "UpdateAccessModeInput"
)

type UpdateAccessModeInput struct {
	Resource `yaml:"-"`

	AccessMode string `json:"accessMode,omitempty" yaml:"access_mode,omitempty"`
}

type UpdateAccessModeInputCollection struct {
	Collection
	Data   []UpdateAccessModeInput `json:"data,omitempty"`
	client *UpdateAccessModeInputClient
}

type UpdateAccessModeInputClient struct {
	rancherClient *RancherClient
}

type UpdateAccessModeInputOperations interface {
	List(opts *ListOpts) (*UpdateAccessModeInputCollection, error)
	Create(opts *UpdateAccessModeInput) (*UpdateAccessModeInput, error)
	Update(existing *UpdateAccessModeInput, updates interface{}) (*UpdateAccessModeInput, error)
	ById(id string) (*UpdateAccessModeInput, error)
	Delete(container *UpdateAccessModeInput) error
}

func newUpdateAccessModeInputClient(rancherClient *RancherClient) *UpdateAccessModeInputClient {
	return &UpdateAccessModeInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateAccessModeInputClient) Create(container *UpdateAccessModeInput) (*UpdateAccessModeInput, error) {
	resp := &UpdateAccessModeInput{}
	err := c.rancherClient.doCreate(UPDATE_ACCESS_MODE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateAccessModeInputClient) Update(existing *UpdateAccessModeInput, updates interface{}) (*UpdateAccessModeInput, error) {
	resp := &UpdateAccessModeInput{}
	err := c.rancherClient.doUpdate(UPDATE_ACCESS_MODE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateAccessModeInputClient) List(opts *ListOpts) (*UpdateAccessModeInputCollection, error) {
	resp := &UpdateAccessModeInputCollection{}
	err := c.rancherClient.doList(UPDATE_ACCESS_MODE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateAccessModeInputCollection) Next() (*UpdateAccessModeInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateAccessModeInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateAccessModeInputClient) ById(id string) (*UpdateAccessModeInput, error) {
	resp := &UpdateAccessModeInput{}
	err := c.rancherClient.doById(UPDATE_ACCESS_MODE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateAccessModeInputClient) Delete(container *UpdateAccessModeInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_ACCESS_MODE_INPUT_TYPE, &container.Resource)
}
