package client

const (
	SETTING_DEFINITION_TYPE = "settingDefinition"
)

type SettingDefinition struct {
	Resource `yaml:"-"`

	Category string `json:"category,omitempty" yaml:"category,omitempty"`

	Default string `json:"default,omitempty" yaml:"default,omitempty"`

	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	DisplayName string `json:"displayName,omitempty" yaml:"display_name,omitempty"`

	Options []string `json:"options,omitempty" yaml:"options,omitempty"`

	ReadOnly bool `json:"readOnly,omitempty" yaml:"read_only,omitempty"`

	Required bool `json:"required,omitempty" yaml:"required,omitempty"`
}

type SettingDefinitionCollection struct {
	Collection
	Data   []SettingDefinition `json:"data,omitempty"`
	client *SettingDefinitionClient
}

type SettingDefinitionClient struct {
	rancherClient *RancherClient
}

type SettingDefinitionOperations interface {
	List(opts *ListOpts) (*SettingDefinitionCollection, error)
	Create(opts *SettingDefinition) (*SettingDefinition, error)
	Update(existing *SettingDefinition, updates interface{}) (*SettingDefinition, error)
	ById(id string) (*SettingDefinition, error)
	Delete(container *SettingDefinition) error
}

func newSettingDefinitionClient(rancherClient *RancherClient) *SettingDefinitionClient {
	return &SettingDefinitionClient{
		rancherClient: rancherClient,
	}
}

func (c *SettingDefinitionClient) Create(container *SettingDefinition) (*SettingDefinition, error) {
	resp := &SettingDefinition{}
	err := c.rancherClient.doCreate(SETTING_DEFINITION_TYPE, container, resp)
	return resp, err
}

func (c *SettingDefinitionClient) Update(existing *SettingDefinition, updates interface{}) (*SettingDefinition, error) {
	resp := &SettingDefinition{}
	err := c.rancherClient.doUpdate(SETTING_DEFINITION_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SettingDefinitionClient) List(opts *ListOpts) (*SettingDefinitionCollection, error) {
	resp := &SettingDefinitionCollection{}
	err := c.rancherClient.doList(SETTING_DEFINITION_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SettingDefinitionCollection) Next() (*SettingDefinitionCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SettingDefinitionCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SettingDefinitionClient) ById(id string) (*SettingDefinition, error) {
	resp := &SettingDefinition{}
	err := c.rancherClient.doById(SETTING_DEFINITION_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SettingDefinitionClient) Delete(container *SettingDefinition) error {
	return c.rancherClient.doResourceDelete(SETTING_DEFINITION_TYPE, &container.Resource)
}
