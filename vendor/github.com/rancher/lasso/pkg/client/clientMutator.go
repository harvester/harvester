package client

import (
	"github.com/rancher/lasso/pkg/log"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// Mutator is function that takes in a client and returns a new mutated client.
type Mutator func(*Client) (*Client, error)

type sharedClientFactoryWithMutation struct {
	SharedClientFactory
	mutator Mutator
}

// NewSharedClientFactoryWithAgent returns a sharedClientFactory that is equivalent to client factory with the addition
// that it will augment returned clients from ForKind, ForResource, and ForResourceKind with the passed user agent.
func NewSharedClientFactoryWithAgent(userAgent string, clientFactory SharedClientFactory) SharedClientFactory {
	agentMutator := func(c *Client) (*Client, error) {
		return c.WithAgent(userAgent)
	}
	return &sharedClientFactoryWithMutation{
		SharedClientFactory: clientFactory,
		mutator:             agentMutator,
	}
}

// NewSharedClientFactoryWithImpersonation returns a sharedClientFactory that is equivalent to client factory with the addition
// that it will augment returned clients from ForKind, ForResource, and ForResourceKind with the passed impersonation config.
func NewSharedClientFactoryWithImpersonation(impersonate rest.ImpersonationConfig, clientFactory SharedClientFactory) SharedClientFactory {
	impMutator := func(c *Client) (*Client, error) {
		return c.WithImpersonation(impersonate)
	}
	return &sharedClientFactoryWithMutation{
		SharedClientFactory: clientFactory,
		mutator:             impMutator,
	}
}

// ForKind returns a newly mutated client for the provided GroupVersionKind.
func (s *sharedClientFactoryWithMutation) ForKind(gvk schema.GroupVersionKind) (*Client, error) {
	client, err := s.SharedClientFactory.ForKind(gvk)
	if err != nil {
		return client, err
	}

	if client == nil {
		return client, nil
	}

	return s.mutator(client)
}

// ForResource returns a newly mutated client for the provided GroupVersionResource.
func (s *sharedClientFactoryWithMutation) ForResource(gvr schema.GroupVersionResource, namespaced bool) (*Client, error) {
	client, err := s.SharedClientFactory.ForResource(gvr, namespaced)
	if err != nil {
		return client, err
	}

	if client == nil {
		return client, nil
	}

	return s.mutator(client)
}

// ForResourceKind returns a newly mutated client for the provided GroupVersionResource and Kind.
func (s *sharedClientFactoryWithMutation) ForResourceKind(gvr schema.GroupVersionResource, kind string, namespaced bool) *Client {
	client := s.SharedClientFactory.ForResourceKind(gvr, kind, namespaced)

	if client == nil {
		return client
	}

	clientWithMutation, err := s.mutator(client)
	if err != nil {
		log.Debugf("%v", err)
	}
	return clientWithMutation
}
