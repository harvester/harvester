package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DirectoryRoleTemplatesClient performs operations on DirectoryRoleTemplates.
type DirectoryRoleTemplatesClient struct {
	BaseClient Client
}

// NewDirectoryRoleTemplatesClient returns a new DirectoryRoleTemplatesClient
func NewDirectoryRoleTemplatesClient(tenantId string) *DirectoryRoleTemplatesClient {
	return &DirectoryRoleTemplatesClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of DirectoryRoleTemplates.
func (c *DirectoryRoleTemplatesClient) List(ctx context.Context) (*[]DirectoryRoleTemplate, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/directoryRoleTemplates",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryRoleTemplatesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		DirectoryRoleTemplates []DirectoryRoleTemplate `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.DirectoryRoleTemplates, status, nil
}

// Get retrieves a DirectoryRoleTemplate manifest.
func (c *DirectoryRoleTemplatesClient) Get(ctx context.Context, id string) (*DirectoryRoleTemplate, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directoryRoleTemplates/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryRoleTemplatesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var dirRoleTemplate DirectoryRoleTemplate
	if err := json.Unmarshal(respBody, &dirRoleTemplate); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &dirRoleTemplate, status, nil
}
