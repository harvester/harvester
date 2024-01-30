package client

const (
	EMPTY_TYPE = "empty"
)

type Empty struct {
	Resource `yaml:"-"`
}

type EmptyCollection struct {
	Collection
	Data   []Empty `json:"data,omitempty"`
	client *EmptyClient
}

type EmptyClient struct {
	rancherClient *RancherClient
}

type EmptyOperations interface {
	List(opts *ListOpts) (*EmptyCollection, error)
	Create(opts *Empty) (*Empty, error)
	Update(existing *Empty, updates interface{}) (*Empty, error)
	ById(id string) (*Empty, error)
	Delete(container *Empty) error
}

func newEmptyClient(rancherClient *RancherClient) *EmptyClient {
	return &EmptyClient{
		rancherClient: rancherClient,
	}
}

func (c *EmptyClient) Create(container *Empty) (*Empty, error) {
	resp := &Empty{}
	err := c.rancherClient.doCreate(EMPTY_TYPE, container, resp)
	return resp, err
}

func (c *EmptyClient) Update(existing *Empty, updates interface{}) (*Empty, error) {
	resp := &Empty{}
	err := c.rancherClient.doUpdate(EMPTY_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *EmptyClient) List(opts *ListOpts) (*EmptyCollection, error) {
	resp := &EmptyCollection{}
	err := c.rancherClient.doList(EMPTY_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *EmptyCollection) Next() (*EmptyCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &EmptyCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *EmptyClient) ById(id string) (*Empty, error) {
	resp := &Empty{}
	err := c.rancherClient.doById(EMPTY_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *EmptyClient) Delete(container *Empty) error {
	return c.rancherClient.doResourceDelete(EMPTY_TYPE, &container.Resource)
}
