package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// MeClient performs operations on the authenticated user.
type MeClient struct {
	BaseClient Client
}

// NewMeClient returns a new MeClient.
func NewMeClient(tenantId string) *MeClient {
	return &MeClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// Get retrieves information about the authenticated user.
func (c *MeClient) Get(ctx context.Context, query odata.Query) (*Me, int, error) {
	var status int

	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/me",
			HasTenantId: false,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("MeClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var me Me
	if err := json.Unmarshal(respBody, &me); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &me, status, nil
}

// GetProfile retrieves the profile of the authenticated user.
func (c *MeClient) GetProfile(ctx context.Context, query odata.Query) (*Me, int, error) {
	var status int

	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/me/profile",
			HasTenantId: false,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("MeClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var me Me
	if err := json.Unmarshal(respBody, &me); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &me, status, nil
}

// SendMail sends message specified in the request body.
// TODO: Needs testing with an O365 user principal
func (c *MeClient) Sendmail(ctx context.Context, message MailMessage) (int, error) {
	var status int

	body, err := json.Marshal(message)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:             body,
		ValidStatusCodes: []int{http.StatusOK, http.StatusAccepted},
		Uri: Uri{
			Entity:      "/me/sendMail",
			HasTenantId: false,
		},
	})
	if err != nil {
		return status, fmt.Errorf("MeClient.BaseClient.Post(): %v", err)
	}

	return status, nil
}
