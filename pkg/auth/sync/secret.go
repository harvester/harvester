package sync

import (
	"errors"
	"fmt"
	"sync"

	dashboardsync "github.com/kubernetes/dashboard/src/app/backend/sync/api"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

func NewSecretSynchronizer(secrets ctlcorev1.SecretClient, namespace, name string) *SecretSynchronizer {
	return &SecretSynchronizer{
		secrets:   secrets,
		namespace: namespace,
		name:      name,
	}
}

// Implements kubernetes dashboard Synchronizer interface, the caller need to handle the panic
type SecretSynchronizer struct {
	secret    *v1.Secret
	secrets   ctlcorev1.SecretClient
	name      string
	namespace string

	mux sync.Mutex
}

func (s *SecretSynchronizer) Name() string {
	return fmt.Sprintf("%s-%s", s.name, s.namespace)
}

func (s *SecretSynchronizer) Create(obj runtime.Object) error {
	secret, err := s.getSecret(obj)
	if err != nil {
		return err
	}

	_, err = s.secrets.Create(secret)
	if err != nil {
		return err
	}

	return nil
}

func (s *SecretSynchronizer) Get() runtime.Object {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.secret == nil {
		// In case secret was not yet initialized try to do it synchronously
		secret, err := s.secrets.Get(s.namespace, s.name, metaV1.GetOptions{})
		if err != nil {
			return nil
		}

		logrus.Infof("Initializing secret synchronizer synchronously using secret %s from namespace %s", s.name, s.namespace)
		s.secret = secret
	}

	return s.secret
}

func (s *SecretSynchronizer) Update(obj runtime.Object) error {
	secret, err := s.getSecret(obj)
	if err != nil {
		return err
	}

	_, err = s.secrets.Update(secret)
	if err != nil {
		return err
	}

	return nil
}

func (s *SecretSynchronizer) Refresh() {
	s.mux.Lock()
	defer s.mux.Unlock()

	secret, err := s.secrets.Get(s.namespace, s.name, metaV1.GetOptions{})
	if err != nil {
		panic(err)
	}

	s.secret = secret
}

func (s *SecretSynchronizer) Delete() error {
	return s.secrets.Delete(s.namespace, s.name,
		&metaV1.DeleteOptions{GracePeriodSeconds: new(int64)})
}

func (s *SecretSynchronizer) getSecret(obj runtime.Object) (*v1.Secret, error) {
	secret, ok := obj.(*v1.Secret)
	if !ok {
		return nil, errors.New("provided object has to be a secret. Most likely this is a programming error")
	}

	secret.Name = s.name
	secret.Namespace = s.namespace
	return secret, nil
}

func (s *SecretSynchronizer) Start() {
	//not implement
}

func (s *SecretSynchronizer) Error() chan error {
	//not implement
	return nil
}

func (s *SecretSynchronizer) RegisterActionHandler(handler dashboardsync.ActionHandlerFunction, events ...watch.EventType) {
	//not implement
}

func (s *SecretSynchronizer) SetPoller(poller dashboardsync.Poller) {
	//not implement
}
