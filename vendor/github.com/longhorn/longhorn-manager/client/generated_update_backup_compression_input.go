package client

const (
	UPDATE_BACKUP_COMPRESSION_INPUT_TYPE = "UpdateBackupCompressionInput"
)

type UpdateBackupCompressionInput struct {
	Resource `yaml:"-"`

	BackupCompressionMethod string `json:"backupCompressionMethod,omitempty" yaml:"backup_compression_method,omitempty"`
}

type UpdateBackupCompressionInputCollection struct {
	Collection
	Data   []UpdateBackupCompressionInput `json:"data,omitempty"`
	client *UpdateBackupCompressionInputClient
}

type UpdateBackupCompressionInputClient struct {
	rancherClient *RancherClient
}

type UpdateBackupCompressionInputOperations interface {
	List(opts *ListOpts) (*UpdateBackupCompressionInputCollection, error)
	Create(opts *UpdateBackupCompressionInput) (*UpdateBackupCompressionInput, error)
	Update(existing *UpdateBackupCompressionInput, updates interface{}) (*UpdateBackupCompressionInput, error)
	ById(id string) (*UpdateBackupCompressionInput, error)
	Delete(container *UpdateBackupCompressionInput) error
}

func newUpdateBackupCompressionInputClient(rancherClient *RancherClient) *UpdateBackupCompressionInputClient {
	return &UpdateBackupCompressionInputClient{
		rancherClient: rancherClient,
	}
}

func (c *UpdateBackupCompressionInputClient) Create(container *UpdateBackupCompressionInput) (*UpdateBackupCompressionInput, error) {
	resp := &UpdateBackupCompressionInput{}
	err := c.rancherClient.doCreate(UPDATE_BACKUP_COMPRESSION_INPUT_TYPE, container, resp)
	return resp, err
}

func (c *UpdateBackupCompressionInputClient) Update(existing *UpdateBackupCompressionInput, updates interface{}) (*UpdateBackupCompressionInput, error) {
	resp := &UpdateBackupCompressionInput{}
	err := c.rancherClient.doUpdate(UPDATE_BACKUP_COMPRESSION_INPUT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *UpdateBackupCompressionInputClient) List(opts *ListOpts) (*UpdateBackupCompressionInputCollection, error) {
	resp := &UpdateBackupCompressionInputCollection{}
	err := c.rancherClient.doList(UPDATE_BACKUP_COMPRESSION_INPUT_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *UpdateBackupCompressionInputCollection) Next() (*UpdateBackupCompressionInputCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &UpdateBackupCompressionInputCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *UpdateBackupCompressionInputClient) ById(id string) (*UpdateBackupCompressionInput, error) {
	resp := &UpdateBackupCompressionInput{}
	err := c.rancherClient.doById(UPDATE_BACKUP_COMPRESSION_INPUT_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *UpdateBackupCompressionInputClient) Delete(container *UpdateBackupCompressionInput) error {
	return c.rancherClient.doResourceDelete(UPDATE_BACKUP_COMPRESSION_INPUT_TYPE, &container.Resource)
}
