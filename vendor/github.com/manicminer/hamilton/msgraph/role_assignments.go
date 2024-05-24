package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// RoleAssignmentsClient performs operations on RoleAssignments.
type RoleAssignmentsClient struct {
	BaseClient Client
}

// NewRoleAssignmentsClient returns a new RoleAssignmentsClient
func NewRoleAssignmentsClient(tenantId string) *RoleAssignmentsClient {
	return &RoleAssignmentsClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of RoleAssignments
func (c *RoleAssignmentsClient) List(ctx context.Context, query odata.Query) (*[]UnifiedRoleAssignment, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/roleManagement/directory/roleAssignments",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("RoleAssignmentsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		RoleAssignments []UnifiedRoleAssignment `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.RoleAssignments, status, nil
}

// Get retrieves a UnifiedRoleAssignment
func (c *RoleAssignmentsClient) Get(ctx context.Context, id string, query odata.Query) (*UnifiedRoleAssignment, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/roleManagement/directory/roleAssignments/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("RoleAssignmentsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var dirRole UnifiedRoleAssignment
	if err := json.Unmarshal(respBody, &dirRole); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &dirRole, status, nil
}

// Create creates a new UnifiedRoleAssignment.
func (c *RoleAssignmentsClient) Create(ctx context.Context, roleAssignment UnifiedRoleAssignment) (*UnifiedRoleAssignment, int, error) {
	var status int

	body, err := json.Marshal(roleAssignment)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/roleManagement/directory/roleAssignments",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("RoleAssignmentsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newRoleAssignment UnifiedRoleAssignment
	if err := json.Unmarshal(respBody, &newRoleAssignment); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newRoleAssignment, status, nil
}

// Delete removes a UnifiedRoleAssignment.
func (c *RoleAssignmentsClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/roleManagement/directory/roleAssignments/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("RoleAssignments.BaseClient.Get(): %v", err)
	}

	return status, nil
}
