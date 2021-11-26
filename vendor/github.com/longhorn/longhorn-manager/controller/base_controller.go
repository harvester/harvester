package controller

import (
	"github.com/sirupsen/logrus"

	"k8s.io/client-go/util/workqueue"
)

type baseController struct {
	name   string
	logger logrus.FieldLogger
	queue  workqueue.RateLimitingInterface
}

func newBaseController(name string, logger logrus.FieldLogger) *baseController {
	return newBaseControllerWithQueue(name, logger,
		workqueue.NewNamedRateLimitingQueue(EnhancedDefaultControllerRateLimiter(), name))
}

func newBaseControllerWithQueue(name string, logger logrus.FieldLogger,
	queue workqueue.RateLimitingInterface) *baseController {
	c := &baseController{
		name:   name,
		logger: logger.WithField("controller", name),
		queue:  queue,
	}

	return c
}
