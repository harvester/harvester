package client

const (
	UPDATE_UNMAP_MARK_SNAP_CHAIN_REMOVED_INPUT_TYPE = "UpdateUnmapMarkSnapChainRemovedInput"
)

type UpdateUnmapMarkSnapChainRemovedInput struct {
	Resource `yaml:"-"`

	UnmapMarkSnapChainRemoved string `json:"unmapMarkSnapChainRemoved,omitempty" yaml:"unmap_mark_snap_chain_removed,omitempty"`
}

type UpdateUnmapMarkSnapChainRemovedInputCollection struct {
	Collection
	Data   []UpdateUnmapMarkSnapChainRemovedInput `json:"data,omitempty"`
	client *UpdateUnmapMarkSnapChainRemovedInputClient
}

type UpdateUnmapMarkSnapChainRemovedInputClient struct {
	rancherClient *RancherClient
}

type UpdateUnmapMarkSnapChainRemovedInputOperations interface {
	List(opts *ListOpts) (*UpdateUnmapMarkSnapChainRemovedInputCollection, error)
	Create(opts *UpdateUnmapMarkSnapChainRemovedInput) (*UpdateUnmapMarkSnapChainRemovedInput, error)
	Update(existing *UpdateUnmapMarkSnapChainRemovedInput, updates interface{}) (*UpdateUnmapMarkSnapChainRemovedInput, error)
	ById(id string) (*UpdateUnmapMarkSnapChainRemovedInput, error)
	Delete(container *UpdateUnmapMarkSnapChainRemovedInput) error
}

func newUpdateUnmapMarkSnapChainRemovedInputClient(rancherClient *RancherClient) *UpdateUnmapMarkSnapChainRemovedInputClient {
	return &UpdateUnmapMarkSnapChainRemovedInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateUnmapMarkSnapChainRemovedInputClient) Create(container *UpdateUnmapMarkSnapChainRemovedInput) (*UpdateUnmapMarkSnapChainRemovedInput, error) {
	resp := &UpdateUnmapMarkSnapChainRemovedInput{}
	err := c.rancherClient.doCreate(UPDATE_UNMAP_MARK_SNAP_CHAIN_REMOVED_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateUnmapMarkSnapChainRemovedInputClient) Update(existing *UpdateUnmapMarkSnapChainRemovedInput, updates interface{}) (*UpdateUnmapMarkSnapChainRemovedInput, error) {
	resp := &UpdateUnmapMarkSnapChainRemovedInput{}
	err := c.rancherClient.doUpdate(UPDATE_UNMAP_MARK_SNAP_CHAIN_REMOVED_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateUnmapMarkSnapChainRemovedInputClient) List(opts *ListOpts) (*UpdateUnmapMarkSnapChainRemovedInputCollection, error) {
	resp := &UpdateUnmapMarkSnapChainRemovedInputCollection{}
	err := c.rancherClient.doList(UPDATE_UNMAP_MARK_SNAP_CHAIN_REMOVED_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateUnmapMarkSnapChainRemovedInputCollection) Next() (*UpdateUnmapMarkSnapChainRemovedInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateUnmapMarkSnapChainRemovedInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateUnmapMarkSnapChainRemovedInputClient) ById(id string) (*UpdateUnmapMarkSnapChainRemovedInput, error) {
	resp := &UpdateUnmapMarkSnapChainRemovedInput{}
	err := c.rancherClient.doById(UPDATE_UNMAP_MARK_SNAP_CHAIN_REMOVED_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateUnmapMarkSnapChainRemovedInputClient) Delete(container *UpdateUnmapMarkSnapChainRemovedInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_UNMAP_MARK_SNAP_CHAIN_REMOVED_INPUT_TYPE, &container.Resource)
}
