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

type AccessPackageCatalogClient struct {
	BaseClient Client
}

func NewAccessPackageCatalogClient(tenantId string) *AccessPackageCatalogClient {
	return &AccessPackageCatalogClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of AccessPackageCatalog.
func (c *AccessPackageCatalogClient) List(ctx context.Context, query odata.Query) (*[]AccessPackageCatalog, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/identityGovernance/entitlementManagement/catalogs",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageCatalogClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AccessPackageCatalogs []AccessPackageCatalog `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.AccessPackageCatalogs, status, nil
}

// Create creates a new AccessPackageCatalog.
func (c *AccessPackageCatalogClient) Create(ctx context.Context, accessPackageCatalog AccessPackageCatalog) (*AccessPackageCatalog, int, error) {
	var status int
	body, err := json.Marshal(accessPackageCatalog)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/identityGovernance/entitlementManagement/catalogs",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageCatalogClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newAccessPackageCatalog AccessPackageCatalog
	if err := json.Unmarshal(respBody, &newAccessPackageCatalog); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newAccessPackageCatalog, status, nil
}

// Get retrieves a AccessPackageCatalog.
func (c *AccessPackageCatalogClient) Get(ctx context.Context, id string, query odata.Query) (*AccessPackageCatalog, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/catalogs/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageCatalogClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var accessPackageCatalog AccessPackageCatalog
	if err := json.Unmarshal(respBody, &accessPackageCatalog); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &accessPackageCatalog, status, nil
}

// Update amends an existing AccessPackageCatalog.
func (c *AccessPackageCatalogClient) Update(ctx context.Context, accessPackageCatalog AccessPackageCatalog) (int, error) {
	var status int

	if accessPackageCatalog.ID == nil {
		return status, errors.New("cannot update accessPackageCatalog with nil ID")
	}

	body, err := json.Marshal(accessPackageCatalog)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/catalogs/%s", *accessPackageCatalog.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AccessPackageCatalogClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a AccessPackageCatalog.
func (c *AccessPackageCatalogClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identityGovernance/entitlementManagement/catalogs/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AccessPackageCatalogClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}
