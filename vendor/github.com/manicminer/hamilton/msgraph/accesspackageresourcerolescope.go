package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/manicminer/hamilton/internal/utils"
	"github.com/manicminer/hamilton/odata"
)

type AccessPackageResourceRoleScopeClient struct {
	BaseClient Client
}

func NewAccessPackageResourceRoleScopeClient(tenantId string) *AccessPackageResourceRoleScopeClient {
	return &AccessPackageResourceRoleScopeClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of AccessPackageResourceRoleScope(s)
func (c *AccessPackageResourceRoleScopeClient) List(ctx context.Context, query odata.Query, accessPackageId string) (*[]AccessPackageResourceRoleScope, int, error) {
	query.Expand = odata.Expand{
		Relationship: "accessPackageResourceRoleScopes",
		Select:       []string{"accessPackageResourceRole", "accessPackageResourceScope"},
	}

	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackages/%s", accessPackageId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageResourceRoleScopeClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AccessPackageResourceRoleScopes []AccessPackageResourceRoleScope `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.AccessPackageResourceRoleScopes, status, nil
}

// Create creates a new AccessPackageResourceRoleScope.
func (c *AccessPackageResourceRoleScopeClient) Create(ctx context.Context, accessPackageResourceRoleScope AccessPackageResourceRoleScope) (*AccessPackageResourceRoleScope, int, error) {
	var status int

	if accessPackageResourceRoleScope.AccessPackageId == nil {
		return nil, status, errors.New("cannot create AccessPackageResourceRoleScope with nil AccessPackageId")
	}

	body, err := json.Marshal(accessPackageResourceRoleScope)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackages/%s/accessPackageResourceRoleScopes", *accessPackageResourceRoleScope.AccessPackageId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageResourceRoleScopeClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newAccessPackageResourceRoleScope AccessPackageResourceRoleScope
	if err := json.Unmarshal(respBody, &newAccessPackageResourceRoleScope); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	accessPackageResourceRoleScope.ID = newAccessPackageResourceRoleScope.ID // Only the ID is returned
	if accessPackageResourceRoleScope.ID == nil {
		return &accessPackageResourceRoleScope, status, fmt.Errorf("accessPackageResourceRoleScope returned with nil ID")
	}

	// We can derive the IDs of the AccessPackageResourceRole & AccessPackageResourceScope from the combined ID
	if ids := strings.Split(*accessPackageResourceRoleScope.ID, "_"); len(ids) == 2 {
		accessPackageResourceRoleScope.AccessPackageResourceRole.ID = utils.StringPtr(ids[0])
		accessPackageResourceRoleScope.AccessPackageResourceScope.ID = utils.StringPtr(ids[1])
	}

	return &accessPackageResourceRoleScope, status, nil
}

// Get retrieves a AccessPackageResourceRoleScope.
func (c *AccessPackageResourceRoleScopeClient) Get(ctx context.Context, accessPackageId string, id string) (*AccessPackageResourceRoleScope, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData: odata.Query{
			Expand: odata.Expand{
				Relationship: "accessPackageResourceRoleScopes",
				Select:       []string{"accessPackageResourceRole", "accessPackageResourceScope"},
			},
		}, //The Resource we made a request to add
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackages/%s", accessPackageId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageResourceRoleScopeClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AccessPackageResourceRoleScopes []AccessPackageResourceRoleScope `json:"accessPackageResourceRoleScopes"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	var accessPackageResourceRoleScope AccessPackageResourceRoleScope

	// There is only a select and expand method on this endpoint, we iterate the result to find the RoleScope
	for _, roleScope := range data.AccessPackageResourceRoleScopes {
		if roleScope.ID != nil && *roleScope.ID == id {
			accessPackageResourceRoleScope = roleScope
			accessPackageResourceRoleScope.AccessPackageId = &accessPackageId
		}
	}

	if accessPackageResourceRoleScope.ID == nil {
		return nil, status, fmt.Errorf("AccessPackageResourceRoleScopeClient.BaseClient.Get(): Could not find accessPackageResourceRoleScope ID")
	}

	return &accessPackageResourceRoleScope, status, nil
}
