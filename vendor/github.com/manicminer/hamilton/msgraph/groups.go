package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// GroupsClient performs operations on Groups.
type GroupsClient struct {
	BaseClient Client
}

// NewGroupsClient returns a new GroupsClient.
func NewGroupsClient(tenantId string) *GroupsClient {
	return &GroupsClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of Groups, optionally queried using OData.
func (c *GroupsClient) List(ctx context.Context, query odata.Query) (*[]Group, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/groups",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Get(): %v", err)
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

// Create creates a new Group.
func (c *GroupsClient) Create(ctx context.Context, group Group) (*Group, int, error) {
	var status int

	body, err := json.Marshal(group)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	ownersNotReplicated := func(resp *http.Response, o *odata.OData) bool {
		if resp != nil && resp.StatusCode == http.StatusBadRequest && o != nil && o.Error != nil {
			return o.Error.Match(odata.ErrorResourceDoesNotExist)
		}
		return false
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: ownersNotReplicated,
		OData: odata.Query{
			Metadata: odata.MetadataFull,
		},
		ValidStatusCodes: []int{http.StatusCreated},
		Uri: Uri{
			Entity:      "/groups",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newGroup Group
	if err := json.Unmarshal(respBody, &newGroup); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newGroup, status, nil
}

// Get retrieves a Group.
func (c *GroupsClient) Get(ctx context.Context, id string, query odata.Query) (*Group, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/groups/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var group Group
	if err := json.Unmarshal(respBody, &group); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &group, status, nil
}

// GetWithSchemaExtensions retrieves a Group, including the values for any specified schema extensions
func (c *GroupsClient) GetWithSchemaExtensions(ctx context.Context, id string, query odata.Query, schemaExtensions *[]SchemaExtensionData) (*Group, int, error) {
	var sel []string
	if len(query.Select) > 0 {
		sel = query.Select
		query.Select = []string{}
	}

	group, status, err := c.Get(ctx, id, query)
	if err != nil {
		return group, status, err
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
			Entity:      fmt.Sprintf("/groups/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	group.SchemaExtensions = schemaExtensions
	if err := json.Unmarshal(respBody, group); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return group, status, nil
}

// GetDeleted retrieves a deleted O365 Group.
func (c *GroupsClient) GetDeleted(ctx context.Context, id string, query odata.Query) (*Group, int, error) {
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
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var group Group
	if err := json.Unmarshal(respBody, &group); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &group, status, nil
}

// Update amends an existing Group.
func (c *GroupsClient) Update(ctx context.Context, group Group) (int, error) {
	var status int

	if group.ID == nil {
		return status, fmt.Errorf("cannot update group with nil ID")
	}

	groupId := *group.ID
	group.ID = nil

	body, err := json.Marshal(group)
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
			Entity:      fmt.Sprintf("/groups/%s", groupId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("GroupsClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a Group.
func (c *GroupsClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/groups/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("GroupsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// DeletePermanently removes a deleted O365 Group permanently.
func (c *GroupsClient) DeletePermanently(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directory/deletedItems/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("GroupsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

// ListDeleted retrieves a list of recently deleted O365 groups, optionally queried using OData.
func (c *GroupsClient) ListDeleted(ctx context.Context, query odata.Query) (*[]Group, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/directory/deleteditems/microsoft.graph.group",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, err
	}

	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var data struct {
		DeletedGroups []Group `json:"value"`
	}
	if err = json.Unmarshal(respBody, &data); err != nil {
		return nil, status, err
	}

	return &data.DeletedGroups, status, nil
}

// RestoreDeleted restores a recently deleted O365 Group.
func (c *GroupsClient) RestoreDeleted(ctx context.Context, id string) (*Group, int, error) {
	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/directory/deletedItems/%s/restore", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var restoredGroup Group
	if err = json.Unmarshal(respBody, &restoredGroup); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &restoredGroup, status, nil
}

// ListMembers retrieves the members of the specified Group.
// id is the object ID of the group.
func (c *GroupsClient) ListMembers(ctx context.Context, id string) (*[]string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/groups/%s/members", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Get(): %v", err)
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

// GetMember retrieves a single member of the specified Group.
// groupId is the object ID of the group.
// memberId is the object ID of the member object.
func (c *GroupsClient) GetMember(ctx context.Context, groupId, memberId string) (*string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id", "url"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/groups/%s/members/%s/$ref", groupId, memberId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Get(): %v", err)
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

// AddMembers adds new members to a Group.
// First populate the `members` field, then call this method
func (c *GroupsClient) AddMembers(ctx context.Context, group *Group) (int, error) {
	var status int

	if group.Members == nil || len(*group.Members) == 0 {
		return status, fmt.Errorf("no members specified")
	}

	for _, member := range *group.Members {
		// don't fail if an member already exists
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
			Body:                   body,
			ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
			ValidStatusCodes:       []int{http.StatusNoContent},
			ValidStatusFunc:        checkMemberAlreadyExists,
			Uri: Uri{
				Entity:      fmt.Sprintf("/groups/%s/members/$ref", *group.ID),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("GroupsClient.BaseClient.Post(): %v", err)
		}
	}

	return status, nil
}

// RemoveMembers removes members from a Group.
// groupId is the object ID of the group.
// memberIds is a *[]string containing object IDs of members to remove.
func (c *GroupsClient) RemoveMembers(ctx context.Context, id string, memberIds *[]string) (int, error) {
	var status int

	if memberIds == nil || len(*memberIds) == 0 {
		return status, fmt.Errorf("no members specified")
	}

	for _, memberId := range *memberIds {
		// check for membership before attempting deletion
		if _, status, err := c.GetMember(ctx, id, memberId); err != nil {
			if status == http.StatusNotFound {
				continue
			}
			return status, err
		}

		// despite the above check, sometimes members are just gone
		checkMemberGone := func(resp *http.Response, o *odata.OData) bool {
			if resp != nil && resp.StatusCode == http.StatusBadRequest && o != nil && o.Error != nil {
				return o.Error.Match(odata.ErrorRemovedObjectReferencesDoNotExist)
			}
			return false
		}

		var err error
		_, status, _, err = c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
			ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
			ValidStatusCodes:       []int{http.StatusNoContent},
			ValidStatusFunc:        checkMemberGone,
			Uri: Uri{
				Entity:      fmt.Sprintf("/groups/%s/members/%s/$ref", id, memberId),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("GroupsClient.BaseClient.Delete(): %v", err)
		}
	}

	return status, nil
}

// ListOwners retrieves the owners of the specified Group.
// id is the object ID of the group.
func (c *GroupsClient) ListOwners(ctx context.Context, id string) (*[]string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/groups/%s/owners", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Get(): %v", err)
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

// GetOwner retrieves a single owner for the specified Group.
// groupId is the object ID of the group.
// ownerId is the object ID of the owning object.
func (c *GroupsClient) GetOwner(ctx context.Context, groupId, ownerId string) (*string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id", "url"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/groups/%s/owners/%s/$ref", groupId, ownerId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("GroupsClient.BaseClient.Get(): %v", err)
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

// AddOwners adds new owners to a Group.
// First populate the `owners` field, then call this method
func (c *GroupsClient) AddOwners(ctx context.Context, group *Group) (int, error) {
	var status int

	if group.Owners == nil || len(*group.Owners) == 0 {
		return status, fmt.Errorf("no owners specified")
	}

	for _, owner := range *group.Owners {
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
				Entity:      fmt.Sprintf("/groups/%s/owners/$ref", *group.ID),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("GroupsClient.BaseClient.Post(): %v", err)
		}
	}

	return status, nil
}

// RemoveOwners removes owners from a Group.
// groupId is the object ID of the group.
// ownerIds is a *[]string containing object IDs of owners to remove.
func (c *GroupsClient) RemoveOwners(ctx context.Context, id string, ownerIds *[]string) (int, error) {
	var status int

	if ownerIds == nil || len(*ownerIds) == 0 {
		return status, fmt.Errorf("no owners specified")
	}

	for _, ownerId := range *ownerIds {
		// check for ownership before attempting deletion
		if _, status, err := c.GetOwner(ctx, id, ownerId); err != nil {
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
				Entity:      fmt.Sprintf("/groups/%s/owners/%s/$ref", id, ownerId),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("GroupsClient.BaseClient.Delete(): %v", err)
		}
	}

	return status, nil
}
