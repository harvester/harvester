package memory

import (
	"github.com/rancher/dynamiclistener"
	"github.com/rancher/dynamiclistener/factory"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
)

func New() dynamiclistener.TLSStorage {
	return &memory{}
}

func NewBacked(storage dynamiclistener.TLSStorage) dynamiclistener.TLSStorage {
	return &memory{storage: storage}
}

type memory struct {
	storage dynamiclistener.TLSStorage
	secret  *v1.Secret
}

func (m *memory) Get() (*v1.Secret, error) {
	if m.secret == nil && m.storage != nil {
		secret, err := m.storage.Get()
		if err != nil {
			return nil, err
		}
		m.secret = secret
	}

	return m.secret, nil
}

func (m *memory) Update(secret *v1.Secret) error {
	if isChanged(m.secret, secret) {
		if m.storage != nil {
			if err := m.storage.Update(secret); err != nil {
				return err
			}
		}

		logrus.Infof("Active TLS secret %s/%s (ver=%s) (count %d): %v", secret.Namespace, secret.Name, secret.ResourceVersion, len(secret.Annotations)-1, secret.Annotations)
		m.secret = secret
	}
	return nil
}

func isChanged(old, new *v1.Secret) bool {
	if new == nil {
		return false
	}
	if old == nil {
		return true
	}
	if old.ResourceVersion == "" {
		return true
	}
	if old.ResourceVersion != new.ResourceVersion {
		return true
	}
	if old.Annotations[factory.Fingerprint] != new.Annotations[factory.Fingerprint] {
		return true
	}
	return false
}
