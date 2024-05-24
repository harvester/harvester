package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// RoleDefinitionsClient performs operations on RoleDefinitions.
type RoleDefinitionsClient struct {
	BaseClient Client
}

// NewRoleDefinitionsClient returns a new RoleDefinitionsClient
func NewRoleDefinitionsClient(tenantId string) *RoleDefinitionsClient {
	return &RoleDefinitionsClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of RoleDefinitions
func (c *RoleDefinitionsClient) List(ctx context.Context, query odata.Query) (*[]UnifiedRoleDefinition, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/roleManagement/directory/roleDefinitions",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("RoleDefinitionsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		RoleDefinitions []UnifiedRoleDefinition `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.RoleDefinitions, status, nil
}

// Get retrieves a UnifiedRoleDefinition
func (c *RoleDefinitionsClient) Get(ctx context.Context, id string, query odata.Query) (*UnifiedRoleDefinition, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/roleManagement/directory/roleDefinitions/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("RoleDefinitionsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var dirRole UnifiedRoleDefinition
	if err := json.Unmarshal(respBody, &dirRole); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &dirRole, status, nil
}

// Create creates a new UnifiedRoleDefinition.
func (c *RoleDefinitionsClient) Create(ctx context.Context, roleDefinition UnifiedRoleDefinition) (*UnifiedRoleDefinition, int, error) {
	var status int

	body, err := json.Marshal(roleDefinition)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/roleManagement/directory/roleDefinitions",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("RoleDefinitionsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newRoleDefinition UnifiedRoleDefinition
	if err := json.Unmarshal(respBody, &newRoleDefinition); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newRoleDefinition, status, nil
}

// Update amends an existing UnifiedRoleDefinition.
func (c *RoleDefinitionsClient) Update(ctx context.Context, roleDefinition UnifiedRoleDefinition) (int, error) {
	var status int

	body, err := json.Marshal(roleDefinition)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/roleManagement/directory/roleDefinitions/%s", *roleDefinition.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("RoleDefinitionsClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a UnifiedRoleDefinition.
func (c *RoleDefinitionsClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/roleManagement/directory/roleDefinitions/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("RoleDefinitions.BaseClient.Get(): %v", err)
	}

	return status, nil
}
