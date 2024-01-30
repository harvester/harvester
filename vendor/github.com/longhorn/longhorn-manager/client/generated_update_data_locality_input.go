package client

const (
	UPDATE_DATA_LOCALITY_INPUT_TYPE = "UpdateDataLocalityInput"
)

type UpdateDataLocalityInput struct {
	Resource `yaml:"-"`

	DataLocality string `json:"dataLocality,omitempty" yaml:"data_locality,omitempty"`
}

type UpdateDataLocalityInputCollection struct {
	Collection
	Data   []UpdateDataLocalityInput `json:"data,omitempty"`
	client *UpdateDataLocalityInputClient
}

type UpdateDataLocalityInputClient struct {
	rancherClient *RancherClient
}

type UpdateDataLocalityInputOperations interface {
	List(opts *ListOpts) (*UpdateDataLocalityInputCollection, error)
	Create(opts *UpdateDataLocalityInput) (*UpdateDataLocalityInput, error)
	Update(existing *UpdateDataLocalityInput, updates interface{}) (*UpdateDataLocalityInput, error)
	ById(id string) (*UpdateDataLocalityInput, error)
	Delete(container *UpdateDataLocalityInput) error
}

func newUpdateDataLocalityInputClient(rancherClient *RancherClient) *UpdateDataLocalityInputClient {
	return &UpdateDataLocalityInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateDataLocalityInputClient) Create(container *UpdateDataLocalityInput) (*UpdateDataLocalityInput, error) {
	resp := &UpdateDataLocalityInput{}
	err := c.rancherClient.doCreate(UPDATE_DATA_LOCALITY_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateDataLocalityInputClient) Update(existing *UpdateDataLocalityInput, updates interface{}) (*UpdateDataLocalityInput, error) {
	resp := &UpdateDataLocalityInput{}
	err := c.rancherClient.doUpdate(UPDATE_DATA_LOCALITY_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateDataLocalityInputClient) List(opts *ListOpts) (*UpdateDataLocalityInputCollection, error) {
	resp := &UpdateDataLocalityInputCollection{}
	err := c.rancherClient.doList(UPDATE_DATA_LOCALITY_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateDataLocalityInputCollection) Next() (*UpdateDataLocalityInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateDataLocalityInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateDataLocalityInputClient) ById(id string) (*UpdateDataLocalityInput, error) {
	resp := &UpdateDataLocalityInput{}
	err := c.rancherClient.doById(UPDATE_DATA_LOCALITY_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateDataLocalityInputClient) Delete(container *UpdateDataLocalityInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_DATA_LOCALITY_INPUT_TYPE, &container.Resource)
}
