package client

const (
	SALVAGE_INPUT_TYPE = "salvageInput"
)

type SalvageInput struct {
	Resource `yaml:"-"`

	Names []string `json:"names,omitempty" yaml:"names,omitempty"`
}

type SalvageInputCollection struct {
	Collection
	Data   []SalvageInput `json:"data,omitempty"`
	client *SalvageInputClient
}

type SalvageInputClient struct {
	rancherClient *RancherClient
}

type SalvageInputOperations interface {
	List(opts *ListOpts) (*SalvageInputCollection, error)
	Create(opts *SalvageInput) (*SalvageInput, error)
	Update(existing *SalvageInput, updates interface{}) (*SalvageInput, error)
	ById(id string) (*SalvageInput, error)
	Delete(container *SalvageInput) error
}

func newSalvageInputClient(rancherClient *RancherClient) *SalvageInputClient {
	return &SalvageInputClient{
		rancherClient: rancherClient,
	}
}

func (c *SalvageInputClient) Create(container *SalvageInput) (*SalvageInput, error) {
	resp := &SalvageInput{}
	err := c.rancherClient.doCreate(SALVAGE_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *SalvageInputClient) Update(existing *SalvageInput, updates interface{}) (*SalvageInput, error) {
	resp := &SalvageInput{}
	err := c.rancherClient.doUpdate(SALVAGE_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *SalvageInputClient) List(opts *ListOpts) (*SalvageInputCollection, error) {
	resp := &SalvageInputCollection{}
	err := c.rancherClient.doList(SALVAGE_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *SalvageInputCollection) Next() (*SalvageInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &SalvageInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *SalvageInputClient) ById(id string) (*SalvageInput, error) {
	resp := &SalvageInput{}
	err := c.rancherClient.doById(SALVAGE_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *SalvageInputClient) Delete(container *SalvageInput) error {
	return c.rancherClient.doResourceDelete(SALVAGE_INPUT_TYPE, &container.Resource)
}
