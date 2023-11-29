// Copyright © 2022 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secret

import (
	"context"
	"time"

	"emperror.dev/errors"

	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SecretGetter interface {
	Get(objectKey client.ObjectKey) (K8sSecret, error)
}

type K8sSecret interface {
	GetToken() []byte
	GetCACert() []byte
}

type readerSecretGetter struct {
	client client.Client
	// Backoff wait for reader secret to be created
	backoff *wait.Backoff
}

type ReaderSecretGetterOption = func(r *readerSecretGetter)

func WithBackOff(backoff *wait.Backoff) ReaderSecretGetterOption {
	return func(rsG *readerSecretGetter) {
		rsG.backoff = backoff
	}
}

var defaultBackoffWaitSecret = &wait.Backoff{
	Duration: time.Second * 3,
	Factor:   1,
	Jitter:   0,
	Steps:    3,
}

func NewReaderSecretGetter(client client.Client, opts ...ReaderSecretGetterOption) (SecretGetter, error) {
	rsGetter := &readerSecretGetter{client: client}

	if rsGetter.client == nil {
		return nil, errors.New("k8s client should be set for reader-secret getter")
	}

	for _, opt := range opts {
		opt(rsGetter)
	}

	if rsGetter.backoff == nil {
		rsGetter.backoff = defaultBackoffWaitSecret
	}

	return rsGetter, nil
}

type readerSecret struct {
	Token  []byte
	CACert []byte
}

func (r *readerSecret) GetToken() []byte {
	return r.Token
}

func (r *readerSecret) GetCACert() []byte {
	return r.CACert
}

func (r *readerSecretGetter) Get(objectKey client.ObjectKey) (K8sSecret, error) {
	ctx := context.Background()

	// fetch SA object so that we can get the Secret object that relates to the SA
	saObjectKey := objectKey
	sa, err := r.getReaderSecretServiceAccount(ctx, saObjectKey)
	if err != nil {
		return nil, errors.WrapIf(err, "error getting reader secret")
	}

	// After K8s v1.24, Secret objects containing ServiceAccount tokens are no longer auto-generated, so we will have to
	// manually create Secret in order to get the token.
	// Reference: https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.24.md#no-really-you-must-read-this-before-you-upgrade
	return r.getOrCreateReaderSecretWithServiceAccount(ctx, sa)
}

func (r *readerSecretGetter) getReaderSecretServiceAccount(ctx context.Context, saObjectKey client.ObjectKey) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{}
	err := r.client.Get(ctx, saObjectKey, sa)
	if err != nil {
		return nil, errors.WrapIff(err, "error getting service account object, service account name: %s, service account namespace: %s",
			saObjectKey.Name,
			saObjectKey.Namespace)
	}

	return sa, nil
}

func (r *readerSecretGetter) getOrCreateReaderSecretWithServiceAccount(ctx context.Context, sa *corev1.ServiceAccount) (K8sSecret, error) {
	secretObj := &corev1.Secret{}

	readerSecretName := sa.Name + "-token"
	if len(sa.Secrets) != 0 {
		readerSecretName = sa.Secrets[0].Name
	}
	secretObjRef := types.NamespacedName{
		Namespace: sa.Namespace,
		Name:      readerSecretName,
	}

	err := r.client.Get(ctx, secretObjRef, secretObj)
	if err != nil && k8sErrors.IsNotFound(err) {
		secretObj = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: secretObjRef.Namespace,
				Name:      secretObjRef.Name,
				Annotations: map[string]string{
					"kubernetes.io/service-account.name": sa.Name,
				},
			},
			Type: "kubernetes.io/service-account-token",
		}

		err = r.client.Create(ctx, secretObj)
		if err != nil {
			return nil, errors.WrapIfWithDetails(err, "creating kubernetes secret failed", "namespace",
				secretObjRef.Namespace,
				"secret name",
				secretObjRef.Name)
		}

		return r.waitAndGetReaderSecret(ctx, secretObjRef.Namespace, secretObjRef.Name)
	}

	readerSecret := &readerSecret{
		Token:  secretObj.Data["token"],
		CACert: secretObj.Data["ca.crt"],
	}

	return readerSecret, nil
}

func (r *readerSecretGetter) waitAndGetReaderSecret(ctx context.Context, secretNamespace string, secretName string) (K8sSecret, error) {
	var token, caCert []byte

	secretObjRef := types.NamespacedName{
		Namespace: secretNamespace,
		Name:      secretName,
	}

	backoffWaitForSecretCreation := *r.backoff
	err := wait.ExponentialBackoff(backoffWaitForSecretCreation, func() (bool, error) {
		tokenSecret := &corev1.Secret{}
		err := r.client.Get(ctx, secretObjRef, tokenSecret)
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				return false, nil
			}

			return false, err
		}

		token = tokenSecret.Data["token"]
		caCert = tokenSecret.Data["ca.crt"]

		if token == nil || caCert == nil {
			return false, nil
		}

		return true, nil
	})

	readerSecret := &readerSecret{
		Token:  token,
		CACert: caCert,
	}

	return readerSecret, errors.WrapIfWithDetails(err, "fail to wait for the token and CA cert to be generated",
		"secret namespace", secretObjRef.Namespace, "secret name", secretObjRef.Name)
}
