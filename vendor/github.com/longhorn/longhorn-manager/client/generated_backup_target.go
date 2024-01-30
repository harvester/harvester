package client

const (
	BACKUP_TARGET_TYPE = "backupTarget"
)

type BackupTarget struct {
	Resource `yaml:"-"`

	Available bool `json:"available,omitempty" yaml:"available,omitempty"`

	BackupTargetURL string `json:"backupTargetURL,omitempty" yaml:"backup_target_url,omitempty"`

	CredentialSecret string `json:"credentialSecret,omitempty" yaml:"credential_secret,omitempty"`

	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	PollInterval string `json:"pollInterval,omitempty" yaml:"poll_interval,omitempty"`
}

type BackupTargetCollection struct {
	Collection
	Data   []BackupTarget `json:"data,omitempty"`
	client *BackupTargetClient
}

type BackupTargetClient struct {
	rancherClient *RancherClient
}

type BackupTargetOperations interface {
	List(opts *ListOpts) (*BackupTargetCollection, error)
	Create(opts *BackupTarget) (*BackupTarget, error)
	Update(existing *BackupTarget, updates interface{}) (*BackupTarget, error)
	ById(id string) (*BackupTarget, error)
	Delete(container *BackupTarget) error
}

func newBackupTargetClient(rancherClient *RancherClient) *BackupTargetClient {
	return &BackupTargetClient{
		rancherClient: rancherClient,
	}
}

func (c *BackupTargetClient) Create(container *BackupTarget) (*BackupTarget, error) {
	resp := &BackupTarget{}
	err := c.rancherClient.doCreate(BACKUP_TARGET_TYPE, container, resp)
	return resp, err
}

func (c *BackupTargetClient) Update(existing *BackupTarget, updates interface{}) (*BackupTarget, error) {
	resp := &BackupTarget{}
	err := c.rancherClient.doUpdate(BACKUP_TARGET_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *BackupTargetClient) List(opts *ListOpts) (*BackupTargetCollection, error) {
	resp := &BackupTargetCollection{}
	err := c.rancherClient.doList(BACKUP_TARGET_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *BackupTargetCollection) Next() (*BackupTargetCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &BackupTargetCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *BackupTargetClient) ById(id string) (*BackupTarget, error) {
	resp := &BackupTarget{}
	err := c.rancherClient.doById(BACKUP_TARGET_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *BackupTargetClient) Delete(container *BackupTarget) error {
	return c.rancherClient.doResourceDelete(BACKUP_TARGET_TYPE, &container.Resource)
}
