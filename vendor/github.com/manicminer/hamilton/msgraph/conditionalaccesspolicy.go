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

// ConditionalAccessPoliciesClient performs operations on ConditionalAccessPolicy.
type ConditionalAccessPoliciesClient struct {
	BaseClient Client
}

// NewConditionalAccessPoliciesClient returns a new ConditionalAccessPoliciesClient
func NewConditionalAccessPoliciesClient(tenantId string) *ConditionalAccessPoliciesClient {
	return &ConditionalAccessPoliciesClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of ConditionalAccessPolicy, optionally queried using OData.
func (c *ConditionalAccessPoliciesClient) List(ctx context.Context, query odata.Query) (*[]ConditionalAccessPolicy, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/identity/conditionalAccess/policies",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ConditionalAccessPoliciesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		ConditionalAccessPolicys []ConditionalAccessPolicy `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.ConditionalAccessPolicys, status, nil
}

// Create creates a new ConditionalAccessPolicy.
func (c *ConditionalAccessPoliciesClient) Create(ctx context.Context, conditionalAccessPolicy ConditionalAccessPolicy) (*ConditionalAccessPolicy, int, error) {
	var status int
	body, err := json.Marshal(conditionalAccessPolicy)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/identity/conditionalAccess/policies",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ConditionalAccessPoliciesClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newConditionalAccessPolicy ConditionalAccessPolicy
	if err := json.Unmarshal(respBody, &newConditionalAccessPolicy); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newConditionalAccessPolicy, status, nil
}

// Get retrieves a ConditionalAccessPolicy.
func (c *ConditionalAccessPoliciesClient) Get(ctx context.Context, id string, query odata.Query) (*ConditionalAccessPolicy, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identity/conditionalAccess/policies/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ConditionalAccessPoliciesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var conditionalAccessPolicy ConditionalAccessPolicy
	if err := json.Unmarshal(respBody, &conditionalAccessPolicy); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &conditionalAccessPolicy, status, nil
}

// Update amends an existing ConditionalAccessPolicy.
func (c *ConditionalAccessPoliciesClient) Update(ctx context.Context, conditionalAccessPolicy ConditionalAccessPolicy) (int, error) {
	var status int

	if conditionalAccessPolicy.ID == nil {
		return status, errors.New("cannot update conditionalAccessPolicy with nil ID")
	}

	body, err := json.Marshal(conditionalAccessPolicy)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identity/conditionalAccess/policies/%s", *conditionalAccessPolicy.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ConditionalAccessPoliciesClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a ConditionalAccessPolicy.
func (c *ConditionalAccessPoliciesClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identity/conditionalAccess/policies/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ConditionalAccessPoliciesClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}
