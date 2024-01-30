package client

const (
	DISK_INFO_TYPE = "diskInfo"
)

type DiskInfo struct {
	Resource `yaml:"-"`

	AllowScheduling bool `json:"allowScheduling,omitempty" yaml:"allow_scheduling,omitempty"`

	Conditions map[string]interface{} `json:"conditions,omitempty" yaml:"conditions,omitempty"`

	DiskType string `json:"diskType,omitempty" yaml:"disk_type,omitempty"`

	DiskUUID string `json:"diskUUID,omitempty" yaml:"disk_uuid,omitempty"`

	EvictionRequested bool `json:"evictionRequested,omitempty" yaml:"eviction_requested,omitempty"`

	Path string `json:"path,omitempty" yaml:"path,omitempty"`

	ScheduledReplica map[string]string `json:"scheduledReplica,omitempty" yaml:"scheduled_replica,omitempty"`

	StorageAvailable int64 `json:"storageAvailable,omitempty" yaml:"storage_available,omitempty"`

	StorageMaximum int64 `json:"storageMaximum,omitempty" yaml:"storage_maximum,omitempty"`

	StorageReserved int64 `json:"storageReserved,omitempty" yaml:"storage_reserved,omitempty"`

	StorageScheduled int64 `json:"storageScheduled,omitempty" yaml:"storage_scheduled,omitempty"`

	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

type DiskInfoCollection struct {
	Collection
	Data   []DiskInfo `json:"data,omitempty"`
	client *DiskInfoClient
}

type DiskInfoClient struct {
	rancherClient *RancherClient
}

type DiskInfoOperations interface {
	List(opts *ListOpts) (*DiskInfoCollection, error)
	Create(opts *DiskInfo) (*DiskInfo, error)
	Update(existing *DiskInfo, updates interface{}) (*DiskInfo, error)
	ById(id string) (*DiskInfo, error)
	Delete(container *DiskInfo) error
}

func newDiskInfoClient(rancherClient *RancherClient) *DiskInfoClient {
	return &DiskInfoClient{
		rancherClient: rancherClient,
	}
}

func (c *DiskInfoClient) Create(container *DiskInfo) (*DiskInfo, error) {
	resp := &DiskInfo{}
	err := c.rancherClient.doCreate(DISK_INFO_TYPE, container, resp)
	return resp, err
}

func (c *DiskInfoClient) Update(existing *DiskInfo, updates interface{}) (*DiskInfo, error) {
	resp := &DiskInfo{}
	err := c.rancherClient.doUpdate(DISK_INFO_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *DiskInfoClient) List(opts *ListOpts) (*DiskInfoCollection, error) {
	resp := &DiskInfoCollection{}
	err := c.rancherClient.doList(DISK_INFO_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *DiskInfoCollection) Next() (*DiskInfoCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &DiskInfoCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *DiskInfoClient) ById(id string) (*DiskInfo, error) {
	resp := &DiskInfo{}
	err := c.rancherClient.doById(DISK_INFO_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *DiskInfoClient) Delete(container *DiskInfo) error {
	return c.rancherClient.doResourceDelete(DISK_INFO_TYPE, &container.Resource)
}
