package client

const (
	BACKUP_INPUT_TYPE = "backupInput"
)

type BackupInput struct {
	Resource `yaml:"-"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type BackupInputCollection struct {
	Collection
	Data   []BackupInput `json:"data,omitempty"`
	client *BackupInputClient
}

type BackupInputClient struct {
	rancherClient *RancherClient
}

type BackupInputOperations interface {
	List(opts *ListOpts) (*BackupInputCollection, error)
	Create(opts *BackupInput) (*BackupInput, error)
	Update(existing *BackupInput, updates interface{}) (*BackupInput, error)
	ById(id string) (*BackupInput, error)
	Delete(container *BackupInput) error
}

func newBackupInputClient(rancherClient *RancherClient) *BackupInputClient {
	return &BackupInputClient{
		rancherClient: rancherClient,
	}
}

func (c *BackupInputClient) Create(container *BackupInput) (*BackupInput, error) {
	resp := &BackupInput{}
	err := c.rancherClient.doCreate(BACKUP_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *BackupInputClient) Update(existing *BackupInput, updates interface{}) (*BackupInput, error) {
	resp := &BackupInput{}
	err := c.rancherClient.doUpdate(BACKUP_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *BackupInputClient) List(opts *ListOpts) (*BackupInputCollection, error) {
	resp := &BackupInputCollection{}
	err := c.rancherClient.doList(BACKUP_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *BackupInputCollection) Next() (*BackupInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &BackupInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *BackupInputClient) ById(id string) (*BackupInput, error) {
	resp := &BackupInput{}
	err := c.rancherClient.doById(BACKUP_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *BackupInputClient) Delete(container *BackupInput) error {
	return c.rancherClient.doResourceDelete(BACKUP_INPUT_TYPE, &container.Resource)
}
