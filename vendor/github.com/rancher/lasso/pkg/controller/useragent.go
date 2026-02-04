package controller

import (
	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/log"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type sharedControllerFactoryWithAgent struct {
	SharedControllerFactory

	userAgent string
}

type sharedControllerWithAgent struct {
	SharedController

	userAgent string
}

// NewSharedControllerFactoryWithAgent returns a controller factory that is equivalent to the passed controller factory
// with the addition that it will wrap the returned controllers from ForKind, ForResource, and ForResourceKind with the
// sharedController struct using the passed user agent.
func NewSharedControllerFactoryWithAgent(userAgent string, clientFactory SharedControllerFactory) SharedControllerFactory {
	return &sharedControllerFactoryWithAgent{
		SharedControllerFactory: clientFactory,
		userAgent:               userAgent,
	}
}

func NewSharedControllerWithAgent(userAgent string, controller SharedController) SharedController {
	return &sharedControllerWithAgent{
		SharedController: controller,
		userAgent:        userAgent,
	}
}

func (s *sharedControllerFactoryWithAgent) ForKind(gvk schema.GroupVersionKind) (SharedController, error) {
	resourceController, err := s.SharedControllerFactory.ForKind(gvk)
	if err != nil {
		return resourceController, err
	}

	return NewSharedControllerWithAgent(s.userAgent, resourceController), err
}

func (s *sharedControllerFactoryWithAgent) ForResource(gvr schema.GroupVersionResource, namespaced bool) SharedController {
	resourceController := s.SharedControllerFactory.ForResource(gvr, namespaced)
	return NewSharedControllerWithAgent(s.userAgent, resourceController)
}

func (s *sharedControllerFactoryWithAgent) ForResourceKind(gvr schema.GroupVersionResource, kind string, namespaced bool) SharedController {
	resourceController := s.SharedControllerFactory.ForResourceKind(gvr, kind, namespaced)
	return NewSharedControllerWithAgent(s.userAgent, resourceController)
}

func (s *sharedControllerWithAgent) Client() *client.Client {
	client := s.SharedController.Client()
	if client == nil {
		return client
	}
	clientWithAgent, err := client.WithAgent(s.userAgent)
	if err != nil {
		log.Debugf("failed to get client with agent [%s]", s.userAgent)
		return client
	}
	return clientWithAgent
}
