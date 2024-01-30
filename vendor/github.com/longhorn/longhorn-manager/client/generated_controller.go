package client

const (
	CONTROLLER_TYPE = "controller"
)

type Controller struct {
	Resource `yaml:"-"`

	ActualSize string `json:"actualSize,omitempty" yaml:"actual_size,omitempty"`

	Address string `json:"address,omitempty" yaml:"address,omitempty"`

	CurrentImage string `json:"currentImage,omitempty" yaml:"current_image,omitempty"`

	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`

	EngineImage string `json:"engineImage,omitempty" yaml:"engine_image,omitempty"`

	HostId string `json:"hostId,omitempty" yaml:"host_id,omitempty"`

	InstanceManagerName string `json:"instanceManagerName,omitempty" yaml:"instance_manager_name,omitempty"`

	IsExpanding bool `json:"isExpanding,omitempty" yaml:"is_expanding,omitempty"`

	LastExpansionError string `json:"lastExpansionError,omitempty" yaml:"last_expansion_error,omitempty"`

	LastExpansionFailedAt string `json:"lastExpansionFailedAt,omitempty" yaml:"last_expansion_failed_at,omitempty"`

	LastRestoredBackup string `json:"lastRestoredBackup,omitempty" yaml:"last_restored_backup,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	RequestedBackupRestore string `json:"requestedBackupRestore,omitempty" yaml:"requested_backup_restore,omitempty"`

	Running bool `json:"running,omitempty" yaml:"running,omitempty"`

	Size string `json:"size,omitempty" yaml:"size,omitempty"`

	UnmapMarkSnapChainRemovedEnabled bool `json:"unmapMarkSnapChainRemovedEnabled,omitempty" yaml:"unmap_mark_snap_chain_removed_enabled,omitempty"`
}

type ControllerCollection struct {
	Collection
	Data   []Controller `json:"data,omitempty"`
	client *ControllerClient
}

type ControllerClient struct {
	rancherClient *RancherClient
}

type ControllerOperations interface {
	List(opts *ListOpts) (*ControllerCollection, error)
	Create(opts *Controller) (*Controller, error)
	Update(existing *Controller, updates interface{}) (*Controller, error)
	ById(id string) (*Controller, error)
	Delete(container *Controller) error
}

func newControllerClient(rancherClient *RancherClient) *ControllerClient {
	return &ControllerClient{
		rancherClient: rancherClient,
	}
}

func (c *ControllerClient) Create(container *Controller) (*Controller, error) {
	resp := &Controller{}
	err := c.rancherClient.doCreate(CONTROLLER_TYPE, container, resp)
	return resp, err
}

func (c *ControllerClient) Update(existing *Controller, updates interface{}) (*Controller, error) {
	resp := &Controller{}
	err := c.rancherClient.doUpdate(CONTROLLER_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *ControllerClient) List(opts *ListOpts) (*ControllerCollection, error) {
	resp := &ControllerCollection{}
	err := c.rancherClient.doList(CONTROLLER_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *ControllerCollection) Next() (*ControllerCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &ControllerCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *ControllerClient) ById(id string) (*Controller, error) {
	resp := &Controller{}
	err := c.rancherClient.doById(CONTROLLER_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *ControllerClient) Delete(container *Controller) error {
	return c.rancherClient.doResourceDelete(CONTROLLER_TYPE, &container.Resource)
}
