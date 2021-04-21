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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/harvester/harvester/pkg/auth/jwe"
	"github.com/harvester/harvester/pkg/config"
)

const (
	privateKey = "priv"
	publicKey  = "pub"
)

func WatchSecret(ctx context.Context, scaled *config.Scaled, namespace, name string) {
	logrus.Infof("start watch secret %v", name)
	secrets := scaled.CoreFactory.Core().V1().Secret()
	opts := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", name),
	}

	var resyncPeriod = 30 * time.Minute

	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return secrets.List(namespace, opts)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return secrets.Watch(namespace, opts)
			},
		},
		&corev1.Secret{},
		resyncPeriod,
		cache.Indexers{},
	)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			if sec, ok := newObj.(*corev1.Secret); ok {
				logrus.Debugf("update auth secret in secret informer")
				if err := refreshKeyInTokenManager(sec, scaled); err != nil {
					logrus.Errorf("Failed to update tokenManager with secret %s:%s, %v", namespace, name, err)
				}
			}
		},
	})
	informer.Run(ctx.Done())
	logrus.Infof("finish watch secret %v", name)
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
