package client

const (
	SNAPSHOT_TYPE = "snapshot"
)

type Snapshot struct {
	Resource `yaml:"-"`

	Checksum string `json:"checksum,omitempty" yaml:"checksum,omitempty"`

	Children map[string]interface{} `json:"children,omitempty" yaml:"children,omitempty"`

	Created string `json:"created,omitempty" yaml:"created,omitempty"`

	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	Parent string `json:"parent,omitempty" yaml:"parent,omitempty"`

	Removed bool `json:"removed,omitempty" yaml:"removed,omitempty"`

	Size string `json:"size,omitempty" yaml:"size,omitempty"`

	Usercreated bool `json:"usercreated,omitempty" yaml:"usercreated,omitempty"`
}

type SnapshotCollection struct {
	Collection
	Data   []Snapshot `json:"data,omitempty"`
	client *SnapshotClient
}

type SnapshotClient struct {
	rancherClient *RancherClient
}

type SnapshotOperations interface {
	List(opts *ListOpts) (*SnapshotCollection, error)
	Create(opts *Snapshot) (*Snapshot, error)
	Update(existing *Snapshot, updates interface{}) (*Snapshot, error)
	ById(id string) (*Snapshot, error)
	Delete(container *Snapshot) error
}

func newSnapshotClient(rancherClient *RancherClient) *SnapshotClient {
	return &SnapshotClient{
		rancherClient: rancherClient,
	}
}

func (c *SnapshotClient) Create(container *Snapshot) (*Snapshot, error) {
	resp := &Snapshot{}
	err := c.rancherClient.doCreate(SNAPSHOT_TYPE, container, resp)
	return resp, err
}

func (c *SnapshotClient) Update(existing *Snapshot, updates interface{}) (*Snapshot, error) {
	resp := &Snapshot{}
	err := c.rancherClient.doUpdate(SNAPSHOT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SnapshotClient) List(opts *ListOpts) (*SnapshotCollection, error) {
	resp := &SnapshotCollection{}
	err := c.rancherClient.doList(SNAPSHOT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SnapshotCollection) Next() (*SnapshotCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SnapshotCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SnapshotClient) ById(id string) (*Snapshot, error) {
	resp := &Snapshot{}
	err := c.rancherClient.doById(SNAPSHOT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SnapshotClient) Delete(container *Snapshot) error {
	return c.rancherClient.doResourceDelete(SNAPSHOT_TYPE, &container.Resource)
}
