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

// ServicePrincipalsClient performs operations on Service Principals.
type ServicePrincipalsClient struct {
	BaseClient Client
}

// NewServicePrincipalsClient returns a new ServicePrincipalsClient.
func NewServicePrincipalsClient(tenantId string) *ServicePrincipalsClient {
	return &ServicePrincipalsClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of Service Principals, optionally queried using OData.
func (c *ServicePrincipalsClient) List(ctx context.Context, query odata.Query) (*[]ServicePrincipal, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/servicePrincipals",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		ServicePrincipals []ServicePrincipal `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.ServicePrincipals, status, nil
}

// Create creates a new Service Principal.
func (c *ServicePrincipalsClient) Create(ctx context.Context, servicePrincipal ServicePrincipal) (*ServicePrincipal, int, error) {
	var status int

	body, err := json.Marshal(servicePrincipal)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	appNotReplicated := func(resp *http.Response, o *odata.OData) bool {
		if resp != nil && o != nil && o.Error != nil {
			if resp.StatusCode == http.StatusBadRequest {
				return o.Error.Match(odata.ErrorServicePrincipalInvalidAppId)
			}
			if resp.StatusCode == http.StatusForbidden {
				return o.Error.Match(odata.ErrorServicePrincipalAppInOtherTenant)
			}
		}
		return false
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: appNotReplicated,
		OData: odata.Query{
			Metadata: odata.MetadataFull,
		},
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/servicePrincipals",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newServicePrincipal ServicePrincipal
	if err := json.Unmarshal(respBody, &newServicePrincipal); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newServicePrincipal, status, nil
}

// Get retrieves a Service Principal.
func (c *ServicePrincipalsClient) Get(ctx context.Context, id string, query odata.Query) (*ServicePrincipal, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var servicePrincipal ServicePrincipal
	if err := json.Unmarshal(respBody, &servicePrincipal); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &servicePrincipal, status, nil
}

// Update amends an existing Service Principal.
func (c *ServicePrincipalsClient) Update(ctx context.Context, servicePrincipal ServicePrincipal) (int, error) {
	var status int

	if servicePrincipal.ID == nil {
		return status, errors.New("cannot update service principal with nil ID")
	}

	body, err := json.Marshal(servicePrincipal)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s", *servicePrincipal.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a Service Principal.
func (c *ServicePrincipalsClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// ListOwners retrieves the owners of the specified Service Principal.
// id is the object ID of the service principal.
func (c *ServicePrincipalsClient) ListOwners(ctx context.Context, id string) (*[]string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/owners", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Owners []struct {
			Type string `json:"@odata.type"`
			Id   string `json:"id"`
		} `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	ret := make([]string, len(data.Owners))
	for i, v := range data.Owners {
		ret[i] = v.Id
	}

	return &ret, status, nil
}

// GetOwner retrieves a single owner for the specified Service Principal.
// servicePrincipalId is the object ID of the service principal.
// ownerId is the object ID of the owning object.
func (c *ServicePrincipalsClient) GetOwner(ctx context.Context, servicePrincipalId, ownerId string) (*string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id", "url"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/owners/%s/$ref", servicePrincipalId, ownerId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Context string `json:"@odata.context"`
		Type    string `json:"@odata.type"`
		Id      string `json:"id"`
		Url     string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.Id, status, nil
}

// AddOwners adds owners to a Service Principal.
// First populate the `owners` field, then call this method
func (c *ServicePrincipalsClient) AddOwners(ctx context.Context, servicePrincipal *ServicePrincipal) (int, error) {
	var status int

	if servicePrincipal.ID == nil {
		return status, errors.New("cannot update service principal with nil ID")
	}
	if servicePrincipal.Owners == nil {
		return status, errors.New("cannot update service principal with nil Owners")
	}

	for _, owner := range *servicePrincipal.Owners {
		// don't fail if an owner already exists
		checkOwnerAlreadyExists := func(resp *http.Response, o *odata.OData) bool {
			if resp != nil && resp.StatusCode == http.StatusBadRequest && o != nil && o.Error != nil {
				return o.Error.Match(odata.ErrorAddedObjectReferencesAlreadyExist)
			}
			return false
		}

		body, err := json.Marshal(DirectoryObject{ODataId: owner.ODataId})
		if err != nil {
			return status, fmt.Errorf("json.Marshal(): %v", err)
		}

		_, status, _, err = c.BaseClient.Post(ctx, PostHttpRequestInput{
			Body:                   body,
			ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
			ValidStatusCodes:       []int{http.StatusNoContent},
			ValidStatusFunc:        checkOwnerAlreadyExists,
			Uri: Uri{
				Entity:      fmt.Sprintf("/servicePrincipals/%s/owners/$ref", *servicePrincipal.ID),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Post(): %v", err)
		}
	}

	return status, nil
}

// RemoveOwners removes owners from a Service Principal.
// servicePrincipalId is the object ID of the service principal.
// ownerIds is a *[]string containing object IDs of owners to remove.
func (c *ServicePrincipalsClient) RemoveOwners(ctx context.Context, servicePrincipalId string, ownerIds *[]string) (int, error) {
	var status int

	if ownerIds == nil {
		return status, errors.New("cannot remove, nil ownerIds")
	}

	for _, ownerId := range *ownerIds {
		// check for ownership before attempting deletion
		if _, status, err := c.GetOwner(ctx, servicePrincipalId, ownerId); err != nil {
			if status == http.StatusNotFound {
				continue
			}
			return status, err
		}

		// despite the above check, sometimes owners are just gone
		checkOwnerGone := func(resp *http.Response, o *odata.OData) bool {
			if resp != nil && resp.StatusCode == http.StatusBadRequest && o != nil && o.Error != nil {
				return o.Error.Match(odata.ErrorRemovedObjectReferencesDoNotExist)
			}
			return false
		}

		_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
			ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
			ValidStatusCodes:       []int{http.StatusNoContent},
			ValidStatusFunc:        checkOwnerGone,
			Uri: Uri{
				Entity:      fmt.Sprintf("/servicePrincipals/%s/owners/%s/$ref", servicePrincipalId, ownerId),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Delete(): %v", err)
		}
	}

	return status, nil
}

// AssignClaimsMappingPolicy assigns a claimsMappingPolicy to a servicePrincipal
func (c *ServicePrincipalsClient) AssignClaimsMappingPolicy(ctx context.Context, servicePrincipal *ServicePrincipal) (int, error) {
	var status int

	if servicePrincipal.ID == nil {
		return status, errors.New("cannot update service principal with nil ID")
	}
	if servicePrincipal.ClaimsMappingPolicies == nil {
		return status, errors.New("cannot update service principal with nil ClaimsMappingPolicies")
	}

	for _, policy := range *servicePrincipal.ClaimsMappingPolicies {
		// don't fail if an owner already exists
		checkPolicyAlreadyExists := func(resp *http.Response, o *odata.OData) bool {
			if resp != nil && resp.StatusCode == http.StatusBadRequest && o != nil && o.Error != nil {
				return o.Error.Match(odata.ErrorAddedObjectReferencesAlreadyExist)
			}
			return false
		}

		body, err := json.Marshal(DirectoryObject{ODataId: policy.ODataId})
		if err != nil {
			return status, fmt.Errorf("json.Marshal(): %v", err)
		}

		_, status, _, err = c.BaseClient.Post(ctx, PostHttpRequestInput{
			Body:                   body,
			ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
			ValidStatusCodes:       []int{http.StatusNoContent},
			ValidStatusFunc:        checkPolicyAlreadyExists,
			Uri: Uri{
				Entity:      fmt.Sprintf("/servicePrincipals/%s/claimsMappingPolicies/$ref", *servicePrincipal.ID),
				HasTenantId: false,
			},
		})
		if err != nil {
			return status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Post(): %v", err)
		}
	}

	return status, nil
}

// ListClaimsMappingPolicy retrieves the claimsMappingPolicies assigned to the specified Service Principal.
// id is the object ID of the service principal.
func (c *ServicePrincipalsClient) ListClaimsMappingPolicy(ctx context.Context, id string) (*[]ClaimsMappingPolicy, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/claimsMappingPolicies", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Policies []ClaimsMappingPolicy `json:"value"`
	}

	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.Policies, status, nil
}

// RemoveClaimsMappingPolicy removes a claimsMappingPolicy from a servicePrincipal
func (c *ServicePrincipalsClient) RemoveClaimsMappingPolicy(ctx context.Context, servicePrincipal *ServicePrincipal, policyIds *[]string) (int, error) {
	var status int

	if policyIds == nil {
		return status, errors.New("cannot remove, nil policyIds")
	}

	assignedPolicies, _, err := c.ListClaimsMappingPolicy(ctx, *servicePrincipal.ID)
	if err != nil {
		return status, fmt.Errorf("ServicePrincipalsClient.BaseClient.ListClaimsMappingPolicy(): %v", err)
	}

	if len(*assignedPolicies) == 0 {
		return http.StatusNoContent, nil
	}

	mapClaimsMappingPolicy := map[string]ClaimsMappingPolicy{}
	for _, v := range *assignedPolicies {
		mapClaimsMappingPolicy[*v.ID] = v
	}

	for _, policyId := range *policyIds {

		// Check if policy is currently assigned
		_, ok := mapClaimsMappingPolicy[policyId]
		if !ok {
			continue
		}

		checkPolicyStatus := func(resp *http.Response, o *odata.OData) bool {
			if resp != nil && resp.StatusCode == http.StatusNotFound && o != nil && o.Error != nil {
				return o.Error.Match(odata.ErrorResourceDoesNotExist)
			}
			return false
		}

		_, status, _, err = c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
			ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
			ValidStatusCodes:       []int{http.StatusNoContent},
			ValidStatusFunc:        checkPolicyStatus,
			Uri: Uri{
				Entity:      fmt.Sprintf("/servicePrincipals/%s/claimsMappingPolicies/%s/$ref", *servicePrincipal.ID, policyId),
				HasTenantId: false,
			},
		})
		if err != nil {
			return status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Delete(): %v", err)
		}
	}

	return status, nil
}

// ListGroupMemberships returns a list of Groups the Service Principal is member of, optionally queried using OData.
func (c *ServicePrincipalsClient) ListGroupMemberships(ctx context.Context, id string, query odata.Query) (*[]Group, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		DisablePaging:          query.Top > 0,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/transitiveMemberOf", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Groups []Group `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.Groups, status, nil
}

// AddPassword appends a new password credential to a Service Principal.
func (c *ServicePrincipalsClient) AddPassword(ctx context.Context, servicePrincipalId string, passwordCredential PasswordCredential) (*PasswordCredential, int, error) {
	var status int

	body, err := json.Marshal(struct {
		PwdCredential PasswordCredential `json:"passwordCredential"`
	}{
		PwdCredential: passwordCredential,
	})
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK, http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/addPassword", servicePrincipalId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newPasswordCredential PasswordCredential
	if err := json.Unmarshal(respBody, &newPasswordCredential); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newPasswordCredential, status, nil
}

// RemovePassword removes a password credential from a Service Principal.
func (c *ServicePrincipalsClient) RemovePassword(ctx context.Context, servicePrincipalId string, keyId string) (int, error) {
	var status int

	body, err := json.Marshal(struct {
		KeyId string `json:"keyId"`
	}{
		KeyId: keyId,
	})
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK, http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/removePassword", servicePrincipalId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Post(): %v", err)
	}

	return status, nil
}

// AddTokenSigningCertificate appends a new self signed certificate (keys and password) to a Service Principal.
func (c *ServicePrincipalsClient) AddTokenSigningCertificate(ctx context.Context, servicePrincipalId string, keyCredential KeyCredential) (*KeyCredential, int, error) {
	var status int

	body, err := json.Marshal(keyCredential)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK, http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/addTokenSigningCertificate", servicePrincipalId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newKeyCredential KeyCredential
	if err := json.Unmarshal(respBody, &newKeyCredential); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newKeyCredential, status, nil
}

// SetPreferredTokenSigningKeyThumbprint sets the field preferredTokenSigningKeyThumbprint for a Service Principal.
func (c *ServicePrincipalsClient) SetPreferredTokenSigningKeyThumbprint(ctx context.Context, servicePrincipalId string, thumbprint string) (int, error) {
	var status int

	body, err := json.Marshal(struct {
		Thumbprint string `json:"preferredTokenSigningKeyThumbprint"`
	}{
		Thumbprint: thumbprint,
	})
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s", servicePrincipalId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// ListOwnedObjects retrieves the owned objects of the specified Service Principal.
// id is the object ID of the service principal.
func (c *ServicePrincipalsClient) ListOwnedObjects(ctx context.Context, id string) (*[]string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/ownedObjects", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, err
	}

	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var data struct {
		OwnedObjects []struct {
			Type string `json:"@odata.type"`
			Id   string `json:"id"`
		} `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, err
	}

	ret := make([]string, len(data.OwnedObjects))
	for i, v := range data.OwnedObjects {
		ret[i] = v.Id
	}

	return &ret, status, nil
}

// ListAppRoleAssignments retrieves a list of appRoleAssignment that users, groups, or client service principals have been granted for the given resource service principal.
func (c *ServicePrincipalsClient) ListAppRoleAssignments(ctx context.Context, resourceId string, query odata.Query) (*[]AppRoleAssignment, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/appRoleAssignedTo", resourceId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AppRoleAssignments []AppRoleAssignment `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.AppRoleAssignments, status, nil
}

// RemoveAppRoleAssignment deletes an appRoleAssignment that a user, group, or client service principal has been granted for a resource service principal.
func (c *ServicePrincipalsClient) RemoveAppRoleAssignment(ctx context.Context, resourceId, appRoleAssignmentId string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/appRoleAssignedTo/%s", resourceId, appRoleAssignmentId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AppRoleAssignmentsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// AssignAppRoleForResource assigns an app role for a resource service principal, to a user, group, or client service principal.
// To grant an app role assignment, you need three identifiers:
//
// principalId: The id of the user, group or client servicePrincipal to which you are assigning the app role.
// resourceId: The id of the resource servicePrincipal which has defined the app role.
// appRoleId: The id of the appRole (defined on the resource service principal) to assign to a user, group, or service principal.
func (c *ServicePrincipalsClient) AssignAppRoleForResource(ctx context.Context, principalId, resourceId, appRoleId string) (*AppRoleAssignment, int, error) {
	var status int

	data := struct {
		PrincipalId string `json:"principalId"`
		ResourceId  string `json:"resourceId"`
		AppRoleId   string `json:"appRoleId"`
	}{
		PrincipalId: principalId,
		ResourceId:  resourceId,
		AppRoleId:   appRoleId,
	}
	body, err := json.Marshal(data)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/servicePrincipals/%s/appRoleAssignedTo", resourceId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ServicePrincipalsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var appRoleAssignment AppRoleAssignment
	if err := json.Unmarshal(respBody, &appRoleAssignment); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &appRoleAssignment, status, nil
}
