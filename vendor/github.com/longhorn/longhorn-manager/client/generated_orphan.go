package client

const (
	ORPHAN_TYPE = "orphan"
)

type Orphan struct {
	Resource `yaml:"-"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	NodeID string `json:"nodeID,omitempty" yaml:"node_id,omitempty"`

	OrphanType string `json:"orphanType,omitempty" yaml:"orphan_type,omitempty"`

	Parameters map[string]string `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

type OrphanCollection struct {
	Collection
	Data   []Orphan `json:"data,omitempty"`
	client *OrphanClient
}

type OrphanClient struct {
	rancherClient *RancherClient
}

type OrphanOperations interface {
	List(opts *ListOpts) (*OrphanCollection, error)
	Create(opts *Orphan) (*Orphan, error)
	Update(existing *Orphan, updates interface{}) (*Orphan, error)
	ById(id string) (*Orphan, error)
	Delete(container *Orphan) error
}

func newOrphanClient(rancherClient *RancherClient) *OrphanClient {
	return &OrphanClient{
		rancherClient: rancherClient,
	}
}

func (c *OrphanClient) Create(container *Orphan) (*Orphan, error) {
	resp := &Orphan{}
	err := c.rancherClient.doCreate(ORPHAN_TYPE, container, resp)
	return resp, err
}

func (c *OrphanClient) Update(existing *Orphan, updates interface{}) (*Orphan, error) {
	resp := &Orphan{}
	err := c.rancherClient.doUpdate(ORPHAN_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *OrphanClient) List(opts *ListOpts) (*OrphanCollection, error) {
	resp := &OrphanCollection{}
	err := c.rancherClient.doList(ORPHAN_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *OrphanCollection) Next() (*OrphanCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &OrphanCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *OrphanClient) ById(id string) (*Orphan, error) {
	resp := &Orphan{}
	err := c.rancherClient.doById(ORPHAN_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *OrphanClient) Delete(container *Orphan) error {
	return c.rancherClient.doResourceDelete(ORPHAN_TYPE, &container.Resource)
}
