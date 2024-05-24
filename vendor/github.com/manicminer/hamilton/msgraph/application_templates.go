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

// ApplicationTemplatesClient performs operations on ApplicationTemplates.
type ApplicationTemplatesClient struct {
	BaseClient Client
}

// NewApplicationTemplatesClient returns a new ApplicationTemplatesClient
func NewApplicationTemplatesClient(tenantId string) *ApplicationTemplatesClient {
	return &ApplicationTemplatesClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of ApplicationTemplates, optionally queried using OData.
func (c *ApplicationTemplatesClient) List(ctx context.Context, query odata.Query) (*[]ApplicationTemplate, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/applicationTemplates",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationTemplatesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		ApplicationTemplates []ApplicationTemplate `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.ApplicationTemplates, status, nil
}

// Get retrieves an ApplicationTemplate
func (c *ApplicationTemplatesClient) Get(ctx context.Context, id string, query odata.Query) (*ApplicationTemplate, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applicationTemplates/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationTemplatesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var applicationTemplate ApplicationTemplate
	if err := json.Unmarshal(respBody, &applicationTemplate); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &applicationTemplate, status, nil
}

// Instantiate instantiates an ApplicationTemplate, which creates an Application and Service Principal in the tenant.
// The created Application and ServicePrincipal are provided in the response.
func (c *ApplicationTemplatesClient) Instantiate(ctx context.Context, applicationTemplate ApplicationTemplate) (*ApplicationTemplate, int, error) {
	var status int

	if applicationTemplate.ID == nil {
		return nil, status, errors.New("ApplicationTemplatesClient.Instantiate(): cannot instantiate ApplicationTemplate with nil ID")
	}

	body, err := json.Marshal(applicationTemplate)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applicationTemplates/%s/instantiate", *applicationTemplate.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationTemplatesClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newApplicationTemplate ApplicationTemplate
	if err := json.Unmarshal(respBody, &newApplicationTemplate); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newApplicationTemplate, status, nil
}
