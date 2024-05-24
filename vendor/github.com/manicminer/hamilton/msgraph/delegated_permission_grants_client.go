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

// DelegatedPermissionGrantsClient performs operations on DelegatedPermissionGrants.
type DelegatedPermissionGrantsClient struct {
	BaseClient Client
}

// NewDelegatedPermissionGrantsClient returns a new DelegatedPermissionGrantsClient
func NewDelegatedPermissionGrantsClient(tenantId string) *DelegatedPermissionGrantsClient {
	return &DelegatedPermissionGrantsClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of delegated permission grants
func (c *DelegatedPermissionGrantsClient) List(ctx context.Context, query odata.Query) (*[]DelegatedPermissionGrant, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/oauth2PermissionGrants",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DelegatedPermissionGrantsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		DelegatedPermissionGrants []DelegatedPermissionGrant `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.DelegatedPermissionGrants, status, nil
}

// Create creates a new delegated permission grant
func (c *DelegatedPermissionGrantsClient) Create(ctx context.Context, delegatedPermissionGrant DelegatedPermissionGrant) (*DelegatedPermissionGrant, int, error) {
	var status int

	if delegatedPermissionGrant.ClientId == nil {
		return nil, status, errors.New("DelegatedPermissionGrantsClient.Create(): ClientId was nil for delegatedPermissionGrant")
	}

	body, err := json.Marshal(delegatedPermissionGrant)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	consistencyFunc := func(resp *http.Response, o *odata.OData) bool {
		if resp != nil && o != nil && o.Error != nil {
			if resp.StatusCode == http.StatusNotFound {
				return true
			} else if resp.StatusCode == http.StatusBadRequest {
				return o.Error.Match(odata.ErrorResourceDoesNotExist)
			}
		}
		return false
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: consistencyFunc,
		ValidStatusCodes:       []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/oauth2PermissionGrants",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DelegatedPermissionGrantsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newDelegatedPermissionGrant DelegatedPermissionGrant
	if err := json.Unmarshal(respBody, &newDelegatedPermissionGrant); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newDelegatedPermissionGrant, status, nil
}

// Get returns a delegated permission grant
func (c *DelegatedPermissionGrantsClient) Get(ctx context.Context, id string, query odata.Query) (*DelegatedPermissionGrant, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/oauth2PermissionGrants/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DelegatedPermissionGrantsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data DelegatedPermissionGrant
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data, status, nil
}

// Update amends an existing delegated permission grant
func (c *DelegatedPermissionGrantsClient) Update(ctx context.Context, delegatedPermissionGrant DelegatedPermissionGrant) (int, error) {
	var status int

	if delegatedPermissionGrant.Id == nil {
		return status, errors.New("cannot update delegated permission grant with nil ID")
	}

	body, err := json.Marshal(delegatedPermissionGrant)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/oauth2PermissionGrants/%s", *delegatedPermissionGrant.Id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("DelegatedPermissionGrantsClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a delegated permission grant
func (c *DelegatedPermissionGrantsClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/oauth2PermissionGrants/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("DelegatedPermissionGrantsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}
