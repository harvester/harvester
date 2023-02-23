package client

import (
	"fmt"

	"github.com/rancher/lasso/pkg/log"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type sharedClientFactoryWithAgent struct {
	SharedClientFactory

	userAgent string
}

// NewSharedClientFactoryWithAgent returns a sharedClientFactory that is equivalent to client factory with the addition
// that it will augment returned clients from ForKind, ForResource, and ForResourceKind with the passed user agent.
func NewSharedClientFactoryWithAgent(userAgent string, clientFactory SharedClientFactory) SharedClientFactory {
	return &sharedClientFactoryWithAgent{
		SharedClientFactory: clientFactory,
		userAgent:           userAgent,
	}
}

func (s *sharedClientFactoryWithAgent) ForKind(gvk schema.GroupVersionKind) (*Client, error) {
	client, err := s.SharedClientFactory.ForKind(gvk)
	if err != nil {
		return client, err
	}

	if client == nil {
		return client, nil
	}

	return client.WithAgent(s.userAgent)
}

func (s *sharedClientFactoryWithAgent) ForResource(gvr schema.GroupVersionResource, namespaced bool) (*Client, error) {
	client, err := s.SharedClientFactory.ForResource(gvr, namespaced)
	if err != nil {
		return client, err
	}

	if client == nil {
		return client, nil
	}

	return client.WithAgent(s.userAgent)
}

func (s *sharedClientFactoryWithAgent) ForResourceKind(gvr schema.GroupVersionResource, kind string, namespaced bool) *Client {
	client := s.SharedClientFactory.ForResourceKind(gvr, kind, namespaced)

	if client == nil {
		return client
	}

	clientWithAgent, err := client.WithAgent(s.userAgent)
	if err != nil {
		log.Debugf(fmt.Sprintf("Failed to return client with agent [%s]: %v", s.userAgent, err))
	}
	return clientWithAgent
}
