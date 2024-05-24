package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// IdentityProvidersClient performs operations on IdentityProviders.
type IdentityProvidersClient struct {
	BaseClient Client
}

// NewIdentityProvidersClient returns a new IdentityProvidersClient
func NewIdentityProvidersClient(tenantId string) *IdentityProvidersClient {
	return &IdentityProvidersClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of IdentityProviders.
func (c *IdentityProvidersClient) List(ctx context.Context) (*[]IdentityProvider, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/identity/identityProviders",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("IdentityProvidersClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		IdentityProviders []IdentityProvider `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.IdentityProviders, status, nil
}

// Create creates a new IdentityProvider.
func (c *IdentityProvidersClient) Create(ctx context.Context, provider IdentityProvider) (*IdentityProvider, int, error) {
	var status int

	body, err := json.Marshal(provider)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/identity/identityProviders",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("IdentityProvidersClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newProvider IdentityProvider
	if err := json.Unmarshal(respBody, &newProvider); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newProvider, status, nil
}

// Get retrieves an IdentityProvider.
func (c *IdentityProvidersClient) Get(ctx context.Context, id string) (*IdentityProvider, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identity/identityProviders/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("IdentityProvidersClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var provider IdentityProvider
	if err := json.Unmarshal(respBody, &provider); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &provider, status, nil
}

// Update amends an existing IdentityProvider.
func (c *IdentityProvidersClient) Update(ctx context.Context, provider IdentityProvider) (int, error) {
	var status int

	if provider.ID == nil {
		return status, errors.New("IdentityProvidersClient.Update(): cannot update identity provider with nil ID")
	}

	body, err := json.Marshal(provider)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identity/identityProviders/%s", *provider.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("IdentityProvidersClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a IdentityProvider.
func (c *IdentityProvidersClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/identity/identityProviders/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("IdentityProvidersClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// List returns a list of all available identity provider types.
func (c *IdentityProvidersClient) ListAvailableProviderTypes(ctx context.Context) (*[]string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/identity/identityProviders/availableProviderTypes",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("IdentityProvidersClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		IdentityProviderTypes []string `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.IdentityProviderTypes, status, nil
}
