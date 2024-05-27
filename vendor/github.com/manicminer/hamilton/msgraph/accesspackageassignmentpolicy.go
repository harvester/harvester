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

type AccessPackageAssignmentPolicyClient struct {
	BaseClient Client
}

func NewAccessPackageAssignmentPolicyClient(tenantId string) *AccessPackageAssignmentPolicyClient {
	return &AccessPackageAssignmentPolicyClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of AccessPackageAssignmentPolicy
func (c *AccessPackageAssignmentPolicyClient) List(ctx context.Context, query odata.Query) (*[]AccessPackageAssignmentPolicy, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/identityGovernance/entitlementManagement/accessPackageAssignmentPolicies",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageAssignmentPolicyClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AccessPackageAssignmentPolicies []AccessPackageAssignmentPolicy `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.AccessPackageAssignmentPolicies, status, nil
}

// Create creates a new AccessPackageAssignmentPolicy.
func (c *AccessPackageAssignmentPolicyClient) Create(ctx context.Context, accessPackageAssignmentPolicy AccessPackageAssignmentPolicy) (*AccessPackageAssignmentPolicy, int, error) {
	var status int
	body, err := json.Marshal(accessPackageAssignmentPolicy)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/identityGovernance/entitlementManagement/accessPackageAssignmentPolicies",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageAssignmentPolicyClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newAccessPackageAssignmentPolicy AccessPackageAssignmentPolicy
	if err := json.Unmarshal(respBody, &newAccessPackageAssignmentPolicy); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newAccessPackageAssignmentPolicy, status, nil
}

// Get retrieves a AccessPackageAssignmentPolicy.
func (c *AccessPackageAssignmentPolicyClient) Get(ctx context.Context, id string, query odata.Query) (*AccessPackageAssignmentPolicy, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackageAssignmentPolicies/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageAssignmentPolicyClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var accessPackageAssignmentPolicy AccessPackageAssignmentPolicy
	if err := json.Unmarshal(respBody, &accessPackageAssignmentPolicy); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &accessPackageAssignmentPolicy, status, nil
}

// Update amends an existing AccessPackageAssignmentPolicy.
func (c *AccessPackageAssignmentPolicyClient) Update(ctx context.Context, accessPackageAssignmentPolicy AccessPackageAssignmentPolicy) (int, error) {
	var status int

	if accessPackageAssignmentPolicy.ID == nil {
		return status, errors.New("cannot update AccessPackageAssignmentPolicy with nil ID")
	}

	body, err := json.Marshal(accessPackageAssignmentPolicy)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Put(ctx, PutHttpRequestInput{ //This is usually a patch but this endpoint uses PUT
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackageAssignmentPolicies/%s", *accessPackageAssignmentPolicy.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AccessPackageAssignmentPolicyClient.BaseClient.Put(): %v", err)
	}

	return status, nil
}

// Delete removes a AccessPackageAssignmentPolicy.
func (c *AccessPackageAssignmentPolicyClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackageAssignmentPolicies/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AccessPackageAssignmentPolicyClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}
