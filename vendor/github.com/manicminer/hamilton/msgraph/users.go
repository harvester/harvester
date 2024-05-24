package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// UsersClient performs operations on Users.
type UsersClient struct {
	BaseClient Client
}

// NewUsersClient returns a new UsersClient.
func NewUsersClient(tenantId string) *UsersClient {
	return &UsersClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of Users, optionally queried using OData.
func (c *UsersClient) List(ctx context.Context, query odata.Query) (*[]User, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/users",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("UsersClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Users []User `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.Users, status, nil
}

// Create creates a new User.
func (c *UsersClient) Create(ctx context.Context, user User) (*User, int, error) {
	var status int

	body, err := json.Marshal(user)
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
			Entity:      "/users",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("UsersClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newUser User
	if err := json.Unmarshal(respBody, &newUser); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newUser, status, nil
}

// Get retrieves a User.
func (c *UsersClient) Get(ctx context.Context, id string, query odata.Query) (*User, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("UsersClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var user User
	if err := json.Unmarshal(respBody, &user); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &user, status, nil
}

// GetWithSchemaExtensions retrieves a User, including the values for any specified schema extensions
func (c *UsersClient) GetWithSchemaExtensions(ctx context.Context, id string, query odata.Query, schemaExtensions *[]SchemaExtensionData) (*User, int, error) {
	var sel []string
	if len(query.Select) > 0 {
		sel = query.Select
		query.Select = []string{}
	}

	user, status, err := c.Get(ctx, id, query)
	if err != nil {
		return user, status, err
	}

	if len(sel) > 0 {
		query.Select = sel
	}

	var resp *http.Response
	resp, status, _, err = c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("UsersClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	user.SchemaExtensions = schemaExtensions
	if err := json.Unmarshal(respBody, user); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return user, status, nil
}

// GetDeleted retrieves a deleted User.
func (c *UsersClient) GetDeleted(ctx context.Context, id string, query odata.Query) (*User, int, error) {
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
		return nil, status, fmt.Errorf("UsersClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var user User
	if err := json.Unmarshal(respBody, &user); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &user, status, nil
}

// Update amends an existing User.
func (c *UsersClient) Update(ctx context.Context, user User) (int, error) {
	var status int

	body, err := json.Marshal(user)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s", *user.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("UsersClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a User.
func (c *UsersClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("UsersClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// DeletePermanently removes a deleted User permanently.
func (c *UsersClient) DeletePermanently(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directory/deletedItems/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("UsersClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// ListDeleted retrieves a list of recently deleted users, optionally queried using OData.
func (c *UsersClient) ListDeleted(ctx context.Context, query odata.Query) (*[]User, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/directory/deleteditems/microsoft.graph.user",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, err
	}

	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var data struct {
		DeletedUsers []User `json:"value"`
	}
	if err = json.Unmarshal(respBody, &data); err != nil {
		return nil, status, err
	}

	return &data.DeletedUsers, status, nil
}

// RestoreDeleted restores a recently deleted User.
func (c *UsersClient) RestoreDeleted(ctx context.Context, id string) (*User, int, error) {
	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directory/deletedItems/%s/restore", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("UsersClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var restoredUser User
	if err = json.Unmarshal(respBody, &restoredUser); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &restoredUser, status, nil
}

// ListGroupMemberships returns a list of Groups the user is member of, optionally queried using OData.
func (c *UsersClient) ListGroupMemberships(ctx context.Context, id string, query odata.Query) (*[]Group, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		DisablePaging:          query.Top > 0,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/transitiveMemberOf", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("UsersClient.BaseClient.Get(): %v", err)
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

// SendMail sends message specified in the request body.
// TODO: Needs testing with an O365 user principal
func (c *UsersClient) Sendmail(ctx context.Context, id string, message MailMessage) (int, error) {
	var status int

	body, err := json.Marshal(message)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusOK, http.StatusAccepted},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/sendMail", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("UsersClient.BaseClient.Post(): %v", err)
	}

	return status, nil
}

// GetManager retrieves an user or organizational contact assigned as the user's manager.
func (c *UsersClient) GetManager(ctx context.Context, id string) (*User, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity: fmt.Sprintf("/users/%s/manager", id),
		},
	})
	if err != nil {
		return nil, status, err
	}

	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var manager User
	if err := json.Unmarshal(respBody, &manager); err != nil {
		return nil, status, err
	}

	return &manager, status, nil
}

// AssignManager assigns a user's manager.
func (c *UsersClient) AssignManager(ctx context.Context, id string, manager User) (int, error) {
	var status int

	body, err := json.Marshal(struct {
		Manager odata.Id `json:"@odata.id"`
	}{
		Manager: *manager.ODataId,
	})
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Put(ctx, PutHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/manager/$ref", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("UsersClient.BaseClient.Post(): %v", err)
	}

	return status, nil
}

// DeleteManager removes a user's manager assignment.
func (c *UsersClient) DeleteManager(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ValidStatusCodes: []int{http.StatusNoContent},
		Uri: Uri{
			Entity: fmt.Sprintf("/users/%s/manager/$ref", id),
		},
	})
	if err != nil {
		return status, err
	}

	return status, nil
}
