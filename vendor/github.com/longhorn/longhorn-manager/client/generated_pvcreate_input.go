package client

const (
	PVCREATE_INPUT_TYPE = "PVCreateInput"
)

type PVCreateInput struct {
	Resource `yaml:"-"`

	FsType string `json:"fsType,omitempty" yaml:"fs_type,omitempty"`

	PvName string `json:"pvName,omitempty" yaml:"pv_name,omitempty"`

	SecretName string `json:"secretName,omitempty" yaml:"secret_name,omitempty"`

	SecretNamespace string `json:"secretNamespace,omitempty" yaml:"secret_namespace,omitempty"`
}

type PVCreateInputCollection struct {
	Collection
	Data   []PVCreateInput `json:"data,omitempty"`
	client *PVCreateInputClient
}

type PVCreateInputClient struct {
	rancherClient *RancherClient
}

type PVCreateInputOperations interface {
	List(opts *ListOpts) (*PVCreateInputCollection, error)
	Create(opts *PVCreateInput) (*PVCreateInput, error)
	Update(existing *PVCreateInput, updates interface{}) (*PVCreateInput, error)
	ById(id string) (*PVCreateInput, error)
	Delete(container *PVCreateInput) error
}

func newPVCreateInputClient(rancherClient *RancherClient) *PVCreateInputClient {
	return &PVCreateInputClient{
		rancherClient: rancherClient,
	}
}

func (c *PVCreateInputClient) Create(container *PVCreateInput) (*PVCreateInput, error) {
	resp := &PVCreateInput{}
	err := c.rancherClient.doCreate(PVCREATE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *PVCreateInputClient) Update(existing *PVCreateInput, updates interface{}) (*PVCreateInput, error) {
	resp := &PVCreateInput{}
	err := c.rancherClient.doUpdate(PVCREATE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *PVCreateInputClient) List(opts *ListOpts) (*PVCreateInputCollection, error) {
	resp := &PVCreateInputCollection{}
	err := c.rancherClient.doList(PVCREATE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *PVCreateInputCollection) Next() (*PVCreateInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &PVCreateInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *PVCreateInputClient) ById(id string) (*PVCreateInput, error) {
	resp := &PVCreateInput{}
	err := c.rancherClient.doById(PVCREATE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *PVCreateInputClient) Delete(container *PVCreateInput) error {
	return c.rancherClient.doResourceDelete(PVCREATE_INPUT_TYPE, &container.Resource)
}
