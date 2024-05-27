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

// ApplicationsClient performs operations on Applications.
type ApplicationsClient struct {
	BaseClient Client
}

// NewApplicationsClient returns a new ApplicationsClient
func NewApplicationsClient(tenantId string) *ApplicationsClient {
	return &ApplicationsClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of Applications, optionally queried using OData.
func (c *ApplicationsClient) List(ctx context.Context, query odata.Query) (*[]Application, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/applications",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Applications []Application `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.Applications, status, nil
}

// Create creates a new Application.
func (c *ApplicationsClient) Create(ctx context.Context, application Application) (*Application, int, error) {
	var status int

	body, err := json.Marshal(application)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body: body,
		OData: odata.Query{
			Metadata: odata.MetadataFull,
		},
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/applications",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newApplication Application
	if err := json.Unmarshal(respBody, &newApplication); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newApplication, status, nil
}

// Get retrieves an Application manifest.
func (c *ApplicationsClient) Get(ctx context.Context, id string, query odata.Query) (*Application, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var application Application
	if err := json.Unmarshal(respBody, &application); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &application, status, nil
}

// GetDeleted retrieves a deleted Application manifest.
// id is the object ID of the application.
func (c *ApplicationsClient) GetDeleted(ctx context.Context, id string, query odata.Query) (*Application, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directory/deletedItems/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var application Application
	if err := json.Unmarshal(respBody, &application); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &application, status, nil
}

// Update amends the manifest of an existing Application.
func (c *ApplicationsClient) Update(ctx context.Context, application Application) (int, error) {
	var status int

	if application.ID == nil {
		return status, errors.New("ApplicationsClient.Update(): cannot update application with nil ID")
	}

	body, err := json.Marshal(application)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	checkApplicationConsistency := func(resp *http.Response, o *odata.OData) bool {
		if resp == nil {
			return false
		}
		if resp.StatusCode == http.StatusNotFound {
			return true
		}
		if resp.StatusCode == http.StatusBadRequest && o != nil && o.Error != nil {
			return o.Error.Match(odata.ErrorCannotDeleteOrUpdateEnabledEntitlement)
		}
		return false
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: checkApplicationConsistency,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s", *application.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ApplicationsClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes an Application.
func (c *ApplicationsClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ApplicationsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// DeletePermanently removes a deleted Application permanently.
// id is the object ID of the application.
func (c *ApplicationsClient) DeletePermanently(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directory/deletedItems/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ApplicationsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// ListDeleted retrieves a list of recently deleted applications, optionally queried using OData.
func (c *ApplicationsClient) ListDeleted(ctx context.Context, query odata.Query) (*[]Application, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/directory/deleteditems/microsoft.graph.application",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, err
	}

	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var data struct {
		DeletedApps []Application `json:"value"`
	}
	if err = json.Unmarshal(respBody, &data); err != nil {
		return nil, status, err
	}

	return &data.DeletedApps, status, nil
}

// RestoreDeleted restores a recently deleted Application.
// id is the object ID of the application.
func (c *ApplicationsClient) RestoreDeleted(ctx context.Context, id string) (*Application, int, error) {
	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directory/deletedItems/%s/restore", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var restoredApplication Application
	if err = json.Unmarshal(respBody, &restoredApplication); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &restoredApplication, status, nil
}

// AddPassword appends a new password credential to an Application.
func (c *ApplicationsClient) AddPassword(ctx context.Context, applicationId string, passwordCredential PasswordCredential) (*PasswordCredential, int, error) {
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
			Entity:      fmt.Sprintf("/applications/%s/addPassword", applicationId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Post(): %v", err)
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

// RemovePassword removes a password credential from an Application.
func (c *ApplicationsClient) RemovePassword(ctx context.Context, applicationId string, keyId string) (int, error) {
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
			Entity:      fmt.Sprintf("/applications/%s/removePassword", applicationId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ApplicationsClient.BaseClient.Post(): %v", err)
	}

	return status, nil
}

// ListOwners retrieves the owners of the specified Application.
// id is the object ID of the application.
func (c *ApplicationsClient) ListOwners(ctx context.Context, id string) (*[]string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/owners", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Get(): %v", err)
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

// GetOwner retrieves a single owner for the specified Application.
// applicationId is the object ID of the application.
// ownerId is the object ID of the owning object.
func (c *ApplicationsClient) GetOwner(ctx context.Context, applicationId, ownerId string) (*string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id", "url"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/owners/%s/$ref", applicationId, ownerId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Get(): %v", err)
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

// AddOwners adds new owners to an Application.
// First populate the `owners` field, then call this method
func (c *ApplicationsClient) AddOwners(ctx context.Context, application *Application) (int, error) {
	var status int

	if application.ID == nil {
		return status, errors.New("cannot update application with nil ID")
	}
	if application.Owners == nil {
		return status, errors.New("cannot update application with nil Owners")
	}

	for _, owner := range *application.Owners {
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
				Entity:      fmt.Sprintf("/applications/%s/owners/$ref", *application.ID),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("ApplicationsClient.BaseClient.Post(): %v", err)
		}
	}

	return status, nil
}

// RemoveOwners removes owners from an Application.
// applicationId is the object ID of the application.
// ownerIds is a *[]string containing object IDs of owners to remove.
func (c *ApplicationsClient) RemoveOwners(ctx context.Context, applicationId string, ownerIds *[]string) (int, error) {
	var status int

	if ownerIds == nil {
		return status, errors.New("cannot remove, nil ownerIds")
	}

	for _, ownerId := range *ownerIds {
		// check for ownership before attempting deletion
		if _, status, err := c.GetOwner(ctx, applicationId, ownerId); err != nil {
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

		var err error
		_, status, _, err = c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
			ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
			ValidStatusCodes:       []int{http.StatusNoContent},
			ValidStatusFunc:        checkOwnerGone,
			Uri: Uri{
				Entity:      fmt.Sprintf("/applications/%s/owners/%s/$ref", applicationId, ownerId),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("ApplicationsClient.BaseClient.Delete(): %v", err)
		}
	}

	return status, nil
}

func (c *ApplicationsClient) ListExtensions(ctx context.Context, id string, query odata.Query) (*[]ApplicationExtension, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/extensionProperties", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.List(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		ApplicationExtension []ApplicationExtension `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.ApplicationExtension, status, nil
}

// Create creates a new ApplicationExtension.
func (c *ApplicationsClient) CreateExtension(ctx context.Context, applicationExtension ApplicationExtension, id string) (*ApplicationExtension, int, error) {
	var status int

	body, err := json.Marshal(applicationExtension)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/extensionProperties", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newApplicationExtension ApplicationExtension
	if err := json.Unmarshal(respBody, &newApplicationExtension); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newApplicationExtension, status, nil
}

// DeleteExtension removes an Application Extension.
func (c *ApplicationsClient) DeleteExtension(ctx context.Context, applicationId, extensionId string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/extensionProperties/%s", applicationId, extensionId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ApplicationsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// UploadLogo uploads the application logo which should be a gif, jpeg or png image
func (c *ApplicationsClient) UploadLogo(ctx context.Context, applicationId, contentType string, logoData []byte) (int, error) {
	var status int

	_, status, _, err := c.BaseClient.Put(ctx, PutHttpRequestInput{
		Body:                   logoData,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ContentType:            contentType,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/logo", applicationId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ApplicationsClient.BaseClient.Put(): %v", err)
	}

	return status, nil
}

// ListFederatedIdentityCredentials returns the federated identity credentials for an application
func (c *ApplicationsClient) ListFederatedIdentityCredentials(ctx context.Context, applicationId string, query odata.Query) (*[]FederatedIdentityCredential, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/federatedIdentityCredentials", applicationId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		FederatedIdentityCredentials []FederatedIdentityCredential `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.FederatedIdentityCredentials, status, nil
}

// GetFederatedIdentityCredential returns the federated identity credentials for an application
func (c *ApplicationsClient) GetFederatedIdentityCredential(ctx context.Context, applicationId, credentialId string, query odata.Query) (*FederatedIdentityCredential, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/federatedIdentityCredentials/%s", applicationId, credentialId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data FederatedIdentityCredential
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data, status, nil
}

// CreateFederatedIdentityCredential adds a new federated identity credential for an application
func (c *ApplicationsClient) CreateFederatedIdentityCredential(ctx context.Context, applicationId string, credential FederatedIdentityCredential) (*FederatedIdentityCredential, int, error) {
	var status int

	body, err := json.Marshal(credential)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/federatedIdentityCredentials", applicationId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var result FederatedIdentityCredential
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &result, status, nil
}

// UpdateFederatedIdentityCredential updates an existing federated identity credential for an application
func (c *ApplicationsClient) UpdateFederatedIdentityCredential(ctx context.Context, applicationId string, credential FederatedIdentityCredential) (int, error) {
	var status int

	if credential.ID == nil {
		return status, errors.New("ApplicationsClient.UpdateFederatedIdentityCredential(): cannot update federated identity credential with nil ID")
	}

	body, err := json.Marshal(credential)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/federatedIdentityCredentials/%s", applicationId, *credential.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ApplicationsClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// DeleteFederatedIdentityCredential removes a federated identity credential from an application
func (c *ApplicationsClient) DeleteFederatedIdentityCredential(ctx context.Context, applicationId, credentialId string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/applications/%s/federatedIdentityCredentials/%s", applicationId, credentialId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("ApplicationsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}
