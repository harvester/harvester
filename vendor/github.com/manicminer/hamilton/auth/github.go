package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"

	"github.com/manicminer/hamilton/environments"
)

type GitHubOIDCConfig struct {
	// Environment is the national cloud environment to use
	Environment environments.Environment

	// TenantID is the required tenant ID for the primary token
	TenantID string

	// AuxiliaryTenantIDs is an optional list of tenant IDs for which to obtain additional tokens
	AuxiliaryTenantIDs []string

	// ClientID is the application's ID.
	ClientID string

	// IDTokenRequestURL is the URL for GitHub's OIDC provider.
	IDTokenRequestURL string

	// IDTokenRequestToken is the bearer token for the request to the OIDC provider.
	IDTokenRequestToken string

	// Scopes specifies a list of requested permission scopes (used for v2 tokens)
	Scopes []string

	// TokenURL is the clientCredentialsToken endpoint, which overrides the default endpoint constructed from a tenant ID
	TokenURL string

	// Audience optionally specifies the intended audience of the
	// request.  If empty, the value of TokenURL is used as the
	// intended audience.
	Audience string
}

func (c *GitHubOIDCConfig) TokenSource(ctx context.Context) Authorizer {
	return NewCachedAuthorizer(&GitHubOIDCAuthorizer{ctx, c})
}

type GitHubOIDCAuthorizer struct {
	ctx  context.Context
	conf *GitHubOIDCConfig
}

func (a *GitHubOIDCAuthorizer) githubAssertion() (*string, error) {
	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet, a.conf.IDTokenRequestURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("githubAssertion: failed to build request")
	}

	query, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("githubAssertion: cannot parse URL query")
	}

	if query.Get("audience") == "" {
		query.Set("audience", "api://AzureADTokenExchange")
		req.URL.RawQuery = query.Encode()
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.conf.IDTokenRequestToken))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("githubAssertion: cannot request token: %v", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("githubAssertion: cannot parse response: %v", err)
	}

	if c := resp.StatusCode; c < 200 || c > 299 {
		return nil, fmt.Errorf("githubAssertion: received HTTP status %d with response: %s", resp.StatusCode, body)
	}

	var tokenRes struct {
		Count *int    `json:"count"`
		Value *string `json:"value"`
	}
	if err := json.Unmarshal(body, &tokenRes); err != nil {
		return nil, fmt.Errorf("githubAssertion: cannot unmarshal response: %v", err)
	}

	return tokenRes.Value, nil
}

func (a *GitHubOIDCAuthorizer) tokenSource() (Authorizer, error) {
	assertion, err := a.githubAssertion()
	if err != nil {
		return nil, err
	}
	if assertion == nil {
		return nil, fmt.Errorf("GitHubOIDCAuthorizer: nil JWT assertion received from GitHub")
	}

	conf := ClientCredentialsConfig{
		Environment:        a.conf.Environment,
		TenantID:           a.conf.TenantID,
		AuxiliaryTenantIDs: a.conf.AuxiliaryTenantIDs,
		ClientID:           a.conf.ClientID,
		FederatedAssertion: *assertion,
		Scopes:             a.conf.Scopes,
		TokenURL:           a.conf.TokenURL,
		TokenVersion:       TokenVersion2,
		Audience:           a.conf.Audience,
	}

	source := conf.TokenSource(a.ctx, ClientCredentialsAssertionType)
	if source == nil {
		return nil, fmt.Errorf("GitHubOIDCAuthorizer: nil Authorizer returned from ClientCredentialsConfig")
	}

	return source, nil
}

func (a *GitHubOIDCAuthorizer) Token() (*oauth2.Token, error) {
	source, err := a.tokenSource()
	if err != nil {
		return nil, err
	}
	return source.Token()
}

func (a *GitHubOIDCAuthorizer) AuxiliaryTokens() ([]*oauth2.Token, error) {
	source, err := a.tokenSource()
	if err != nil {
		return nil, err
	}
	return source.AuxiliaryTokens()
}
