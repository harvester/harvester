package client

const (
	SNAPSHOT_CR_TYPE = "snapshotCR"
)

type SnapshotCR struct {
	Resource `yaml:"-"`

	Checksum string `json:"checksum,omitempty" yaml:"checksum,omitempty"`

	Children map[string]interface{} `json:"children,omitempty" yaml:"children,omitempty"`

	CrCreationTime string `json:"crCreationTime,omitempty" yaml:"cr_creation_time,omitempty"`

	CreateSnapshot bool `json:"createSnapshot,omitempty" yaml:"create_snapshot,omitempty"`

	CreationTime string `json:"creationTime,omitempty" yaml:"creation_time,omitempty"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	MarkRemoved bool `json:"markRemoved,omitempty" yaml:"mark_removed,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	OwnerID string `json:"ownerID,omitempty" yaml:"owner_id,omitempty"`

	Parent string `json:"parent,omitempty" yaml:"parent,omitempty"`

	ReadyToUse bool `json:"readyToUse,omitempty" yaml:"ready_to_use,omitempty"`

	RestoreSize int64 `json:"restoreSize,omitempty" yaml:"restore_size,omitempty"`

	Size int64 `json:"size,omitempty" yaml:"size,omitempty"`

	UserCreated bool `json:"userCreated,omitempty" yaml:"user_created,omitempty"`

	Volume string `json:"volume,omitempty" yaml:"volume,omitempty"`
}

type SnapshotCRCollection struct {
	Collection
	Data   []SnapshotCR `json:"data,omitempty"`
	client *SnapshotCRClient
}

type SnapshotCRClient struct {
	rancherClient *RancherClient
}

type SnapshotCROperations interface {
	List(opts *ListOpts) (*SnapshotCRCollection, error)
	Create(opts *SnapshotCR) (*SnapshotCR, error)
	Update(existing *SnapshotCR, updates interface{}) (*SnapshotCR, error)
	ById(id string) (*SnapshotCR, error)
	Delete(container *SnapshotCR) error
}

func newSnapshotCRClient(rancherClient *RancherClient) *SnapshotCRClient {
	return &SnapshotCRClient{
		rancherClient: rancherClient,
	}
}

func (c *SnapshotCRClient) Create(container *SnapshotCR) (*SnapshotCR, error) {
	resp := &SnapshotCR{}
	err := c.rancherClient.doCreate(SNAPSHOT_CR_TYPE, container, resp)
	return resp, err
}

func (c *SnapshotCRClient) Update(existing *SnapshotCR, updates interface{}) (*SnapshotCR, error) {
	resp := &SnapshotCR{}
	err := c.rancherClient.doUpdate(SNAPSHOT_CR_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SnapshotCRClient) List(opts *ListOpts) (*SnapshotCRCollection, error) {
	resp := &SnapshotCRCollection{}
	err := c.rancherClient.doList(SNAPSHOT_CR_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SnapshotCRCollection) Next() (*SnapshotCRCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SnapshotCRCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SnapshotCRClient) ById(id string) (*SnapshotCR, error) {
	resp := &SnapshotCR{}
	err := c.rancherClient.doById(SNAPSHOT_CR_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SnapshotCRClient) Delete(container *SnapshotCR) error {
	return c.rancherClient.doResourceDelete(SNAPSHOT_CR_TYPE, &container.Resource)
}
