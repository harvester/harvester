package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// SchemaExtensionsClient performs operations on Schema Extensions.
type SchemaExtensionsClient struct {
	BaseClient Client
}

// NewSchemaExtensionsClient returns a new SchemaExtensionsClient.
func NewSchemaExtensionsClient(tenantId string) *SchemaExtensionsClient {
	return &SchemaExtensionsClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of Schema Extensions, optionally filtered using OData.
func (c *SchemaExtensionsClient) List(ctx context.Context, query odata.Query) (*[]SchemaExtension, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/schemaExtensions",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("SchemaExtensionsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		SchemaExtensions []SchemaExtension `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.SchemaExtensions, status, nil
}

// Get retrieves a Schema Extension.
func (c *SchemaExtensionsClient) Get(ctx context.Context, id string, query odata.Query) (*SchemaExtension, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/schemaExtensions/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("SchemaExtensionsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var schemaExtension SchemaExtension
	if err := json.Unmarshal(respBody, &schemaExtension); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &schemaExtension, status, nil
}

// Update amends an existing schema Extension.
func (c *SchemaExtensionsClient) Update(ctx context.Context, schemaExtension SchemaExtension) (int, error) {
	var status int

	body, err := json.Marshal(schemaExtension)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/schemaExtensions/%s", *schemaExtension.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("SchemaExtensionsClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Create creates a new Schema Extension
func (c *SchemaExtensionsClient) Create(ctx context.Context, schemaExtension SchemaExtension) (*SchemaExtension, int, error) {
	var status int

	body, err := json.Marshal(schemaExtension)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/schemaExtensions",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("SchemaExtensionsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newSchemaExtension SchemaExtension
	if err := json.Unmarshal(respBody, &newSchemaExtension); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newSchemaExtension, status, nil
}

// Delete removes a schema extension.
func (c *SchemaExtensionsClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/schemaExtensions/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("SchemaExtensionsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}
