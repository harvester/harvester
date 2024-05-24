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

type AccessPackageClient struct {
	BaseClient Client
}

func NewAccessPackageClient(tenantId string) *AccessPackageClient {
	return &AccessPackageClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of AccessPackage
func (c *AccessPackageClient) List(ctx context.Context, query odata.Query) (*[]AccessPackage, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/identityGovernance/entitlementManagement/accessPackages",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AccessPackages []AccessPackage `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.AccessPackages, status, nil
}

// Create creates a new AccessPackage.
func (c *AccessPackageClient) Create(ctx context.Context, accessPackage AccessPackage) (*AccessPackage, int, error) {
	var status int
	body, err := json.Marshal(accessPackage)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/identityGovernance/entitlementManagement/accessPackages",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newAccessPackage AccessPackage
	if err := json.Unmarshal(respBody, &newAccessPackage); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	if c.BaseClient.ApiVersion == Version10 {
		newAccessPackage.Catalog = &AccessPackageCatalog{
			ID: accessPackage.Catalog.ID,
		} //Stable API doesn't return this
	}

	return &newAccessPackage, status, nil
}

// Get retrieves a AccessPackage.
func (c *AccessPackageClient) Get(ctx context.Context, id string, query odata.Query) (*AccessPackage, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackages/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var accessPackage AccessPackage
	if err := json.Unmarshal(respBody, &accessPackage); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &accessPackage, status, nil
}

// Update amends an existing AccessPackage.
func (c *AccessPackageClient) Update(ctx context.Context, accessPackage AccessPackage) (int, error) {
	var status int

	if accessPackage.ID == nil {
		return status, errors.New("cannot update AccessPackage with nil ID")
	}

	body, err := json.Marshal(accessPackage)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackages/%s", *accessPackage.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AccessPackageClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a AccessPackage.
func (c *AccessPackageClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackages/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AccessPackageClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}
