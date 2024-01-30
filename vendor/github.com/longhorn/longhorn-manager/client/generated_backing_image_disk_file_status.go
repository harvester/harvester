package client

const (
	BACKING_IMAGE_DISK_FILE_STATUS_TYPE = "backingImageDiskFileStatus"
)

type BackingImageDiskFileStatus struct {
	Resource `yaml:"-"`

	LastStateTransitionTime string `json:"lastStateTransitionTime,omitempty" yaml:"last_state_transition_time,omitempty"`

	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	Progress int64 `json:"progress,omitempty" yaml:"progress,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`
}

type BackingImageDiskFileStatusCollection struct {
	Collection
	Data   []BackingImageDiskFileStatus `json:"data,omitempty"`
	client *BackingImageDiskFileStatusClient
}

type BackingImageDiskFileStatusClient struct {
	rancherClient *RancherClient
}

type BackingImageDiskFileStatusOperations interface {
	List(opts *ListOpts) (*BackingImageDiskFileStatusCollection, error)
	Create(opts *BackingImageDiskFileStatus) (*BackingImageDiskFileStatus, error)
	Update(existing *BackingImageDiskFileStatus, updates interface{}) (*BackingImageDiskFileStatus, error)
	ById(id string) (*BackingImageDiskFileStatus, error)
	Delete(container *BackingImageDiskFileStatus) error
}

func newBackingImageDiskFileStatusClient(rancherClient *RancherClient) *BackingImageDiskFileStatusClient {
	return &BackingImageDiskFileStatusClient{
		rancherClient: rancherClient,
	}
}

func (c *BackingImageDiskFileStatusClient) Create(container *BackingImageDiskFileStatus) (*BackingImageDiskFileStatus, error) {
	resp := &BackingImageDiskFileStatus{}
	err := c.rancherClient.doCreate(BACKING_IMAGE_DISK_FILE_STATUS_TYPE, container, resp)
	return resp, err
}

func (c *BackingImageDiskFileStatusClient) Update(existing *BackingImageDiskFileStatus, updates interface{}) (*BackingImageDiskFileStatus, error) {
	resp := &BackingImageDiskFileStatus{}
	err := c.rancherClient.doUpdate(BACKING_IMAGE_DISK_FILE_STATUS_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *BackingImageDiskFileStatusClient) List(opts *ListOpts) (*BackingImageDiskFileStatusCollection, error) {
	resp := &BackingImageDiskFileStatusCollection{}
	err := c.rancherClient.doList(BACKING_IMAGE_DISK_FILE_STATUS_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *BackingImageDiskFileStatusCollection) Next() (*BackingImageDiskFileStatusCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &BackingImageDiskFileStatusCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *BackingImageDiskFileStatusClient) ById(id string) (*BackingImageDiskFileStatus, error) {
	resp := &BackingImageDiskFileStatus{}
	err := c.rancherClient.doById(BACKING_IMAGE_DISK_FILE_STATUS_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *BackingImageDiskFileStatusClient) Delete(container *BackingImageDiskFileStatus) error {
	return c.rancherClient.doResourceDelete(BACKING_IMAGE_DISK_FILE_STATUS_TYPE, &container.Resource)
}
