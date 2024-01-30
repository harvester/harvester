package client

const (
	TAG_TYPE = "tag"
)

type Tag struct {
	Resource `yaml:"-"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	TagType string `json:"tagType,omitempty" yaml:"tag_type,omitempty"`
}

type TagCollection struct {
	Collection
	Data   []Tag `json:"data,omitempty"`
	client *TagClient
}

type TagClient struct {
	rancherClient *RancherClient
}

type TagOperations interface {
	List(opts *ListOpts) (*TagCollection, error)
	Create(opts *Tag) (*Tag, error)
	Update(existing *Tag, updates interface{}) (*Tag, error)
	ById(id string) (*Tag, error)
	Delete(container *Tag) error
}

func newTagClient(rancherClient *RancherClient) *TagClient {
	return &TagClient{
		rancherClient: rancherClient,
	}
}

func (c *TagClient) Create(container *Tag) (*Tag, error) {
	resp := &Tag{}
	err := c.rancherClient.doCreate(TAG_TYPE, container, resp)
	return resp, err
}

func (c *TagClient) Update(existing *Tag, updates interface{}) (*Tag, error) {
	resp := &Tag{}
	err := c.rancherClient.doUpdate(TAG_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *TagClient) List(opts *ListOpts) (*TagCollection, error) {
	resp := &TagCollection{}
	err := c.rancherClient.doList(TAG_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *TagCollection) Next() (*TagCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &TagCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *TagClient) ById(id string) (*Tag, error) {
	resp := &Tag{}
	err := c.rancherClient.doById(TAG_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *TagClient) Delete(container *Tag) error {
	return c.rancherClient.doResourceDelete(TAG_TYPE, &container.Resource)
}
