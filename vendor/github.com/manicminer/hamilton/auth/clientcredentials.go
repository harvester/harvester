package auth

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/manicminer/hamilton/environments"

	"github.com/hashicorp/go-uuid"
	"golang.org/x/oauth2"
)

type ClientCredentialsType int

const (
	ClientCredentialsAssertionType ClientCredentialsType = iota
	ClientCredentialsSecretType
)

// ClientCredentialsConfig is the configuration for using client credentials flow.
//
// For more information see:
// https://docs.microsoft.com/en-us/azure/active-directory/develop/v2-oauth2-client-creds-grant-flow#get-a-token
// https://docs.microsoft.com/en-us/azure/active-directory/develop/active-directory-certificate-credentials
type ClientCredentialsConfig struct {
	// Environment is the national cloud environment to use
	Environment environments.Environment

	// TenantID is the required tenant ID for the primary token
	TenantID string

	// AuxiliaryTenantIDs is an optional list of tenant IDs for which to obtain additional tokens
	AuxiliaryTenantIDs []string

	// ClientID is the application's ID.
	ClientID string

	// ClientSecret is the application's secret.
	ClientSecret string

	// PrivateKey contains the contents of an RSA private key or the
	// contents of a PEM file that contains a private key. The provided
	// private key is used to sign JWT assertions.
	// PEM containers with a passphrase are not supported.
	// Use the following command to convert a PKCS 12 file into a PEM.
	//
	//    $ openssl pkcs12 -in key.p12 -out key.pem -nodes
	//
	PrivateKey []byte

	// Certificate contains the (optionally PEM encoded) X509 certificate registered
	// for the application with which you are authenticating. Used when FederatedAssertion is empty.
	Certificate []byte

	// FederatedAssertion contains a JWT provided by a trusted third-party vendor
	// for obtaining an access token with a federated credential. When empty, an
	// assertion will be created and signed using the specified PrivateKey and Certificate
	FederatedAssertion string

	// Resource specifies an API resource for which to request access (used for v1 tokens)
	Resource string

	// Scopes specifies a list of requested permission scopes (used for v2 tokens)
	Scopes []string

	// TokenURL is the clientCredentialsToken endpoint, which overrides the default endpoint constructed from a tenant ID
	TokenURL string

	// TokenVersion is the auth token version to acquire
	TokenVersion TokenVersion

	// Audience optionally specifies the intended audience of the
	// request.  If empty, the value of TokenURL is used as the
	// intended audience.
	Audience string
}

// TokenSource provides a source for obtaining access tokens using clientAssertionAuthorizer or clientSecretAuthorizer.
func (c *ClientCredentialsConfig) TokenSource(ctx context.Context, authType ClientCredentialsType) (source Authorizer) {
	switch authType {
	case ClientCredentialsAssertionType:
		source = NewCachedAuthorizer(&clientAssertionAuthorizer{ctx, c})
	case ClientCredentialsSecretType:
		source = NewCachedAuthorizer(&clientSecretAuthorizer{ctx, c})
	}
	return
}

type clientAssertionTokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
	KeyId     string `json:"kid"`
}

func (h *clientAssertionTokenHeader) encode() (string, error) {
	b, err := json.Marshal(h)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

type clientAssertionTokenClaims struct {
	Audience  string `json:"aud"`
	Expiry    int64  `json:"exp"`
	Issuer    string `json:"iss"`
	JwtId     string `json:"jti"`
	NotBefore int64  `json:"nbf"`
	Subject   string `json:"sub"`
}

func (c *clientAssertionTokenClaims) encode() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

type clientAssertionToken struct {
	header clientAssertionTokenHeader
	claims clientAssertionTokenClaims
}

func (c *clientAssertionToken) encode(key *rsa.PrivateKey) (string, error) {
	var err error

	c.claims.NotBefore = time.Now().Unix()
	c.claims.Expiry = time.Now().Add(time.Hour).Unix()
	c.claims.JwtId, err = uuid.GenerateUUID()
	if err != nil {
		return "", err
	}

	sign := func(data []byte) (sig []byte, err error) {
		h := sha256.New()
		_, err = h.Write(data)
		if err != nil {
			return
		}
		return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h.Sum(nil))
	}

	// encode the header
	hs, err := c.header.encode()
	if err != nil {
		return "", err
	}

	// encode the claims
	cs, err := c.claims.encode()
	if err != nil {
		return "", err
	}

	// sign the token
	ss := fmt.Sprintf("%s.%s", hs, cs)
	sig, err := sign([]byte(ss))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s.%s", ss, base64.RawURLEncoding.EncodeToString(sig)), nil
}

type clientAssertionAuthorizer struct {
	ctx  context.Context
	conf *ClientCredentialsConfig
}

func (a *clientAssertionAuthorizer) assertion(tokenUrl string) (*string, error) {
	crt := a.conf.Certificate
	if der, _ := pem.Decode(a.conf.Certificate); der != nil {
		crt = der.Bytes
	}

	cert, err := x509.ParseCertificate(crt)
	if err != nil {
		return nil, fmt.Errorf("clientAssertionAuthorizer: cannot parse certificate: %v", err)
	}

	keySig := sha1.Sum(cert.Raw)
	keyId := base64.URLEncoding.EncodeToString(keySig[:])

	privKey, err := parseKey(a.conf.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("clientAssertionAuthorizer: cannot parse private key: %v", err)
	}

	audience := a.conf.Audience
	if audience == "" {
		audience = tokenUrl
	}

	t := clientAssertionToken{
		header: clientAssertionTokenHeader{
			Algorithm: "RS256",
			Type:      "JWT",
			KeyId:     keyId,
		},
		claims: clientAssertionTokenClaims{
			Audience: audience,
			Issuer:   a.conf.ClientID,
			Subject:  a.conf.ClientID,
		},
	}
	assertion, err := t.encode(privKey)
	if err != nil {
		return nil, fmt.Errorf("clientAssertionAuthorizer: failed to encode and sign JWT assertion")
	}

	return &assertion, nil
}

func (a *clientAssertionAuthorizer) token(tokenUrl string) (*oauth2.Token, error) {
	assertion := a.conf.FederatedAssertion
	if assertion == "" {
		a, err := a.assertion(tokenUrl)
		if err != nil {
			return nil, err
		}
		if a == nil {
			return nil, fmt.Errorf("clientAssertionAuthorizer: assertion was nil")
		}
		assertion = *a
	}

	v := url.Values{
		"client_assertion":      {assertion},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_id":             {a.conf.ClientID},
		"grant_type":            {"client_credentials"},
	}

	if a.conf.TokenVersion == TokenVersion1 {
		v["resource"] = []string{a.conf.Resource}
	} else {
		v["scope"] = []string{strings.Join(a.conf.Scopes, " ")}
	}

	return clientCredentialsToken(a.ctx, tokenUrl, &v)
}

func (a *clientAssertionAuthorizer) Token() (*oauth2.Token, error) {
	if a.conf == nil {
		return nil, fmt.Errorf("could not request token: conf is nil")
	}

	tokenUrl := a.conf.TokenURL
	if tokenUrl == "" {
		tokenUrl = TokenEndpoint(a.conf.Environment.AzureADEndpoint, a.conf.TenantID, a.conf.TokenVersion)
	}

	return a.token(tokenUrl)
}

// AuxiliaryTokens returns additional tokens for auxiliary tenant IDs, for use in multi-tenant scenarios
func (a *clientAssertionAuthorizer) AuxiliaryTokens() ([]*oauth2.Token, error) {
	if a.conf == nil {
		return nil, fmt.Errorf("could not request token: conf is nil")
	}

	tokens := make([]*oauth2.Token, 0)

	if len(a.conf.AuxiliaryTenantIDs) == 0 {
		return tokens, nil
	}

	for _, tenantId := range a.conf.AuxiliaryTenantIDs {
		tokenUrl := a.conf.TokenURL
		if tokenUrl == "" {
			tokenUrl = TokenEndpoint(a.conf.Environment.AzureADEndpoint, tenantId, a.conf.TokenVersion)
		}

		token, err := a.token(tokenUrl)
		if err != nil {
			return tokens, err
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}

// parseKey returns an rsa.PrivateKey containing the provided binary key data.
// If the provided key is PEM encoded, it is decoded first.
func parseKey(key []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(key)
	if block != nil {
		key = block.Bytes
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		parsedKey, err = x509.ParsePKCS1PrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("private key should be a PEM or plain PKCS1 or PKCS8; parse error: %v", err)
		}
	}
	parsed, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is invalid")
	}
	return parsed, nil
}

type clientSecretAuthorizer struct {
	ctx  context.Context
	conf *ClientCredentialsConfig
}

func (a *clientSecretAuthorizer) Token() (*oauth2.Token, error) {
	if a.conf == nil {
		return nil, fmt.Errorf("could not request token: conf is nil")
	}

	v := url.Values{
		"client_id":     {a.conf.ClientID},
		"client_secret": {a.conf.ClientSecret},
		"grant_type":    {"client_credentials"},
	}

	if a.conf.TokenVersion == TokenVersion1 {
		v["resource"] = []string{a.conf.Resource}
	} else {
		v["scope"] = []string{strings.Join(a.conf.Scopes, " ")}
	}

	tokenUrl := a.conf.TokenURL
	if tokenUrl == "" {
		tokenUrl = TokenEndpoint(a.conf.Environment.AzureADEndpoint, a.conf.TenantID, a.conf.TokenVersion)
	}

	return clientCredentialsToken(a.ctx, tokenUrl, &v)
}

// AuxiliaryTokens returns additional tokens for auxiliary tenant IDs, for use in multi-tenant scenarios
func (a *clientSecretAuthorizer) AuxiliaryTokens() ([]*oauth2.Token, error) {
	if a.conf == nil {
		return nil, fmt.Errorf("could not request token: conf is nil")
	}

	tokens := make([]*oauth2.Token, 0)

	if len(a.conf.AuxiliaryTenantIDs) == 0 {
		return tokens, nil
	}

	for _, tenantId := range a.conf.AuxiliaryTenantIDs {
		v := url.Values{
			"client_id":     {a.conf.ClientID},
			"client_secret": {a.conf.ClientSecret},
			"grant_type":    {"client_credentials"},
		}

		if a.conf.TokenVersion == TokenVersion1 {
			v["resource"] = []string{a.conf.Resource}
		} else {
			v["scope"] = []string{strings.Join(a.conf.Scopes, " ")}
		}

		tokenUrl := a.conf.TokenURL
		if tokenUrl == "" {
			tokenUrl = TokenEndpoint(a.conf.Environment.AzureADEndpoint, tenantId, a.conf.TokenVersion)
		}

		token, err := clientCredentialsToken(a.ctx, tokenUrl, &v)
		if err != nil {
			return tokens, err
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}

func clientCredentialsToken(ctx context.Context, endpoint string, params *url.Values) (*oauth2.Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer([]byte(params.Encode())))
	if err != nil {
		return nil, fmt.Errorf("clientCredentialsToken: failed to build request")
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clientCredentialsToken: cannot request token: %v", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("clientCredentialsToken: cannot parse response: %v", err)
	}

	if c := resp.StatusCode; c < 200 || c > 299 {
		return nil, fmt.Errorf("clientCredentialsToken: received HTTP status %d with response: %s", resp.StatusCode, body)
	}

	// clientCredentialsToken response can arrive with numeric values as integers or strings :(
	var tokenRes struct {
		AccessToken string      `json:"access_token"`
		TokenType   string      `json:"token_type"`
		IDToken     string      `json:"id_token"`
		Resource    string      `json:"resource"`
		Scope       string      `json:"scope"`
		ExpiresIn   interface{} `json:"expires_in"` // relative seconds from now
		ExpiresOn   interface{} `json:"expires_on"` // timestamp
	}
	if err := json.Unmarshal(body, &tokenRes); err != nil {
		return nil, fmt.Errorf("clientCredentialsToken: cannot unmarshal response: %v", err)
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
