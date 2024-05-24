package auth

import (
	"sync"

	"golang.org/x/oauth2"
)

// CachedAuthorizer caches a token until it expires, then acquires a new token from Source
type CachedAuthorizer struct {
	// Source contains the underlying Authorizer for obtaining tokens
	Source Authorizer

	mutex     sync.RWMutex
	token     *oauth2.Token
	auxTokens []*oauth2.Token
}

// Token returns the current token if it's still valid, else will acquire a new token
func (c *CachedAuthorizer) Token() (*oauth2.Token, error) {
	c.mutex.RLock()
	valid := c.token != nil && c.token.Valid()
	c.mutex.RUnlock()

	if !valid {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		var err error
		c.token, err = c.Source.Token()
		if err != nil {
			return nil, err
		}
	}

	return c.token, nil
}

// AuxiliaryTokens returns additional tokens for auxiliary tenant IDs, for use in multi-tenant scenarios
func (c *CachedAuthorizer) AuxiliaryTokens() ([]*oauth2.Token, error) {
	c.mutex.RLock()
	var valid bool
	for _, token := range c.auxTokens {
		valid = token != nil && token.Valid()
		if !valid {
			break
		}
	}
	c.mutex.RUnlock()

	if !valid {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		var err error
		c.auxTokens, err = c.Source.AuxiliaryTokens()
		if err != nil {
			return nil, err
		}
	}

	return c.auxTokens, nil
}

// NewCachedAuthorizer returns an Authorizer that caches an access token for the duration of its validity.
// If the cached token expires, a new one is acquired and cached.
func NewCachedAuthorizer(src Authorizer) Authorizer {
	return &CachedAuthorizer{
		Source: src,
	}
}
