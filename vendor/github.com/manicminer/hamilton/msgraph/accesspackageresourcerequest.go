package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/internal/utils"

	"github.com/manicminer/hamilton/odata"
)

type AccessPackageResourceRequestClient struct {
	BaseClient Client
}

func NewAccessPackageResourceRequestClient(tenantId string) *AccessPackageResourceRequestClient {
	return &AccessPackageResourceRequestClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of AccessPackageResourceRequest
func (c *AccessPackageResourceRequestClient) List(ctx context.Context, query odata.Query) (*[]AccessPackageResourceRequest, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/identityGovernance/entitlementManagement/accessPackageResourceRequests",
			Params:      query.Values(),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageResourceRequestClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AccessPackageResourceRequests []AccessPackageResourceRequest `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.AccessPackageResourceRequests, status, nil
}

// Create creates a new AccessPackageResourceRequest.
func (c *AccessPackageResourceRequestClient) Create(ctx context.Context, accessPackageResourceRequest AccessPackageResourceRequest, pollForId bool) (*AccessPackageResourceRequest, int, error) {
	// We are always going to assume a user wants to execute this immediately as having a wait on this makes no sense programmatically
	accessPackageResourceRequest.ExecuteImmediately = utils.BoolPtr(true)

	var status int
	body, err := json.Marshal(accessPackageResourceRequest)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resourceDoesNotExist := func(resp *http.Response, o *odata.OData) bool {
		return o != nil && o.Error != nil && o.Error.Match(odata.ErrorResourceDoesNotExist)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: resourceDoesNotExist,
		ValidStatusCodes:       []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/identityGovernance/entitlementManagement/accessPackageResourceRequests",
			HasTenantId: true,
		},
	})

	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageResourceRequestClient.BaseClient.Post(): %v ", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newAccessPackageResourceRequest AccessPackageResourceRequest
	if err := json.Unmarshal(respBody, &newAccessPackageResourceRequest); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	// The endpoint does not actually return the AccessPackageResources created which makes implementation impossible
	// As workaround make sure to tag responses that 201 back with what we've just posted
	newAccessPackageResourceRequest.AccessPackageResource = accessPackageResourceRequest.AccessPackageResource

	if newAccessPackageResourceRequest.CatalogId == nil {
		return &newAccessPackageResourceRequest, status, fmt.Errorf("response has no catalogId")
	}
	if newAccessPackageResourceRequest.AccessPackageResource == nil {
		return &newAccessPackageResourceRequest, status, fmt.Errorf("response has no accessPackageResource")
	}
	if newAccessPackageResourceRequest.AccessPackageResource.OriginId == nil {
		return &newAccessPackageResourceRequest, status, fmt.Errorf("response has no originId for accessPackageResource")
	}

	// Optionally poll For ID
	if pollForId {
		resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
			ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
			ValidStatusCodes:       []int{http.StatusOK},
			Uri: Uri{
				Entity: fmt.Sprintf("/identityGovernance/entitlementManagement/accessPackageCatalogs/%s/accessPackageResources", *newAccessPackageResourceRequest.CatalogId), //Catalog ID Used in Request
				Params: odata.Query{
					Filter: fmt.Sprintf("startswith(originId,'%s')", *newAccessPackageResourceRequest.AccessPackageResource.OriginId),
				}.Values(), // The Resource we made a request to add
				HasTenantId: true,
			},
		})
		if err != nil {
			return nil, status, fmt.Errorf("pollForId: AccessPackageResourceClient.BaseClient.Get(): %v", err)
		}

		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
		}
		var data struct {
			AccessPackageResources []AccessPackageResource `json:"value"`
		}

		if err := json.Unmarshal(respBody, &data); err != nil {
			return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
		}

		if len(data.AccessPackageResources) == 0 {
			return &newAccessPackageResourceRequest, status, fmt.Errorf("failed to poll for new AccessPackageResource")
		}

		accessPackageResource := data.AccessPackageResources[0]
		newAccessPackageResourceRequest.AccessPackageResource.ID = accessPackageResource.ID                     // Set resultant ID
		newAccessPackageResourceRequest.AccessPackageResource.ResourceType = accessPackageResource.ResourceType // Set resource type (is mandatory for role scopes)
	}

	return &newAccessPackageResourceRequest, status, nil
}

// Get retrieves an AccessPackageResourceRequest
// This uses OData Filter as there is no native Get method
func (c *AccessPackageResourceRequestClient) Get(ctx context.Context, id string) (*AccessPackageResourceRequest, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity: "/identityGovernance/entitlementManagement/accessPackageResourceRequests",
			Params: odata.Query{
				Filter: fmt.Sprintf("startswith(id,'%s')", id),
			}.Values(),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AccessPackageResourceRequestClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var accessPackageResourceRequest AccessPackageResourceRequest
	if err := json.Unmarshal(respBody, &accessPackageResourceRequest); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &accessPackageResourceRequest, status, nil
}

// Delete removes a AccessPackageResourceRequest
// This is a pseudo delete method because there is no Delete endpoint.
// Instead, we construct and send a request to delete a resource assignment.
// See tests for example usage.
// Docs: https://docs.microsoft.com/en-us/graph/api/accesspackageresourcerequest-post?view=graph-rest-beta#example-5-create-an-accesspackageresourcerequest-for-removing-a-resource
func (c *AccessPackageResourceRequestClient) Delete(ctx context.Context, accessPackageResourceRequest AccessPackageResourceRequest) (int, error) {
	var status int

	// Deletion request based off the initial resource request
	newAccessPackageResourceRequest := AccessPackageResourceRequest{
		CatalogId:   accessPackageResourceRequest.CatalogId,
		RequestType: utils.StringPtr("AdminRemove"),
		AccessPackageResource: &AccessPackageResource{
			ID: accessPackageResourceRequest.AccessPackageResource.ID, // This is known after Create with poll
		},
	}

	body, err := json.Marshal(newAccessPackageResourceRequest)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/identityGovernance/entitlementManagement/accessPackageResourceRequests",
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AccessPackageResourceRequestClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	return status, nil
}
