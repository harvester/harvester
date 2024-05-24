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

// DirectoryRolesClient performs operations on DirectoryRoles.
type DirectoryRolesClient struct {
	BaseClient Client
}

// NewDirectoryRolesClient returns a new DirectoryRolesClient
func NewDirectoryRolesClient(tenantId string) *DirectoryRolesClient {
	return &DirectoryRolesClient{
		BaseClient: NewClient(Version10, tenantId),
	}
}

// List returns a list of DirectoryRoles activated in the tenant.
func (c *DirectoryRolesClient) List(ctx context.Context) (*[]DirectoryRole, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/directoryRoles",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryRolesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		DirectoryRoles []DirectoryRole `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.DirectoryRoles, status, nil
}

// Get retrieves a DirectoryRole manifest.
func (c *DirectoryRolesClient) Get(ctx context.Context, id string) (*DirectoryRole, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directoryRoles/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryRolesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var dirRole DirectoryRole
	if err := json.Unmarshal(respBody, &dirRole); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &dirRole, status, nil
}

// GetByTemplateId retrieves a DirectoryRole manifest for a DirectoryRoleTemplate id.
func (c *DirectoryRolesClient) GetByTemplateId(ctx context.Context, templateId string) (*DirectoryRole, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directoryRoles/roleTemplateId=%s", templateId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryRolesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var dirRole DirectoryRole
	if err := json.Unmarshal(respBody, &dirRole); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &dirRole, status, nil
}

// ListMembers retrieves the members of the specified directory role.
// id is the object ID of the directory role.
func (c *DirectoryRolesClient) ListMembers(ctx context.Context, id string) (*[]string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData: odata.Query{
			Select: []string{"id"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directoryRoles/%s/members", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryRolesClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Members []struct {
			Type string `json:"@odata.type"`
			Id   string `json:"id"`
		} `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	ret := make([]string, len(data.Members))
	for i, v := range data.Members {
		ret[i] = v.Id
	}

	return &ret, status, nil
}

// AddMembers adds new members to a Directory Role.
// First populate the `members` field, then call this method
func (c *DirectoryRolesClient) AddMembers(ctx context.Context, directoryRole *DirectoryRole) (int, error) {
	var status int

	if directoryRole.ID == nil {
		return status, errors.New("cannot update directory role with nil ID")
	}
	if directoryRole.Members == nil {
		return status, errors.New("cannot update directory role with nil Owners")
	}

	for _, member := range *directoryRole.Members {
		// don't fail if a member already exists
		checkMemberAlreadyExists := func(resp *http.Response, o *odata.OData) bool {
			if resp != nil && resp.StatusCode == http.StatusBadRequest && o != nil && o.Error != nil {
				return o.Error.Match(odata.ErrorAddedObjectReferencesAlreadyExist)
			}
			return false
		}

		body, err := json.Marshal(DirectoryObject{ODataId: member.ODataId})
		if err != nil {
			return status, fmt.Errorf("json.Marshal(): %v", err)
		}

		_, status, _, err = c.BaseClient.Post(ctx, PostHttpRequestInput{
			Body:             body,
			ValidStatusCodes: []int{http.StatusNoContent},
			ValidStatusFunc:  checkMemberAlreadyExists,
			Uri: Uri{
				Entity:      fmt.Sprintf("/directoryRoles/%s/members/$ref", *directoryRole.ID),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("DirectoryRolesClient.BaseClient.Post(): %v", err)
		}
	}

	return status, nil
}

// RemoveMembers removes members from a Directory Role.
// id is the object ID of the Directory Role.
// memberIds is a *[]string containing object IDs of members to remove.
func (c *DirectoryRolesClient) RemoveMembers(ctx context.Context, directoryRoleId string, memberIds *[]string) (int, error) {
	var status int

	if memberIds == nil {
		return status, errors.New("cannot remove, nil memberIds")
	}

	for _, memberId := range *memberIds {
		// check for membership before attempting deletion
		if _, status, err := c.GetMember(ctx, directoryRoleId, memberId); err != nil {
			if status == http.StatusNotFound {
				continue
			}
			return status, err
		}

		var err error
		_, status, _, err = c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
			ValidStatusCodes: []int{http.StatusNoContent},
			Uri: Uri{
				Entity:      fmt.Sprintf("/directoryRoles/%s/members/%s/$ref", directoryRoleId, memberId),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("DirectoryRolesClient.BaseClient.Delete(): %v", err)
		}
	}

	return status, nil
}

// GetMember retrieves a single member of the specified DirectoryRole.
// directoryRoleId is the object ID of the directory role.
// memberId is the object ID of the member object.
func (c *DirectoryRolesClient) GetMember(ctx context.Context, directoryRoleId, memberId string) (*string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData: odata.Query{
			Select: []string{"id", "url"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directoryRoles/%s/members/%s/$ref", directoryRoleId, memberId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryRolesClient.BaseClient.Get(): %v", err)
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

// Activate activates a directory role. To read a directory role or update its members, it must first be activated in the tenant using role template id.
// This method will attempt to detect whether a role is already activated in the tenant, but may fail in some circumstances.
func (c *DirectoryRolesClient) Activate(ctx context.Context, roleTemplateID string) (*DirectoryRole, int, error) {
	var status int

	// don't fail if a role is already activated
	checkRoleAlreadyActivated := func(resp *http.Response, o *odata.OData) bool {
		if resp != nil && resp.StatusCode == http.StatusBadRequest && o != nil && o.Error != nil {
			return o.Error.Match(odata.ErrorConflictingObjectPresentInDirectory)
		}
		return false
	}

	data := struct {
		RoleTemplateID string `json:"roleTemplateId"`
	}{
		RoleTemplateID: roleTemplateID,
	}
	body, err := json.Marshal(data)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusCreated},
		ValidStatusFunc:  checkRoleAlreadyActivated,
		Uri: Uri{
			Entity:      "/directoryRoles",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("DirectoryRolesClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newDirRole DirectoryRole
	if err := json.Unmarshal(respBody, &newDirRole); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newDirRole, status, nil
}
