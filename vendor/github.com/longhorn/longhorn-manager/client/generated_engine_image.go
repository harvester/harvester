package client

const (
	ENGINE_IMAGE_TYPE = "engineImage"
)

type EngineImage struct {
	Resource `yaml:"-"`

	BuildDate string `json:"buildDate,omitempty" yaml:"build_date,omitempty"`

	CliAPIMinVersion int64 `json:"cliAPIMinVersion,omitempty" yaml:"cli_apimin_version,omitempty"`

	CliAPIVersion int64 `json:"cliAPIVersion,omitempty" yaml:"cli_apiversion,omitempty"`

	Conditions []string `json:"conditions,omitempty" yaml:"conditions,omitempty"`

	ControllerAPIMinVersion int64 `json:"controllerAPIMinVersion,omitempty" yaml:"controller_apimin_version,omitempty"`

	ControllerAPIVersion int64 `json:"controllerAPIVersion,omitempty" yaml:"controller_apiversion,omitempty"`

	DataFormatMinVersion int64 `json:"dataFormatMinVersion,omitempty" yaml:"data_format_min_version,omitempty"`

	DataFormatVersion int64 `json:"dataFormatVersion,omitempty" yaml:"data_format_version,omitempty"`

	Default bool `json:"default,omitempty" yaml:"default,omitempty"`

	GitCommit string `json:"gitCommit,omitempty" yaml:"git_commit,omitempty"`

	Image string `json:"image,omitempty" yaml:"image,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	NoRefSince string `json:"noRefSince,omitempty" yaml:"no_ref_since,omitempty"`

	NodeDeploymentMap map[string]bool `json:"nodeDeploymentMap,omitempty" yaml:"node_deployment_map,omitempty"`

	OwnerID string `json:"ownerID,omitempty" yaml:"owner_id,omitempty"`

	RefCount int64 `json:"refCount,omitempty" yaml:"ref_count,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`

	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

type EngineImageCollection struct {
	Collection
	Data   []EngineImage `json:"data,omitempty"`
	client *EngineImageClient
}

type EngineImageClient struct {
	rancherClient *RancherClient
}

type EngineImageOperations interface {
	List(opts *ListOpts) (*EngineImageCollection, error)
	Create(opts *EngineImage) (*EngineImage, error)
	Update(existing *EngineImage, updates interface{}) (*EngineImage, error)
	ById(id string) (*EngineImage, error)
	Delete(container *EngineImage) error
}

func newEngineImageClient(rancherClient *RancherClient) *EngineImageClient {
	return &EngineImageClient{
		rancherClient: rancherClient,
	}
}

func (c *EngineImageClient) Create(container *EngineImage) (*EngineImage, error) {
	resp := &EngineImage{}
	err := c.rancherClient.doCreate(ENGINE_IMAGE_TYPE, container, resp)
	return resp, err
}

func (c *EngineImageClient) Update(existing *EngineImage, updates interface{}) (*EngineImage, error) {
	resp := &EngineImage{}
	err := c.rancherClient.doUpdate(ENGINE_IMAGE_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *EngineImageClient) List(opts *ListOpts) (*EngineImageCollection, error) {
	resp := &EngineImageCollection{}
	err := c.rancherClient.doList(ENGINE_IMAGE_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *EngineImageCollection) Next() (*EngineImageCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &EngineImageCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *EngineImageClient) ById(id string) (*EngineImage, error) {
	resp := &EngineImage{}
	err := c.rancherClient.doById(ENGINE_IMAGE_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *EngineImageClient) Delete(container *EngineImage) error {
	return c.rancherClient.doResourceDelete(ENGINE_IMAGE_TYPE, &container.Resource)
}
