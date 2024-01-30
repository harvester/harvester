package client

const (
	ATTACHMENT_TYPE = "attachment"
)

type Attachment struct {
	Resource `yaml:"-"`

	AttachmentID string `json:"attachmentID,omitempty" yaml:"attachment_id,omitempty"`

	AttachmentType string `json:"attachmentType,omitempty" yaml:"attachment_type,omitempty"`

	Conditions []LonghornCondition `json:"conditions,omitempty" yaml:"conditions,omitempty"`

	NodeID string `json:"nodeID,omitempty" yaml:"node_id,omitempty"`

	Parameters map[string]string `json:"parameters,omitempty" yaml:"parameters,omitempty"`

	Satisfied bool `json:"satisfied,omitempty" yaml:"satisfied,omitempty"`
}

type AttachmentCollection struct {
	Collection
	Data   []Attachment `json:"data,omitempty"`
	client *AttachmentClient
}

type AttachmentClient struct {
	rancherClient *RancherClient
}

type AttachmentOperations interface {
	List(opts *ListOpts) (*AttachmentCollection, error)
	Create(opts *Attachment) (*Attachment, error)
	Update(existing *Attachment, updates interface{}) (*Attachment, error)
	ById(id string) (*Attachment, error)
	Delete(container *Attachment) error
}

func newAttachmentClient(rancherClient *RancherClient) *AttachmentClient {
	return &AttachmentClient{
		rancherClient: rancherClient,
	}
}

func (c *AttachmentClient) Create(container *Attachment) (*Attachment, error) {
	resp := &Attachment{}
	err := c.rancherClient.doCreate(ATTACHMENT_TYPE, container, resp)
	return resp, err
}

func (c *AttachmentClient) Update(existing *Attachment, updates interface{}) (*Attachment, error) {
	resp := &Attachment{}
	err := c.rancherClient.doUpdate(ATTACHMENT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *AttachmentClient) List(opts *ListOpts) (*AttachmentCollection, error) {
	resp := &AttachmentCollection{}
	err := c.rancherClient.doList(ATTACHMENT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *AttachmentCollection) Next() (*AttachmentCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &AttachmentCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *AttachmentClient) ById(id string) (*Attachment, error) {
	resp := &Attachment{}
	err := c.rancherClient.doById(ATTACHMENT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *AttachmentClient) Delete(container *Attachment) error {
	return c.rancherClient.doResourceDelete(ATTACHMENT_TYPE, &container.Resource)
}
