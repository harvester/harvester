package client

const (
	PVCCREATE_INPUT_TYPE = "PVCCreateInput"
)

type PVCCreateInput struct {
	Resource `yaml:"-"`

	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	PvcName string `json:"pvcName,omitempty" yaml:"pvc_name,omitempty"`
}

type PVCCreateInputCollection struct {
	Collection
	Data   []PVCCreateInput `json:"data,omitempty"`
	client *PVCCreateInputClient
}

type PVCCreateInputClient struct {
	rancherClient *RancherClient
}

type PVCCreateInputOperations interface {
	List(opts *ListOpts) (*PVCCreateInputCollection, error)
	Create(opts *PVCCreateInput) (*PVCCreateInput, error)
	Update(existing *PVCCreateInput, updates interface{}) (*PVCCreateInput, error)
	ById(id string) (*PVCCreateInput, error)
	Delete(container *PVCCreateInput) error
}

func newPVCCreateInputClient(rancherClient *RancherClient) *PVCCreateInputClient {
	return &PVCCreateInputClient{
		rancherClient: rancherClient,
	}
}

func (c *PVCCreateInputClient) Create(container *PVCCreateInput) (*PVCCreateInput, error) {
	resp := &PVCCreateInput{}
	err := c.rancherClient.doCreate(PVCCREATE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *PVCCreateInputClient) Update(existing *PVCCreateInput, updates interface{}) (*PVCCreateInput, error) {
	resp := &PVCCreateInput{}
	err := c.rancherClient.doUpdate(PVCCREATE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *PVCCreateInputClient) List(opts *ListOpts) (*PVCCreateInputCollection, error) {
	resp := &PVCCreateInputCollection{}
	err := c.rancherClient.doList(PVCCREATE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *PVCCreateInputCollection) Next() (*PVCCreateInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &PVCCreateInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *PVCCreateInputClient) ById(id string) (*PVCCreateInput, error) {
	resp := &PVCCreateInput{}
	err := c.rancherClient.doById(PVCCREATE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *PVCCreateInputClient) Delete(container *PVCCreateInput) error {
	return c.rancherClient.doResourceDelete(PVCCREATE_INPUT_TYPE, &container.Resource)
}
