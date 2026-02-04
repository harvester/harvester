// Copyright © 2020 Banzai Cloud
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
	"fmt"

	"emperror.dev/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SecretLoader interface {
	Load(secret *Secret) (string, error)
}

func NewSecretLoader(client client.Reader, namespace, mountPath string, secrets *MountSecrets) SecretLoader {
	return &secretLoader{
		client:    client,
		mountPath: mountPath,
		namespace: namespace,
		secrets:   secrets,
	}
}

type MountSecrets []MountSecret

func (m *MountSecrets) Append(namespace string, secret *corev1.SecretKeySelector, mappedKey string, value []byte) {
	*m = append(*m, MountSecret{
		Name:      secret.Name,
		Key:       secret.Key,
		MappedKey: mappedKey,
		Namespace: namespace,
		Value:     value,
	})
}

type MountSecret struct {
	Namespace string
	Name      string
	Key       string
	MappedKey string
	Value     []byte
}

type secretLoader struct {
	// secretLoader is limited to a single namespace, to avoid hijacking other namespace's secrets
	namespace string
	mountPath string
	client    client.Reader
	secrets   *MountSecrets
}

func (k *secretLoader) Load(secret *Secret) (string, error) {
	if secret.Value != "" {
		return secret.Value, nil
	}

	if secret.MountFrom != nil && secret.MountFrom.SecretKeyRef != nil {
		mappedKey := fmt.Sprintf("%s-%s-%s", k.namespace, secret.MountFrom.SecretKeyRef.Name, secret.MountFrom.SecretKeyRef.Key)
		secretItem := &corev1.Secret{}
		err := k.client.Get(context.TODO(), types.NamespacedName{
			Name:      secret.MountFrom.SecretKeyRef.Name,
			Namespace: k.namespace}, secretItem)
		if err != nil {
			return "", errors.WrapIfWithDetails(
				err, "failed to load secret", "secret", secret.MountFrom.SecretKeyRef.Name, "namespace", k.namespace)
		}
		k.secrets.Append(k.namespace, secret.MountFrom.SecretKeyRef, mappedKey, secretItem.Data[secret.MountFrom.SecretKeyRef.Key])
		return k.mountPath + "/" + mappedKey, nil
	}

	if secret.ValueFrom != nil && secret.ValueFrom.SecretKeyRef != nil {
		k8sSecret := &corev1.Secret{}
		err := k.client.Get(context.TODO(), types.NamespacedName{
			Name:      secret.ValueFrom.SecretKeyRef.Name,
			Namespace: k.namespace}, k8sSecret)
		if err != nil {
			return "", errors.WrapIff(err, "failed to get kubernetes secret %s:%s",
				k.namespace,
				secret.ValueFrom.SecretKeyRef.Name)
		}
		value, ok := k8sSecret.Data[secret.ValueFrom.SecretKeyRef.Key]
		if !ok {
			return "", errors.Errorf("key %q not found in secret %q in namespace %q",
				secret.ValueFrom.SecretKeyRef.Key,
				secret.ValueFrom.SecretKeyRef.Name,
				k.namespace)
		}
		return string(value), nil
	}

	return "", errors.New("No secret Value or ValueFrom defined for field")
}
