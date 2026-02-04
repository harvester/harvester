package kubernetes

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/dynamiclistener"
	"github.com/rancher/dynamiclistener/cert"
	"github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	v1controller "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/start"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

type CoreGetter func() *core.Factory

func Load(ctx context.Context, secrets v1controller.SecretController, namespace, name string, backing dynamiclistener.TLSStorage) dynamiclistener.TLSStorage {
	storage := &storage{
		name:      name,
		namespace: namespace,
		storage:   backing,
		ctx:       ctx,
		initSync:  &sync.Once{},
	}
	storage.init(secrets)
	return storage
}

func New(ctx context.Context, core CoreGetter, namespace, name string, backing dynamiclistener.TLSStorage) dynamiclistener.TLSStorage {
	storage := &storage{
		name:      name,
		namespace: namespace,
		storage:   backing,
		ctx:       ctx,
		initSync:  &sync.Once{},
	}

	// lazy init
	go func() {
		wait.PollImmediateUntilWithContext(ctx, time.Second, func(cxt context.Context) (bool, error) {
			if coreFactory := core(); coreFactory != nil {
				storage.init(coreFactory.Core().V1().Secret())
				return true, start.All(ctx, 5, coreFactory)
			}
			return false, nil
		})
	}()

	return storage
}

type storage struct {
	sync.RWMutex

	namespace, name string
	storage         dynamiclistener.TLSStorage
	secrets         v1controller.SecretController
	ctx             context.Context
	tls             dynamiclistener.TLSFactory
	initialized     bool
	initSync        *sync.Once
}

func (s *storage) SetFactory(tls dynamiclistener.TLSFactory) {
	s.Lock()
	defer s.Unlock()
	s.tls = tls
}

func (s *storage) init(secrets v1controller.SecretController) {
	s.Lock()
	defer s.Unlock()

	secrets.OnChange(s.ctx, "tls-storage", func(key string, secret *v1.Secret) (*v1.Secret, error) {
		if secret == nil {
			return nil, nil
		}
		if secret.Namespace == s.namespace && secret.Name == s.name {
			if err := s.Update(secret); err != nil {
				return nil, err
			}
		}

		return secret, nil
	})
	s.secrets = secrets

	// Asynchronously sync the backing storage to the Kubernetes secret, as doing so inline may
	// block the listener from accepting new connections if the apiserver becomes unavailable
	// after the Secrets controller has been initialized. We're not passing around any contexts
	// here, nor does the controller accept any, so there's no good way to soft-fail with a
	// reasonable timeout.
	go s.syncStorage()
}

func (s *storage) syncStorage() {
	var updateStorage bool
	secret, err := s.Get()
	if err == nil && cert.IsValidTLSSecret(secret) {
		// local storage had a cached secret, ensure that it exists in Kubernetes
		_, err := s.secrets.Create(&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        s.name,
				Namespace:   s.namespace,
				Annotations: secret.Annotations,
			},
			Type: v1.SecretTypeTLS,
			Data: secret.Data,
		})
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Warnf("Failed to create Kubernetes secret: %v", err)
		}
	} else {
		// local storage was empty, try to populate it
		secret, err = s.secrets.Get(s.namespace, s.name, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				logrus.Warnf("Failed to init Kubernetes secret: %v", err)
			}
		} else {
			updateStorage = true
		}
	}

	s.Lock()
	defer s.Unlock()
	s.initialized = true
	if updateStorage {
		if err := s.storage.Update(secret); err != nil {
			logrus.Warnf("Failed to init backing storage secret: %v", err)
		}
	}
}

func (s *storage) Get() (*v1.Secret, error) {
	s.RLock()
	defer s.RUnlock()

	return s.storage.Get()
}

func (s *storage) targetSecret() (*v1.Secret, error) {
	s.RLock()
	defer s.RUnlock()

	existingSecret, err := s.secrets.Get(s.namespace, s.name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.name,
				Namespace: s.namespace,
			},
			Type: v1.SecretTypeTLS,
		}, nil
	}
	return existingSecret, err
}

func (s *storage) saveInK8s(secret *v1.Secret) (*v1.Secret, error) {
	if !s.initComplete() {
		// Start a goroutine to attempt to save the secret later, once init is complete.
		// If this was already handled by initComplete, it should be a no-op, or at worst get
		// merged with the Kubernetes secret.
		go s.initSync.Do(func() {
			if err := wait.Poll(100*time.Millisecond, 15*time.Minute, func() (bool, error) {
				if !s.initComplete() {
					return false, nil
				}
				_, err := s.saveInK8s(secret)
				return true, err
			}); err != nil {
				logrus.Errorf("Failed to save TLS secret after controller init: %v", err)
			}
		})
		return secret, nil
	}

	targetSecret, err := s.targetSecret()
	if err != nil {
		return nil, err
	}

	// if we don't have a TLS factory we can't create certs, so don't bother trying to merge anything,
	// in favor of just blindly replacing the fields on the Kubernetes secret.
	if s.tls != nil {
		// merge new secret with secret from backing storage, if one exists
		if existing, err := s.Get(); err == nil && cert.IsValidTLSSecret(existing) {
			if newSecret, updated, err := s.tls.Merge(existing, secret); err == nil && updated {
				secret = newSecret
			}
		}

		// merge new secret with existing secret from Kubernetes, if one exists
		if cert.IsValidTLSSecret(targetSecret) {
			if newSecret, updated, err := s.tls.Merge(targetSecret, secret); err != nil {
				return nil, err
			} else if !updated {
				return newSecret, nil
			} else {
				secret = newSecret
			}
		}
	}

	// ensure that the merged secret actually contains data before overwriting the existing fields
	if !cert.IsValidTLSSecret(secret) {
		logrus.Warnf("Skipping save of TLS secret for %s/%s due to missing certificate data", secret.Namespace, secret.Name)
		return targetSecret, nil
	}

	targetSecret.Annotations = secret.Annotations
	targetSecret.Type = v1.SecretTypeTLS
	targetSecret.Data = secret.Data

	if targetSecret.UID == "" {
		logrus.Infof("Creating new TLS secret for %s/%s (count: %d): %v", targetSecret.Namespace, targetSecret.Name, len(targetSecret.Annotations)-1, targetSecret.Annotations)
		return s.secrets.Create(targetSecret)
	}
	logrus.Infof("Updating TLS secret for %s/%s (count: %d): %v", targetSecret.Namespace, targetSecret.Name, len(targetSecret.Annotations)-1, targetSecret.Annotations)
	return s.secrets.Update(targetSecret)
}

func (s *storage) Update(secret *v1.Secret) error {
	// Asynchronously update the Kubernetes secret, as doing so inline may block the listener from
	// accepting new connections if the apiserver becomes unavailable after the Secrets controller
	// has been initialized. We're not passing around any contexts here, nor does the controller
	// accept any, so there's no good way to soft-fail with a reasonable timeout.
	go func() {
		if err := s.update(secret); err != nil {
			logrus.Errorf("Failed to save TLS secret for %s/%s: %v", secret.Namespace, secret.Name, err)
		}
	}()
	return nil
}

func isConflictOrAlreadyExists(err error) bool {
	return errors.IsConflict(err) || errors.IsAlreadyExists(err)
}

func (s *storage) update(secret *v1.Secret) (err error) {
	var newSecret *v1.Secret
	err = retry.OnError(retry.DefaultRetry, isConflictOrAlreadyExists, func() error {
		newSecret, err = s.saveInK8s(secret)
		return err
	})

	if err != nil {
		return err
	}

	// Only hold the lock while updating underlying storage
	s.Lock()
	defer s.Unlock()
	return s.storage.Update(newSecret)
}

func (s *storage) initComplete() bool {
	s.RLock()
	defer s.RUnlock()
	return s.initialized
}
