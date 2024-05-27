package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

type ClaimsMappingPolicyClient struct {
	BaseClient Client
}

// NewClaimsMappingPolicyClient returns a new ClaimsMappingPolicyClient
func NewClaimsMappingPolicyClient(tenantId string) *ClaimsMappingPolicyClient {
	return &ClaimsMappingPolicyClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// Create creates a new ClaimsMappingPolicy.
func (c *ClaimsMappingPolicyClient) Create(ctx context.Context, policy ClaimsMappingPolicy) (*ClaimsMappingPolicy, int, error) {
	var status int

	body, err := json.Marshal(policy)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		OData:            odata.Query{Metadata: odata.MetadataFull},
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/policies/claimsMappingPolicies",
			HasTenantId: false,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ClaimsMappingPolicyClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newPolicy ClaimsMappingPolicy
	if err := json.Unmarshal(respBody, &newPolicy); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newPolicy, status, nil
}

// List returns a list of ClaimsMappingPolicy, optionally queried using OData.
func (c *ClaimsMappingPolicyClient) List(ctx context.Context, query odata.Query) (*[]ClaimsMappingPolicy, int, error) {
	query.Metadata = odata.MetadataFull

	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/policies/claimsMappingPolicies",
			HasTenantId: false,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ClaimsMappingPolicyClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		ClaimsMappingPolicies []ClaimsMappingPolicy `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.ClaimsMappingPolicies, status, nil
}

// Get retrieves a ClaimsMappingPolicy.
func (c *ClaimsMappingPolicyClient) Get(ctx context.Context, id string, query odata.Query) (*ClaimsMappingPolicy, int, error) {
	query.Metadata = odata.MetadataFull

	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/policies/claimsMappingPolicies/%s", id),
			HasTenantId: false,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ClaimsMappingPolicyClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var claimsMappingPolicies ClaimsMappingPolicy
	if err := json.Unmarshal(respBody, &claimsMappingPolicies); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &claimsMappingPolicies, status, nil
}

// Update amends an existing ClaimsMappingPolicy.
func (c *ClaimsMappingPolicyClient) Update(ctx context.Context, claimsMappingPolicy ClaimsMappingPolicy) (int, error) {
	var status int

	if claimsMappingPolicy.ID == nil {
		return status, fmt.Errorf("cannot update ClaimsMappingPolicy with nil ID")
	}

	claimsMappingPolicyId := *claimsMappingPolicy.ID
	claimsMappingPolicy.ID = nil

	body, err := json.Marshal(claimsMappingPolicy)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes: []int{
			http.StatusOK,
			http.StatusNoContent,
		},
		Uri: Uri{
			Entity:      fmt.Sprintf("/policies/claimsMappingPolicies/%s", claimsMappingPolicyId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ClaimsMappingPolicy.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a ClaimsMappingPolicy.
func (c *ClaimsMappingPolicyClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/policies/claimsMappingPolicies/%s", id),
			HasTenantId: false,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ClaimsMappingPolicyClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}
