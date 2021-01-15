package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	k8sdashboardjwe "github.com/kubernetes/dashboard/src/app/backend/auth/jwe"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/square/go-jose.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/rancher/harvester/pkg/auth/jwe"
	"github.com/rancher/harvester/pkg/config"
)

const (
	privateKey = "priv"
	publicKey  = "pub"
)

func WatchSecret(ctx context.Context, scaled *config.Scaled, namespace, name string) {
	secrets := scaled.CoreFactory.Core().V1().Secret()
	opts := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", name),
	}

OUTER:
	for {
		logrus.Debugf("Starting auth secret watcher of %s:%s", namespace, name)
		watcher, err := secrets.Watch(namespace, opts)
		if err != nil {
			logrus.Fatalf("Failed to watch secret %s:%s, %v", namespace, name, err)
		}

		for {
			select {
			case watchEvent, ok := <-watcher.ResultChan():
				if !ok {
					// server will timeout the watch connection after 30-60 minutes, re-initialize the secret watcher
					watcher.Stop()
					logrus.Debugln("Close secret watcher after default timeout")
					continue OUTER
				}
				switch watchEvent.Type {
				case watch.Modified:
					sec, ok := watchEvent.Object.(*corev1.Secret)
					if !ok {
						logrus.Errorf("Failed to convert obj to secret")
						continue
					}

					if err := refreshKeyInTokenManager(sec, scaled); err != nil {
						logrus.Errorf("Failed to update tokenManager with secret %s:%s, %v", namespace, name, err)
						continue
					}
					logrus.Infoln("Refresh auth secret token manager done")
				case watch.Added:
					// ignore add case here
				case watch.Deleted:
					// ignore delete case here
				case watch.Bookmark:
					// A `Bookmark` means watch has synced here, just update the resourceVersion
				default:
					logrus.Errorf("Unable to understand watch event %#v", watchEvent)
				}
			case <-ctx.Done():
				break OUTER
			}
		}
	}
}

func refreshKeyInTokenManager(sec *corev1.Secret, scaled *config.Scaled) (err error) {
	//handle panic from calling kubernetes dashboard tokenManager.Decrypt
	defer func() {
		if recoveryMessage := recover(); recoveryMessage != nil {
			err = fmt.Errorf("%v", recoveryMessage)
		}
	}()

	priv, err := k8sdashboardjwe.ParseRSAKey(string(sec.Data[privateKey]), string(sec.Data[publicKey]))
	if err != nil {
		return errors.Wrapf(err, "Failed to parse rsa key from secret %s/%s", sec.Namespace, sec.Name)
	}

	encrypter, err := jose.NewEncrypter(jose.A256GCM, jose.Recipient{Algorithm: jose.RSA_OAEP_256, Key: &priv.PublicKey}, nil)
	if err != nil {
		return errors.Wrap(err, "Failed to create jose encrypter")
	}

	add, err := getAdd()
	if err != nil {
		return err
	}

	jwtEncryption, err := encrypter.EncryptWithAuthData([]byte(`{}`), add)
	if err != nil {
		return errors.Wrapf(err, "Failed to encrypt with key from secret %s/%s", sec.Namespace, sec.Name)
	}

	//TokenManager will refresh the key if decrypt failed
	_, err = scaled.TokenManager.Decrypt(jwtEncryption.FullSerialize())
	if err != nil {
		return errors.Wrapf(err, "Failed to decrypt generated token with key from secret %s/%s", sec.Namespace, sec.Name)
	}
	return
}

func getAdd() ([]byte, error) {
	now := time.Now()
	claim := map[string]string{
		"iat": now.Format(time.RFC3339),
		"exp": now.Add(jwe.GetTokenMaxTTL()).Format(time.RFC3339),
	}
	add, err := json.Marshal(claim)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshal jwe claim")
	}
	return add, nil
}
