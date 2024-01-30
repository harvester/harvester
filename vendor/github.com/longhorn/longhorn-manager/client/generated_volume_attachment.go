package client

const (
	VOLUME_ATTACHMENT_TYPE = "volumeAttachment"
)

type VolumeAttachment struct {
	Resource `yaml:"-"`

	Attachments map[string]Attachment `json:"attachments,omitempty" yaml:"attachments,omitempty"`

	Volume string `json:"volume,omitempty" yaml:"volume,omitempty"`
}

type VolumeAttachmentCollection struct {
	Collection
	Data   []VolumeAttachment `json:"data,omitempty"`
	client *VolumeAttachmentClient
}

type VolumeAttachmentClient struct {
	rancherClient *RancherClient
}

type VolumeAttachmentOperations interface {
	List(opts *ListOpts) (*VolumeAttachmentCollection, error)
	Create(opts *VolumeAttachment) (*VolumeAttachment, error)
	Update(existing *VolumeAttachment, updates interface{}) (*VolumeAttachment, error)
	ById(id string) (*VolumeAttachment, error)
	Delete(container *VolumeAttachment) error
}

func newVolumeAttachmentClient(rancherClient *RancherClient) *VolumeAttachmentClient {
	return &VolumeAttachmentClient{
		rancherClient: rancherClient,
	}
}

func (c *VolumeAttachmentClient) Create(container *VolumeAttachment) (*VolumeAttachment, error) {
	resp := &VolumeAttachment{}
	err := c.rancherClient.doCreate(VOLUME_ATTACHMENT_TYPE, container, resp)
	return resp, err
}

func (c *VolumeAttachmentClient) Update(existing *VolumeAttachment, updates interface{}) (*VolumeAttachment, error) {
	resp := &VolumeAttachment{}
	err := c.rancherClient.doUpdate(VOLUME_ATTACHMENT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *VolumeAttachmentClient) List(opts *ListOpts) (*VolumeAttachmentCollection, error) {
	resp := &VolumeAttachmentCollection{}
	err := c.rancherClient.doList(VOLUME_ATTACHMENT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *VolumeAttachmentCollection) Next() (*VolumeAttachmentCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &VolumeAttachmentCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *VolumeAttachmentClient) ById(id string) (*VolumeAttachment, error) {
	resp := &VolumeAttachment{}
	err := c.rancherClient.doById(VOLUME_ATTACHMENT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *VolumeAttachmentClient) Delete(container *VolumeAttachment) error {
	return c.rancherClient.doResourceDelete(VOLUME_ATTACHMENT_TYPE, &container.Resource)
}
