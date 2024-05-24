package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/oauth2"
)

const (
	msiDefaultApiVersion = "2018-02-01"
	msiDefaultEndpoint   = "http://169.254.169.254/metadata/identity/oauth2/token"
	msiDefaultTimeout    = 10 * time.Second
)

// MsiAuthorizer is an Authorizer which supports managed service identity.
type MsiAuthorizer struct {
	ctx  context.Context
	conf *MsiConfig
}

// Token returns an access token acquired from the metadata endpoint.
func (a *MsiAuthorizer) Token() (*oauth2.Token, error) {
	if a.conf == nil {
		return nil, fmt.Errorf("could not request token: conf is nil")
	}

	query := url.Values{
		"api-version": []string{a.conf.MsiApiVersion},
		"resource":    []string{a.conf.Resource},
	}

	if a.conf.ClientID != "" {
		query["client_id"] = []string{a.conf.ClientID}
	}

	url := fmt.Sprintf("%s?%s", a.conf.MsiEndpoint, query.Encode())

	body, err := azureMetadata(a.ctx, url)
	if err != nil {
		return nil, fmt.Errorf("MsiAuthorizer: failed to request token from metadata endpoint: %v", err)
	}

	var tokenRes struct {
		AccessToken  string      `json:"access_token"`
		ClientID     string      `json:"client_id"`
		Resource     string      `json:"resource"`
		TokenType    string      `json:"token_type"`
		ExpiresIn    interface{} `json:"expires_in"`     // relative seconds from now
		ExpiresOn    interface{} `json:"expires_on"`     // timestamp
		ExtExpiresIn interface{} `json:"ext_expires_in"` // relative seconds from now
	}
	if err := json.Unmarshal(body, &tokenRes); err != nil {
		return nil, fmt.Errorf("MsiAuthorizer: failed to unmarshal token: %v", err)
	}

	token := &oauth2.Token{
		AccessToken: tokenRes.AccessToken,
		TokenType:   tokenRes.TokenType,
	}

	var secs time.Duration
	if exp, ok := tokenRes.ExpiresIn.(string); ok && exp != "" {
		if v, err := strconv.Atoi(exp); err == nil {
			secs = time.Duration(v)
		}
	} else if exp, ok := tokenRes.ExpiresIn.(int64); ok {
		secs = time.Duration(exp)
	} else if exp, ok := tokenRes.ExpiresIn.(float64); ok {
		secs = time.Duration(exp)
	}
	if secs > 0 {
		token.Expiry = time.Now().Add(secs * time.Second)
	}

	return token, nil
}

// AuxiliaryTokens returns additional tokens for auxiliary tenant IDs, for use in multi-tenant scenarios
func (a *MsiAuthorizer) AuxiliaryTokens() ([]*oauth2.Token, error) {
	return nil, fmt.Errorf("auxiliary tokens are not supported with MSI authentication")
}

// MsiConfig configures an MsiAuthorizer.
type MsiConfig struct {
	// ClientID is optionally used to determine which application to assume when a resource has multiple managed identities
	ClientID string

	// MsiApiVersion is the API version to use when requesting a token from the metadata service
	MsiApiVersion string

	// MsiEndpoint is the endpoint where the metadata service can be found
	MsiEndpoint string

	// Resource is the service for which to request an access token
	Resource string
}

// NewMsiConfig returns a new MsiConfig with a configured metadata endpoint and resource.
// clientId and objectId can be left blank when a single managed identity is available
func NewMsiConfig(resource, msiEndpoint, clientId string) (*MsiConfig, error) {
	endpoint := msiDefaultEndpoint
	if msiEndpoint != "" {
		endpoint = msiEndpoint
	}

	return &MsiConfig{
		ClientID:      clientId,
		Resource:      resource,
		MsiApiVersion: msiDefaultApiVersion,
		MsiEndpoint:   endpoint,
	}, nil
}

// TokenSource provides a source for obtaining access tokens using MsiAuthorizer.
func (c *MsiConfig) TokenSource(ctx context.Context) Authorizer {
	return NewCachedAuthorizer(&MsiAuthorizer{ctx: ctx, conf: c})
}

func azureMetadata(ctx context.Context, url string) (body []byte, err error) {
	var req *http.Request
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return
	}
	req.Header = http.Header{
		"Metadata": []string{"true"},
	}
	client := &http.Client{
		Timeout: msiDefaultTimeout,
	}
	var resp *http.Response
	resp, err = client.Do(req)
	if err != nil {
		return
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if c := resp.StatusCode; c < 200 || c > 299 {
		err = fmt.Errorf("received HTTP status %d with body: %s", resp.StatusCode, body)
		return
	}
	return
}
