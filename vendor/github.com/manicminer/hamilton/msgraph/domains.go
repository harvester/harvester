package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// DomainsClient performs operations on Domains.
type DomainsClient struct {
	BaseClient Client
}

// NewDomainsClient returns a new DomainsClient.
func NewDomainsClient(tenantId string) *DomainsClient {
	return &DomainsClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of Domains.
func (c *DomainsClient) List(ctx context.Context, query odata.Query) (*[]Domain, int, error) {
	var status int
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ValidStatusCodes: []int{http.StatusOK},
		OData:            query,
		Uri: Uri{
			Entity:      "/domains",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DomainsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Domains []Domain `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.Domains, status, nil
}

// Get retrieves a Domain.
func (c *DomainsClient) Get(ctx context.Context, id string, query odata.Query) (*Domain, int, error) {
	var status int

	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/domains/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DomainsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var domain Domain
	if err := json.Unmarshal(respBody, &domain); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &domain, status, nil
}
