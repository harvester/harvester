package client

const (
	DISK_UPDATE_INPUT_TYPE = "diskUpdateInput"
)

type DiskUpdateInput struct {
	Resource `yaml:"-"`

	Disks []DiskUpdate `json:"disks,omitempty" yaml:"disks,omitempty"`
}

type DiskUpdateInputCollection struct {
	Collection
	Data   []DiskUpdateInput `json:"data,omitempty"`
	client *DiskUpdateInputClient
}

type DiskUpdateInputClient struct {
	rancherClient *RancherClient
}

type DiskUpdateInputOperations interface {
	List(opts *ListOpts) (*DiskUpdateInputCollection, error)
	Create(opts *DiskUpdateInput) (*DiskUpdateInput, error)
	Update(existing *DiskUpdateInput, updates interface{}) (*DiskUpdateInput, error)
	ById(id string) (*DiskUpdateInput, error)
	Delete(container *DiskUpdateInput) error
}

func newDiskUpdateInputClient(rancherClient *RancherClient) *DiskUpdateInputClient {
	return &DiskUpdateInputClient{
		rancherClient: rancherClient,
	}
}

func (c *DiskUpdateInputClient) Create(container *DiskUpdateInput) (*DiskUpdateInput, error) {
	resp := &DiskUpdateInput{}
	err := c.rancherClient.doCreate(DISK_UPDATE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *DiskUpdateInputClient) Update(existing *DiskUpdateInput, updates interface{}) (*DiskUpdateInput, error) {
	resp := &DiskUpdateInput{}
	err := c.rancherClient.doUpdate(DISK_UPDATE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *DiskUpdateInputClient) List(opts *ListOpts) (*DiskUpdateInputCollection, error) {
	resp := &DiskUpdateInputCollection{}
	err := c.rancherClient.doList(DISK_UPDATE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *DiskUpdateInputCollection) Next() (*DiskUpdateInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &DiskUpdateInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *DiskUpdateInputClient) ById(id string) (*DiskUpdateInput, error) {
	resp := &DiskUpdateInput{}
	err := c.rancherClient.doById(DISK_UPDATE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *DiskUpdateInputClient) Delete(container *DiskUpdateInput) error {
	return c.rancherClient.doResourceDelete(DISK_UPDATE_INPUT_TYPE, &container.Resource)
}
