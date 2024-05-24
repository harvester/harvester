package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/internal/utils"
	"github.com/manicminer/hamilton/odata"
)

// DirectoryObjectsClient performs operations on Directory Objects (the base type for other objects such as users and groups)
type DirectoryObjectsClient struct {
	BaseClient Client
}

// NewDirectoryObjectsClient returns a new DirectoryObjectsClient.
func NewDirectoryObjectsClient(tenantId string) *DirectoryObjectsClient {
	return &DirectoryObjectsClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// Get retrieves a DirectoryObject.
func (c *DirectoryObjectsClient) Get(ctx context.Context, id string, query odata.Query) (*DirectoryObject, int, error) {
	query.Metadata = odata.MetadataFull

	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directoryObjects/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryObjects.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var directoryObject DirectoryObject
	if err := json.Unmarshal(respBody, &directoryObject); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &directoryObject, status, nil
}

// GetByIds retrieves multiple DirectoryObjects from a list of IDs.
func (c *DirectoryObjectsClient) GetByIds(ctx context.Context, ids []string, types []odata.ShortType) (*[]DirectoryObject, int, error) {
	var status int

	body, err := json.Marshal(struct {
		IDs   []string     `json:"ids"`
		Types []odata.Type `json:"types"`
	}{
		IDs:   ids,
		Types: types,
	})
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/directoryObjects/getByIds",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryObjects.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Objects []DirectoryObject `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.Objects, status, nil
}

// Delete removes a DirectoryObject.
func (c *DirectoryObjectsClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directoryObjects/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("DirectoryObjects.BaseClient.Get(): %v", err)
	}

	return status, nil
}

// GetMemberGroups retrieves IDs of the groups and directory roles that a directory object is a member of.
// id is the object ID of the directory object.
func (c *DirectoryObjectsClient) GetMemberGroups(ctx context.Context, id string, securityEnabledOnly bool) (*[]DirectoryObject, int, error) {
	var status int

	body, err := json.Marshal(struct {
		SecurityEnabledOnly bool `json:"securityEnabledOnly"`
	}{
		SecurityEnabledOnly: securityEnabledOnly,
	})
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directoryObjects/%s/getMemberGroups", id),
			HasTenantId: false,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryObjectsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		IDs []string `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	result := make([]DirectoryObject, len(data.IDs))
	for i, id := range data.IDs {
		result[i].ID = utils.StringPtr(id)
	}

	return &result, status, nil
}

// GetMemberObjects retrieves IDs of the groups and directory roles that a directory object is a member of.
// id is the object ID of the directory object.
func (c *DirectoryObjectsClient) GetMemberObjects(ctx context.Context, id string, securityEnabledOnly bool) (*[]DirectoryObject, int, error) {
	var status int

	body, err := json.Marshal(struct {
		SecurityEnabledOnly bool `json:"securityEnabledOnly"`
	}{
		SecurityEnabledOnly: securityEnabledOnly,
	})
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directoryObjects/%s/getMemberObjects", id),
			HasTenantId: false,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryObjectsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		IDs []string `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	result := make([]DirectoryObject, len(data.IDs))
	for i, id := range data.IDs {
		result[i].ID = utils.StringPtr(id)
	}

	return &result, status, nil
}
