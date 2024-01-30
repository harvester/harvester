package client

const (
	ERROR_TYPE = "error"
)

type Error struct {
	Resource `yaml:"-"`

	Code string `json:"code,omitempty" yaml:"code,omitempty"`

	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`

	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	Status int64 `json:"status,omitempty" yaml:"status,omitempty"`
}

type ErrorCollection struct {
	Collection
	Data   []Error `json:"data,omitempty"`
	client *ErrorClient
}

type ErrorClient struct {
	rancherClient *RancherClient
}

type ErrorOperations interface {
	List(opts *ListOpts) (*ErrorCollection, error)
	Create(opts *Error) (*Error, error)
	Update(existing *Error, updates interface{}) (*Error, error)
	ById(id string) (*Error, error)
	Delete(container *Error) error
}

func newErrorClient(rancherClient *RancherClient) *ErrorClient {
	return &ErrorClient{
		rancherClient: rancherClient,
	}
}

func (c *ErrorClient) Create(container *Error) (*Error, error) {
	resp := &Error{}
	err := c.rancherClient.doCreate(ERROR_TYPE, container, resp)
	return resp, err
}

func (c *ErrorClient) Update(existing *Error, updates interface{}) (*Error, error) {
	resp := &Error{}
	err := c.rancherClient.doUpdate(ERROR_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *ErrorClient) List(opts *ListOpts) (*ErrorCollection, error) {
	resp := &ErrorCollection{}
	err := c.rancherClient.doList(ERROR_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *ErrorCollection) Next() (*ErrorCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &ErrorCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *ErrorClient) ById(id string) (*Error, error) {
	resp := &Error{}
	err := c.rancherClient.doById(ERROR_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *ErrorClient) Delete(container *Error) error {
	return c.rancherClient.doResourceDelete(ERROR_TYPE, &container.Resource)
}
