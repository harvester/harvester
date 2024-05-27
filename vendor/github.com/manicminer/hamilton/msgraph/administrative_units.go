package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// AdministrativeUnitsClient performs operations on Administrative Units
type AdministrativeUnitsClient struct {
	BaseClient Client
}

// NewAdministrativeUnitsClient returns a new AdministrativeUnitsClient.
func NewAdministrativeUnitsClient(tenantId string) *AdministrativeUnitsClient {
	return &AdministrativeUnitsClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of AdministrativeUnits, optionally queried using OData.
func (c *AdministrativeUnitsClient) List(ctx context.Context, query odata.Query) (*[]AdministrativeUnit, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/administrativeUnits",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AdministrativeUnits []AdministrativeUnit `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.AdministrativeUnits, status, nil
}

// Create creates a new AdministrativeUnit.
func (c *AdministrativeUnitsClient) Create(ctx context.Context, administrativeUnit AdministrativeUnit) (*AdministrativeUnit, int, error) {
	var status int

	body, err := json.Marshal(administrativeUnit)
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
			Entity:      "/administrativeUnits",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newAdministrativeUnit AdministrativeUnit
	if err := json.Unmarshal(respBody, &newAdministrativeUnit); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newAdministrativeUnit, status, nil
}

// Get retrieves an AdministrativeUnit
func (c *AdministrativeUnitsClient) Get(ctx context.Context, id string, query odata.Query) (*AdministrativeUnit, int, error) {
	query.Metadata = odata.MetadataFull

	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/administrativeUnits/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var administrativeUnit AdministrativeUnit
	if err := json.Unmarshal(respBody, &administrativeUnit); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &administrativeUnit, status, nil
}

// Update amends an existing AdministrativeUnit.
func (c *AdministrativeUnitsClient) Update(ctx context.Context, administrativeUnit AdministrativeUnit) (int, error) {
	var status int

	body, err := json.Marshal(administrativeUnit)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Patch(ctx, PatchHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/administrativeUnits/%s", *administrativeUnit.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Patch(): %v", err)
	}

	return status, nil
}

// Delete removes a AdministrativeUnit.
func (c *AdministrativeUnitsClient) Delete(ctx context.Context, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/administrativeUnits/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AdministrativeUnits.BaseClient.Get(): %v", err)
	}

	return status, nil
}

// ListMembers retrieves the members of the specified AdministrativeUnit.
func (c *AdministrativeUnitsClient) ListMembers(ctx context.Context, administrativeUnitId string) (*[]string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/administrativeUnits/%s/members", administrativeUnitId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Get(): %v", err)
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

// GetMember retrieves a single member of the specified AdministrativeUnit.
func (c *AdministrativeUnitsClient) GetMember(ctx context.Context, administrativeUnitId, memberId string) (*string, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData: odata.Query{
			Select: []string{"id", "url"},
		},
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/administrativeUnits/%s/members/%s/$ref", administrativeUnitId, memberId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Get(): %v", err)
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

// AddMembers adds new members to a AdministrativeUnit.
func (c *AdministrativeUnitsClient) AddMembers(ctx context.Context, administrativeUnitId string, members *Members) (int, error) {
	var status int

	if members == nil || len(*members) == 0 {
		return status, fmt.Errorf("no members specified")
	}

	for _, member := range *members {
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
			Body:                   body,
			ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
			ValidStatusCodes:       []int{http.StatusNoContent},
			ValidStatusFunc:        checkMemberAlreadyExists,
			Uri: Uri{
				Entity:      fmt.Sprintf("/administrativeUnits/%s/members/$ref", administrativeUnitId),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Post(): %v", err)
		}
	}

	return status, nil
}

// RemoveMembers removes members from a AdministrativeUnit.
func (c *AdministrativeUnitsClient) RemoveMembers(ctx context.Context, administrativeUnitId string, memberIds *[]string) (int, error) {
	var status int

	if memberIds == nil || len(*memberIds) == 0 {
		return status, fmt.Errorf("no members specified")
	}

	for _, memberId := range *memberIds {
		// check for membership before attempting deletion
		if _, status, err := c.GetMember(ctx, administrativeUnitId, memberId); err != nil {
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
				Entity:      fmt.Sprintf("/administrativeUnits/%s/members/%s/$ref", administrativeUnitId, memberId),
				HasTenantId: true,
			},
		})
		if err != nil {
			return status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Delete(): %v", err)
		}
	}

	return status, nil
}

// ListScopedRoleMembers retrieves the members of the specified AdministrativeUnit.
func (c *AdministrativeUnitsClient) ListScopedRoleMembers(ctx context.Context, administrativeUnitId string, query odata.Query) (*[]ScopedRoleMembership, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/administrativeUnits/%s/scopedRoleMembers", administrativeUnitId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		ScopedRoleMembers []ScopedRoleMembership `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.ScopedRoleMembers, status, nil
}

// GetScopedRoleMember retrieves a single member of the specified AdministrativeUnit.
func (c *AdministrativeUnitsClient) GetScopedRoleMember(ctx context.Context, administrativeUnitId, scopedRoleMembershipId string, query odata.Query) (*ScopedRoleMembership, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/administrativeUnits/%s/scopedRoleMembers/%s", administrativeUnitId, scopedRoleMembershipId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data ScopedRoleMembership
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data, status, nil
}

// AddScopedRoleMember adds a new scoped role membership for a AdministrativeUnit.
func (c *AdministrativeUnitsClient) AddScopedRoleMember(ctx context.Context, administrativeUnitId string, scopedRoleMembership ScopedRoleMembership) (*ScopedRoleMembership, int, error) {
	var status int

	body, err := json.Marshal(scopedRoleMembership)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/administrativeUnits/%s/scopedRoleMembers", administrativeUnitId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data ScopedRoleMembership
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data, status, nil
}

// RemoveScopedRoleMembers removes members from a AdministrativeUnit.
func (c *AdministrativeUnitsClient) RemoveScopedRoleMembers(ctx context.Context, administrativeUnitId, scopedRoleMembershipId string) (int, error) {
	var status int

	var err error
	_, status, _, err = c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/administrativeUnits/%s/scopedRoleMembers/%s", administrativeUnitId, scopedRoleMembershipId),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AdministrativeUnitsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}
