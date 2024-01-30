package client

const (
	BACKING_IMAGE_CLEANUP_INPUT_TYPE = "backingImageCleanupInput"
)

type BackingImageCleanupInput struct {
	Resource `yaml:"-"`

	Disks []string `json:"disks,omitempty" yaml:"disks,omitempty"`
}

type BackingImageCleanupInputCollection struct {
	Collection
	Data   []BackingImageCleanupInput `json:"data,omitempty"`
	client *BackingImageCleanupInputClient
}

type BackingImageCleanupInputClient struct {
	rancherClient *RancherClient
}

type BackingImageCleanupInputOperations interface {
	List(opts *ListOpts) (*BackingImageCleanupInputCollection, error)
	Create(opts *BackingImageCleanupInput) (*BackingImageCleanupInput, error)
	Update(existing *BackingImageCleanupInput, updates interface{}) (*BackingImageCleanupInput, error)
	ById(id string) (*BackingImageCleanupInput, error)
	Delete(container *BackingImageCleanupInput) error
}

func newBackingImageCleanupInputClient(rancherClient *RancherClient) *BackingImageCleanupInputClient {
	return &BackingImageCleanupInputClient{
		rancherClient: rancherClient,
	}
}

func (c *BackingImageCleanupInputClient) Create(container *BackingImageCleanupInput) (*BackingImageCleanupInput, error) {
	resp := &BackingImageCleanupInput{}
	err := c.rancherClient.doCreate(BACKING_IMAGE_CLEANUP_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *BackingImageCleanupInputClient) Update(existing *BackingImageCleanupInput, updates interface{}) (*BackingImageCleanupInput, error) {
	resp := &BackingImageCleanupInput{}
	err := c.rancherClient.doUpdate(BACKING_IMAGE_CLEANUP_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *BackingImageCleanupInputClient) List(opts *ListOpts) (*BackingImageCleanupInputCollection, error) {
	resp := &BackingImageCleanupInputCollection{}
	err := c.rancherClient.doList(BACKING_IMAGE_CLEANUP_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *BackingImageCleanupInputCollection) Next() (*BackingImageCleanupInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &BackingImageCleanupInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *BackingImageCleanupInputClient) ById(id string) (*BackingImageCleanupInput, error) {
	resp := &BackingImageCleanupInput{}
	err := c.rancherClient.doById(BACKING_IMAGE_CLEANUP_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *BackingImageCleanupInputClient) Delete(container *BackingImageCleanupInput) error {
	return c.rancherClient.doResourceDelete(BACKING_IMAGE_CLEANUP_INPUT_TYPE, &container.Resource)
}
