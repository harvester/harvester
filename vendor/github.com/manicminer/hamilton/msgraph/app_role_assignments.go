package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

type appRoleAssignmentsResourceType string

const (
	groupsAppRoleAssignmentsResource            appRoleAssignmentsResourceType = "groups"
	usersAppRoleAssignmentsResource             appRoleAssignmentsResourceType = "users"
	servicePrincipalsAppRoleAssignmentsResource appRoleAssignmentsResourceType = "servicePrincipals"
)

// AppRoleAssignmentsClient performs operations on AppRoleAssignments.
type AppRoleAssignmentsClient struct {
	BaseClient   Client
	resourceType appRoleAssignmentsResourceType
}

// NewUsersAppRoleAssignmentsClient returns a new AppRoleAssignmentsClient for users assignments
func NewUsersAppRoleAssignmentsClient(tenantId string) *AppRoleAssignmentsClient {
	return &AppRoleAssignmentsClient{
		BaseClient:   NewClient(Version10, tenantId),
		resourceType: usersAppRoleAssignmentsResource,
	}
}

// NewGroupsAppRoleAssignmentsClient returns a new AppRoleAssignmentsClient for groups assignments
func NewGroupsAppRoleAssignmentsClient(tenantId string) *AppRoleAssignmentsClient {
	return &AppRoleAssignmentsClient{
		BaseClient:   NewClient(Version10, tenantId),
		resourceType: groupsAppRoleAssignmentsResource,
	}
}

// NewServicePrincipalsAppRoleAssignmentsClient returns a new AppRoleAssignmentsClient for service principal assignments
func NewServicePrincipalsAppRoleAssignmentsClient(tenantId string) *AppRoleAssignmentsClient {
	return &AppRoleAssignmentsClient{
		BaseClient:   NewClient(Version10, tenantId),
		resourceType: servicePrincipalsAppRoleAssignmentsResource,
	}
}

// List returns a list of app role assignments.
func (c *AppRoleAssignmentsClient) List(ctx context.Context, id string) (*[]AppRoleAssignment, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/%s/%s/appRoleAssignments", c.resourceType, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AppRoleAssignmentsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AppRoleAssignments []AppRoleAssignment `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.AppRoleAssignments, status, nil
}

// Remove removes a app role assignment.
func (c *AppRoleAssignmentsClient) Remove(ctx context.Context, id, appRoleAssignmentId string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/%s/%s/appRoleAssignments/%s", c.resourceType, id, appRoleAssignmentId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AppRoleAssignmentsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// Assign assigns an app role to a user, group or service principal depending on client resource type.
func (c *AppRoleAssignmentsClient) Assign(ctx context.Context, clientServicePrincipalId, resourceServicePrincipalId, appRoleId string) (*AppRoleAssignment, int, error) {
	var status int

	data := struct {
		PrincipalId string `json:"principalId"`
		ResourceId  string `json:"resourceId"`
		AppRoleId   string `json:"appRoleId"`
	}{
		PrincipalId: clientServicePrincipalId,
		ResourceId:  resourceServicePrincipalId,
		AppRoleId:   appRoleId,
	}

	body, err := json.Marshal(data)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/%s/%s/appRoleAssignments", c.resourceType, clientServicePrincipalId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AppRoleAssignmentsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var appRoleAssignment AppRoleAssignment
	if err := json.Unmarshal(respBody, &appRoleAssignment); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &appRoleAssignment, status, nil
}

// AppRoleAssignedToClient performs operations on AppRoleAssignments.
type AppRoleAssignedToClient struct {
	BaseClient Client
}

// NewAppRoleAssignedToClient returns a new AppRoleAssignedToClient
func NewAppRoleAssignedToClient(tenantId string) *AppRoleAssignedToClient {
	return &AppRoleAssignedToClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of app role assignments granted for a service principal
func (c *AppRoleAssignedToClient) List(ctx context.Context, id string, query odata.Query) (*[]AppRoleAssignment, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/appRoleAssignedTo", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AppRoleAssignedToClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AppRoleAssignments []AppRoleAssignment `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.AppRoleAssignments, status, nil
}

// Remove removes an app role assignment for a service principal
func (c *AppRoleAssignedToClient) Remove(ctx context.Context, resourceId, appRoleAssignmentId string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/appRoleAssignedTo/%s", resourceId, appRoleAssignmentId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AppRoleAssignedToClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// Assign assigns an app role for a service principal to the specified user/group/service principal object
func (c *AppRoleAssignedToClient) Assign(ctx context.Context, appRoleAssignment AppRoleAssignment) (*AppRoleAssignment, int, error) {
	var status int

	if appRoleAssignment.ResourceId == nil {
		return nil, status, errors.New("AppRoleAssignedToClient.Assign(): ResourceId was nil for appRoleAssignment")
	}
	if appRoleAssignment.AppRoleId == nil {
		return nil, status, errors.New("AppRoleAssignedToClient.Assign(): AppRoleId was nil for appRoleAssignment")
	}
	if appRoleAssignment.PrincipalId == nil {
		return nil, status, errors.New("AppRoleAssignedToClient.Assign(): PrincipalId was nil for appRoleAssignment")
	}

	body, err := json.Marshal(appRoleAssignment)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	consistencyFunc := func(resp *http.Response, o *odata.OData) bool {
		if resp != nil && o != nil && o.Error != nil {
			if resp.StatusCode == http.StatusNotFound {
				return true
			} else if resp.StatusCode == http.StatusBadRequest {
				return o.Error.Match(odata.ErrorNotValidReferenceUpdate)
			}
		}
		return false
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: consistencyFunc,
		ValidStatusCodes:       []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/appRoleAssignedTo", *appRoleAssignment.ResourceId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AppRoleAssignedToClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newAppRoleAssignment AppRoleAssignment
	if err := json.Unmarshal(respBody, &newAppRoleAssignment); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newAppRoleAssignment, status, nil
}
