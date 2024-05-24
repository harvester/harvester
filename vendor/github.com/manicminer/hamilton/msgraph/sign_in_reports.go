package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// SignInReports Client performs operations on Sign in reports.
type SignInReportsClient struct {
	BaseClient Client
}

// NewSignInReportsClient returns a new SignInReportsClient.
func NewSignInReportsClient(tenantId string) *SignInReportsClient {
	return &SignInReportsClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List returns a list of Sign-in Reports, optionally queried using OData.
func (c *SignInReportsClient) List(ctx context.Context, query odata.Query) (*[]SignInReport, int, error) {
	unknownError := func(resp *http.Response, o *odata.OData) bool {
		if resp != nil && resp.StatusCode == http.StatusBadRequest && o != nil && o.Error != nil {
			return o.Error.Match(odata.ErrorUnknownUnsupportedQuery)
		}
		return false
	}

	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: unknownError,
		DisablePaging:          query.Top > 0,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      "/auditLogs/signIns",
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("SignInLogsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		SignInLogs []SignInReport `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.SignInLogs, status, nil
}

// Get retrieves a Sign-in Report.
func (c *SignInReportsClient) Get(ctx context.Context, id string, query odata.Query) (*SignInReport, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/auditLogs/signIns/%s", id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("SignInLogsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var signInReport SignInReport
	if err := json.Unmarshal(respBody, &signInReport); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &signInReport, status, nil
}
