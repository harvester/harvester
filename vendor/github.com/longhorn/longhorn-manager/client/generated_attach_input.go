package client

const (
	ATTACH_INPUT_TYPE = "attachInput"
)

type AttachInput struct {
	Resource `yaml:"-"`

	AttachedBy string `json:"attachedBy,omitempty" yaml:"attached_by,omitempty"`

	AttacherType string `json:"attacherType,omitempty" yaml:"attacher_type,omitempty"`

	AttachmentID string `json:"attachmentID,omitempty" yaml:"attachment_id,omitempty"`

	DisableFrontend bool `json:"disableFrontend,omitempty" yaml:"disable_frontend,omitempty"`

	HostId string `json:"hostId,omitempty" yaml:"host_id,omitempty"`
}

type AttachInputCollection struct {
	Collection
	Data   []AttachInput `json:"data,omitempty"`
	client *AttachInputClient
}

type AttachInputClient struct {
	rancherClient *RancherClient
}

type AttachInputOperations interface {
	List(opts *ListOpts) (*AttachInputCollection, error)
	Create(opts *AttachInput) (*AttachInput, error)
	Update(existing *AttachInput, updates interface{}) (*AttachInput, error)
	ById(id string) (*AttachInput, error)
	Delete(container *AttachInput) error
}

func newAttachInputClient(rancherClient *RancherClient) *AttachInputClient {
	return &AttachInputClient{
		rancherClient: rancherClient,
	}
}

func (c *AttachInputClient) Create(container *AttachInput) (*AttachInput, error) {
	resp := &AttachInput{}
	err := c.rancherClient.doCreate(ATTACH_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *AttachInputClient) Update(existing *AttachInput, updates interface{}) (*AttachInput, error) {
	resp := &AttachInput{}
	err := c.rancherClient.doUpdate(ATTACH_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *AttachInputClient) List(opts *ListOpts) (*AttachInputCollection, error) {
	resp := &AttachInputCollection{}
	err := c.rancherClient.doList(ATTACH_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *AttachInputCollection) Next() (*AttachInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &AttachInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *AttachInputClient) ById(id string) (*AttachInput, error) {
	resp := &AttachInput{}
	err := c.rancherClient.doById(ATTACH_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *AttachInputClient) Delete(container *AttachInput) error {
	return c.rancherClient.doResourceDelete(ATTACH_INPUT_TYPE, &container.Resource)
}
