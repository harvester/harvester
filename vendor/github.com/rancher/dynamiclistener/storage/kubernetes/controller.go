package kubernetes

import (
	"context"
	"maps"
	"time"

	"github.com/rancher/dynamiclistener"
	"github.com/rancher/dynamiclistener/cert"
	"github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	v1controller "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	toolswatch "k8s.io/client-go/tools/watch"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
)

type CoreGetter func() *core.Factory

type storage struct {
	namespace, name string
	storage         dynamiclistener.TLSStorage
	secrets         v1controller.SecretController
	tls             dynamiclistener.TLSFactory
	queue           workqueue.TypedInterface[string]
	queuedSecret    *v1.Secret
}

func Load(ctx context.Context, secrets v1controller.SecretController, namespace, name string, backing dynamiclistener.TLSStorage) dynamiclistener.TLSStorage {
	storage := &storage{
		name:      name,
		namespace: namespace,
		storage:   backing,
		queue:     workqueue.NewTyped[string](),
	}
	storage.runQueue()
	storage.init(ctx, secrets)
	return storage
}

func New(ctx context.Context, core CoreGetter, namespace, name string, backing dynamiclistener.TLSStorage) dynamiclistener.TLSStorage {
	storage := &storage{
		name:      name,
		namespace: namespace,
		storage:   backing,
		queue:     workqueue.NewTyped[string](),
	}
	storage.runQueue()

	// lazy init
	go func() {
		wait.PollImmediateUntilWithContext(ctx, time.Second, func(cxt context.Context) (bool, error) {
			if coreFactory := core(); coreFactory != nil {
				storage.init(ctx, coreFactory.Core().V1().Secret())
				return true, nil
			}
			return false, nil
		})
	}()

	return storage
}

// always return secret from backing storage
func (s *storage) Get() (*v1.Secret, error) {
	return s.storage.Get()
}

// sync secret to Kubernetes and backing storage via workqueue
func (s *storage) Update(secret *v1.Secret) error {
	// Asynchronously update the Kubernetes secret, as doing so inline may block the listener from
	// accepting new connections if the apiserver becomes unavailable after the Secrets controller
	// has been initialized.
	s.queuedSecret = secret
	s.queue.Add(s.name)
	return nil
}

func (s *storage) SetFactory(tls dynamiclistener.TLSFactory) {
	s.tls = tls
}

func (s *storage) init(ctx context.Context, secrets v1controller.SecretController) {
	s.secrets = secrets

	// Watch just the target secret, instead of using a wrangler OnChange handler
	// which watches all secrets in all namespaces. Changes to the secret
	// will be sent through the workqueue.
	go func() {
		fieldSelector := fields.Set{"metadata.name": s.name}.String()
		lw := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (object runtime.Object, e error) {
				options.FieldSelector = fieldSelector
				return secrets.List(s.namespace, options)
			},
			WatchFunc: func(options metav1.ListOptions) (i watch.Interface, e error) {
				options.FieldSelector = fieldSelector
				return secrets.Watch(s.namespace, options)
			},
		}
		_, _, watch, done := toolswatch.NewIndexerInformerWatcher(lw, &v1.Secret{})

		defer func() {
			s.queue.ShutDown()
			watch.Stop()
			<-done
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-watch.ResultChan():
				if secret, ok := ev.Object.(*v1.Secret); ok {
					s.queuedSecret = secret
					s.queue.Add(secret.Name)
				}
			}
		}
	}()

	// enqueue initial sync of the backing secret
	s.queuedSecret, _ = s.Get()
	s.queue.Add(s.name)
}

// runQueue starts a goroutine to process secrets updates from the workqueue
func (s *storage) runQueue() {
	go func() {
		for s.processQueue() {
		}
	}()
}

// processQueue processes the secret update queue.
// The key doesn't actually matter, as we are only handling a single secret with a single worker.
func (s *storage) processQueue() bool {
	key, shutdown := s.queue.Get()
	if shutdown {
		return false
	}

	defer s.queue.Done(key)
	if err := s.update(); err != nil {
		logrus.Errorf("Failed to update Secret %s/%s: %v", s.namespace, s.name, err)
	}

	return true
}

func (s *storage) targetSecret() (*v1.Secret, error) {
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

// saveInK8s handles merging the provided secret with the kubernetes secret.
// This includes calling the tls factory to sign a new certificate with the
// merged SAN entries, if possible.  Note that the provided secret could be
// either from Kubernetes due to the secret being changed by another client, or
// from the listener trying to add SANs or regenerate the cert.
func (s *storage) saveInK8s(secret *v1.Secret) (*v1.Secret, error) {
	// secret controller not initialized yet, just return the current secret.
	// if there is an existing secret in Kubernetes, that will get synced by the
	// list/watch once the controller is initialized.
	if s.secrets == nil {
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
		logrus.Warnf("Skipping save of TLS secret for %s/%s due to missing certificate data", s.namespace, s.name)
		return targetSecret, nil
	}

	// Any changes to the cert will change the fingerprint annotation, so we can use that
	// for change detection, and skip updating an existing secret if it has not changed.
	changed := !maps.Equal(targetSecret.Annotations, secret.Annotations)

	targetSecret.Type = v1.SecretTypeTLS
	targetSecret.Annotations = secret.Annotations
	targetSecret.Data = secret.Data

	if targetSecret.UID == "" {
		logrus.Infof("Creating new TLS secret for %s/%s (count: %d): %v", targetSecret.Namespace, targetSecret.Name, len(targetSecret.Annotations)-1, targetSecret.Annotations)
		return s.secrets.Create(targetSecret)
	} else if changed {
		logrus.Infof("Updating TLS secret for %s/%s (count: %d): %v", targetSecret.Namespace, targetSecret.Name, len(targetSecret.Annotations)-1, targetSecret.Annotations)
		return s.secrets.Update(targetSecret)
	}
	return targetSecret, nil
}

func isConflictOrAlreadyExists(err error) bool {
	return errors.IsConflict(err) || errors.IsAlreadyExists(err)
}

// update wraps a conflict retry around saveInK8s, which handles merging the
// queued secret with the Kubernetes secret.  Only after successfully
// updating the Kubernetes secret will the backing storage be updated.
func (s *storage) update() (err error) {
	var newSecret *v1.Secret
	if err := retry.OnError(retry.DefaultRetry, isConflictOrAlreadyExists, func() error {
		newSecret, err = s.saveInK8s(s.queuedSecret)
		return err
	}); err != nil {
		return err
	}
	return s.storage.Update(newSecret)
}
