package client

const (
	DISK_UPDATE_TYPE = "diskUpdate"
)

type DiskUpdate struct {
	Resource `yaml:"-"`

	AllowScheduling bool `json:"allowScheduling,omitempty" yaml:"allow_scheduling,omitempty"`

	DiskType string `json:"diskType,omitempty" yaml:"disk_type,omitempty"`

	EvictionRequested bool `json:"evictionRequested,omitempty" yaml:"eviction_requested,omitempty"`

	Path string `json:"path,omitempty" yaml:"path,omitempty"`

	StorageReserved int64 `json:"storageReserved,omitempty" yaml:"storage_reserved,omitempty"`

	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

type DiskUpdateCollection struct {
	Collection
	Data   []DiskUpdate `json:"data,omitempty"`
	client *DiskUpdateClient
}

type DiskUpdateClient struct {
	rancherClient *RancherClient
}

type DiskUpdateOperations interface {
	List(opts *ListOpts) (*DiskUpdateCollection, error)
	Create(opts *DiskUpdate) (*DiskUpdate, error)
	Update(existing *DiskUpdate, updates interface{}) (*DiskUpdate, error)
	ById(id string) (*DiskUpdate, error)
	Delete(container *DiskUpdate) error
}

func newDiskUpdateClient(rancherClient *RancherClient) *DiskUpdateClient {
	return &DiskUpdateClient{
		rancherClient: rancherClient,
	}
}

func (c *DiskUpdateClient) Create(container *DiskUpdate) (*DiskUpdate, error) {
	resp := &DiskUpdate{}
	err := c.rancherClient.doCreate(DISK_UPDATE_TYPE, container, resp)
	return resp, err
}

func (c *DiskUpdateClient) Update(existing *DiskUpdate, updates interface{}) (*DiskUpdate, error) {
	resp := &DiskUpdate{}
	err := c.rancherClient.doUpdate(DISK_UPDATE_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *DiskUpdateClient) List(opts *ListOpts) (*DiskUpdateCollection, error) {
	resp := &DiskUpdateCollection{}
	err := c.rancherClient.doList(DISK_UPDATE_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *DiskUpdateCollection) Next() (*DiskUpdateCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &DiskUpdateCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *DiskUpdateClient) ById(id string) (*DiskUpdate, error) {
	resp := &DiskUpdate{}
	err := c.rancherClient.doById(DISK_UPDATE_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *DiskUpdateClient) Delete(container *DiskUpdate) error {
	return c.rancherClient.doResourceDelete(DISK_UPDATE_TYPE, &container.Resource)
}
