package client

const (
	REPLICA_TYPE = "replica"
)

type Replica struct {
	Resource `yaml:"-"`

	Address string `json:"address,omitempty" yaml:"address,omitempty"`

	BackendStoreDriver string `json:"backendStoreDriver,omitempty" yaml:"backend_store_driver,omitempty"`

	CurrentImage string `json:"currentImage,omitempty" yaml:"current_image,omitempty"`

	DataPath string `json:"dataPath,omitempty" yaml:"data_path,omitempty"`

	DiskID string `json:"diskID,omitempty" yaml:"disk_id,omitempty"`

	DiskPath string `json:"diskPath,omitempty" yaml:"disk_path,omitempty"`

	EngineImage string `json:"engineImage,omitempty" yaml:"engine_image,omitempty"`

	FailedAt string `json:"failedAt,omitempty" yaml:"failed_at,omitempty"`

	HostId string `json:"hostId,omitempty" yaml:"host_id,omitempty"`

	InstanceManagerName string `json:"instanceManagerName,omitempty" yaml:"instance_manager_name,omitempty"`

	Mode string `json:"mode,omitempty" yaml:"mode,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	Running bool `json:"running,omitempty" yaml:"running,omitempty"`
}

type ReplicaCollection struct {
	Collection
	Data   []Replica `json:"data,omitempty"`
	client *ReplicaClient
}

type ReplicaClient struct {
	rancherClient *RancherClient
}

type ReplicaOperations interface {
	List(opts *ListOpts) (*ReplicaCollection, error)
	Create(opts *Replica) (*Replica, error)
	Update(existing *Replica, updates interface{}) (*Replica, error)
	ById(id string) (*Replica, error)
	Delete(container *Replica) error
}

func newReplicaClient(rancherClient *RancherClient) *ReplicaClient {
	return &ReplicaClient{
		rancherClient: rancherClient,
	}
}

func (c *ReplicaClient) Create(container *Replica) (*Replica, error) {
	resp := &Replica{}
	err := c.rancherClient.doCreate(REPLICA_TYPE, container, resp)
	return resp, err
}

func (c *ReplicaClient) Update(existing *Replica, updates interface{}) (*Replica, error) {
	resp := &Replica{}
	err := c.rancherClient.doUpdate(REPLICA_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *ReplicaClient) List(opts *ListOpts) (*ReplicaCollection, error) {
	resp := &ReplicaCollection{}
	err := c.rancherClient.doList(REPLICA_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *ReplicaCollection) Next() (*ReplicaCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &ReplicaCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *ReplicaClient) ById(id string) (*Replica, error) {
	resp := &Replica{}
	err := c.rancherClient.doById(REPLICA_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *ReplicaClient) Delete(container *Replica) error {
	return c.rancherClient.doResourceDelete(REPLICA_TYPE, &container.Resource)
}
