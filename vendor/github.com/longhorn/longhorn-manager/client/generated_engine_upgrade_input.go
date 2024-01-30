package client

const (
	ENGINE_UPGRADE_INPUT_TYPE = "engineUpgradeInput"
)

type EngineUpgradeInput struct {
	Resource `yaml:"-"`

	Image string `json:"image,omitempty" yaml:"image,omitempty"`
}

type EngineUpgradeInputCollection struct {
	Collection
	Data   []EngineUpgradeInput `json:"data,omitempty"`
	client *EngineUpgradeInputClient
}

type EngineUpgradeInputClient struct {
	rancherClient *RancherClient
}

type EngineUpgradeInputOperations interface {
	List(opts *ListOpts) (*EngineUpgradeInputCollection, error)
	Create(opts *EngineUpgradeInput) (*EngineUpgradeInput, error)
	Update(existing *EngineUpgradeInput, updates interface{}) (*EngineUpgradeInput, error)
	ById(id string) (*EngineUpgradeInput, error)
	Delete(container *EngineUpgradeInput) error
}

func newEngineUpgradeInputClient(rancherClient *RancherClient) *EngineUpgradeInputClient {
	return &EngineUpgradeInputClient{
		rancherClient: rancherClient,
	}
}

func (c *EngineUpgradeInputClient) Create(container *EngineUpgradeInput) (*EngineUpgradeInput, error) {
	resp := &EngineUpgradeInput{}
	err := c.rancherClient.doCreate(ENGINE_UPGRADE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *EngineUpgradeInputClient) Update(existing *EngineUpgradeInput, updates interface{}) (*EngineUpgradeInput, error) {
	resp := &EngineUpgradeInput{}
	err := c.rancherClient.doUpdate(ENGINE_UPGRADE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *EngineUpgradeInputClient) List(opts *ListOpts) (*EngineUpgradeInputCollection, error) {
	resp := &EngineUpgradeInputCollection{}
	err := c.rancherClient.doList(ENGINE_UPGRADE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *EngineUpgradeInputCollection) Next() (*EngineUpgradeInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &EngineUpgradeInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *EngineUpgradeInputClient) ById(id string) (*EngineUpgradeInput, error) {
	resp := &EngineUpgradeInput{}
	err := c.rancherClient.doById(ENGINE_UPGRADE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *EngineUpgradeInputClient) Delete(container *EngineUpgradeInput) error {
	return c.rancherClient.doResourceDelete(ENGINE_UPGRADE_INPUT_TYPE, &container.Resource)
}
