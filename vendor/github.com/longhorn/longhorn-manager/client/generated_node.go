package client

const (
	NODE_TYPE = "node"
)

type Node struct {
	Resource `yaml:"-"`

	Address string `json:"address,omitempty" yaml:"address,omitempty"`

	AllowScheduling bool `json:"allowScheduling,omitempty" yaml:"allow_scheduling,omitempty"`

	Conditions map[string]interface{} `json:"conditions,omitempty" yaml:"conditions,omitempty"`

	Disks map[string]interface{} `json:"disks,omitempty" yaml:"disks,omitempty"`

	EvictionRequested bool `json:"evictionRequested,omitempty" yaml:"eviction_requested,omitempty"`

	InstanceManagerCPURequest int64 `json:"instanceManagerCPURequest,omitempty" yaml:"instance_manager_cpurequest,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	Region string `json:"region,omitempty" yaml:"region,omitempty"`

	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	Zone string `json:"zone,omitempty" yaml:"zone,omitempty"`
}

type NodeCollection struct {
	Collection
	Data   []Node `json:"data,omitempty"`
	client *NodeClient
}

type NodeClient struct {
	rancherClient *RancherClient
}

type NodeOperations interface {
	List(opts *ListOpts) (*NodeCollection, error)
	Create(opts *Node) (*Node, error)
	Update(existing *Node, updates interface{}) (*Node, error)
	ById(id string) (*Node, error)
	Delete(container *Node) error

	ActionDiskUpdate(*Node, *DiskUpdateInput) (*Node, error)
}

func newNodeClient(rancherClient *RancherClient) *NodeClient {
	return &NodeClient{
		rancherClient: rancherClient,
	}
}

func (c *NodeClient) Create(container *Node) (*Node, error) {
	resp := &Node{}
	err := c.rancherClient.doCreate(NODE_TYPE, container, resp)
	return resp, err
}

func (c *NodeClient) Update(existing *Node, updates interface{}) (*Node, error) {
	resp := &Node{}
	err := c.rancherClient.doUpdate(NODE_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *NodeClient) List(opts *ListOpts) (*NodeCollection, error) {
	resp := &NodeCollection{}
	err := c.rancherClient.doList(NODE_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *NodeCollection) Next() (*NodeCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &NodeCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *NodeClient) ById(id string) (*Node, error) {
	resp := &Node{}
	err := c.rancherClient.doById(NODE_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *NodeClient) Delete(container *Node) error {
	return c.rancherClient.doResourceDelete(NODE_TYPE, &container.Resource)
}

func (c *NodeClient) ActionDiskUpdate(resource *Node, input *DiskUpdateInput) (*Node, error) {

	resp := &Node{}

	err := c.rancherClient.doAction(NODE_TYPE, "diskUpdate", &resource.Resource, input, resp)

	return resp, err
}
