package client

const (
	BACKUP_LIST_OUTPUT_TYPE = "backupListOutput"
)

type BackupListOutput struct {
	Resource `yaml:"-"`

	Data []Backup `json:"data,omitempty" yaml:"data,omitempty"`
}

type BackupListOutputCollection struct {
	Collection
	Data   []BackupListOutput `json:"data,omitempty"`
	client *BackupListOutputClient
}

type BackupListOutputClient struct {
	rancherClient *RancherClient
}

type BackupListOutputOperations interface {
	List(opts *ListOpts) (*BackupListOutputCollection, error)
	Create(opts *BackupListOutput) (*BackupListOutput, error)
	Update(existing *BackupListOutput, updates interface{}) (*BackupListOutput, error)
	ById(id string) (*BackupListOutput, error)
	Delete(container *BackupListOutput) error
}

func newBackupListOutputClient(rancherClient *RancherClient) *BackupListOutputClient {
	return &BackupListOutputClient{
		rancherClient: rancherClient,
	}
}

func (c *BackupListOutputClient) Create(container *BackupListOutput) (*BackupListOutput, error) {
	resp := &BackupListOutput{}
	err := c.rancherClient.doCreate(BACKUP_LIST_OUTPUT_TYPE, container, resp)
	return resp, err
}

func (c *BackupListOutputClient) Update(existing *BackupListOutput, updates interface{}) (*BackupListOutput, error) {
	resp := &BackupListOutput{}
	err := c.rancherClient.doUpdate(BACKUP_LIST_OUTPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *BackupListOutputClient) List(opts *ListOpts) (*BackupListOutputCollection, error) {
	resp := &BackupListOutputCollection{}
	err := c.rancherClient.doList(BACKUP_LIST_OUTPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *BackupListOutputCollection) Next() (*BackupListOutputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &BackupListOutputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *BackupListOutputClient) ById(id string) (*BackupListOutput, error) {
	resp := &BackupListOutput{}
	err := c.rancherClient.doById(BACKUP_LIST_OUTPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *BackupListOutputClient) Delete(container *BackupListOutput) error {
	return c.rancherClient.doResourceDelete(BACKUP_LIST_OUTPUT_TYPE, &container.Resource)
}
