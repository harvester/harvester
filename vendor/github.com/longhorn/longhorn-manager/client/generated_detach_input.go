package client

const (
	DETACH_INPUT_TYPE = "detachInput"
)

type DetachInput struct {
	Resource `yaml:"-"`

	AttachmentID string `json:"attachmentID,omitempty" yaml:"attachment_id,omitempty"`

	ForceDetach bool `json:"forceDetach,omitempty" yaml:"force_detach,omitempty"`

	HostId string `json:"hostId,omitempty" yaml:"host_id,omitempty"`
}

type DetachInputCollection struct {
	Collection
	Data   []DetachInput `json:"data,omitempty"`
	client *DetachInputClient
}

type DetachInputClient struct {
	rancherClient *RancherClient
}

type DetachInputOperations interface {
	List(opts *ListOpts) (*DetachInputCollection, error)
	Create(opts *DetachInput) (*DetachInput, error)
	Update(existing *DetachInput, updates interface{}) (*DetachInput, error)
	ById(id string) (*DetachInput, error)
	Delete(container *DetachInput) error
}

func newDetachInputClient(rancherClient *RancherClient) *DetachInputClient {
	return &DetachInputClient{
		rancherClient: rancherClient,
	}
}

func (c *DetachInputClient) Create(container *DetachInput) (*DetachInput, error) {
	resp := &DetachInput{}
	err := c.rancherClient.doCreate(DETACH_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *DetachInputClient) Update(existing *DetachInput, updates interface{}) (*DetachInput, error) {
	resp := &DetachInput{}
	err := c.rancherClient.doUpdate(DETACH_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *DetachInputClient) List(opts *ListOpts) (*DetachInputCollection, error) {
	resp := &DetachInputCollection{}
	err := c.rancherClient.doList(DETACH_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *DetachInputCollection) Next() (*DetachInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &DetachInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *DetachInputClient) ById(id string) (*DetachInput, error) {
	resp := &DetachInput{}
	err := c.rancherClient.doById(DETACH_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *DetachInputClient) Delete(container *DetachInput) error {
	return c.rancherClient.doResourceDelete(DETACH_INPUT_TYPE, &container.Resource)
}
