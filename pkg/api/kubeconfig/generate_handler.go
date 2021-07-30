package kubeconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	ctlrbacv1 "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
)

const defaultTimeout = time.Second * 3

type GenerateHandler struct {
	context                  context.Context
	clusterRole              *rbacv1.ClusterRole
	saClient                 ctlcorev1.ServiceAccountClient
	saCache                  ctlcorev1.ServiceAccountCache
	clusterRoleCache         ctlrbacv1.ClusterRoleCache
	clusterRoleBindingClient ctlrbacv1.ClusterRoleBindingClient
	clusterRoleBindingCache  ctlrbacv1.ClusterRoleBindingCache
	secretCache              ctlcorev1.SecretCache
	secretClient             ctlcorev1.SecretClient
}

func NewGenerateHandler(scaled *config.Scaled) *GenerateHandler {
	return &GenerateHandler{
		context:                  scaled.Ctx,
		saClient:                 scaled.CoreFactory.Core().V1().ServiceAccount(),
		saCache:                  scaled.CoreFactory.Core().V1().ServiceAccount().Cache(),
		clusterRoleCache:         scaled.RbacFactory.Rbac().V1().ClusterRole().Cache(),
		clusterRoleBindingClient: scaled.RbacFactory.Rbac().V1().ClusterRoleBinding(),
		clusterRoleBindingCache:  scaled.RbacFactory.Rbac().V1().ClusterRoleBinding().Cache(),
		secretCache:              scaled.CoreFactory.Core().V1().Secret().Cache(),
		secretClient:             scaled.CoreFactory.Core().V1().Secret(),
	}
}

type req struct {
	ServerUrl       string `json:"serverUrl"`
	ClusterRoleName string `json:"clusterRoleName"`
	Namespace       string `json:"namespace"`
	SaName          string `json:"serviceAccountName"`
}

func (h *GenerateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// TODO authentication
	var req req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to decode request"))
		return
	}
	if err := h.setClusterRole(req.ClusterRoleName); err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to set clusterRole"))
		return
	}

	sa, secret, err := h.ensureSaAndSecret(req.Namespace, req.SaName)
	if err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to create serviceAccount"))
		return
	}

	if _, err := h.createClusterRoleBindingIfNotExit(req.ClusterRoleName, sa); err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, errors.Wrap(err, "fail to create clusterRoleBinding"))
		return
	}

	kubeConfig, err := h.generateKubeConfig(secret, req.ServerUrl)
	if err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, errors.Wrap(err, "fail to generate kubeconfig"))
		return
	}

	util.ResponseOKWithBody(rw, kubeConfig)
}

func (h *GenerateHandler) setClusterRole(clusterRoleName string) error {
	var err error
	h.clusterRole, err = h.clusterRoleCache.Get(clusterRoleName)
	return err
}

func (h *GenerateHandler) createClusterRoleBindingIfNotExit(clusterRoleName string, sa *corev1.ServiceAccount) (*rbacv1.ClusterRoleBinding, error) {
	name := sa.Namespace + "-" + sa.Name
	clusterRoleBinding, err := h.clusterRoleBindingCache.Get(name)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		return clusterRoleBinding, nil
	}

	return h.clusterRoleBindingClient.Create(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Namespace: sa.Namespace,
				Name:      sa.Name,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
	})
}

// createSaIfNotExit returns the serviceAccount and the associated secret
func (h *GenerateHandler) ensureSaAndSecret(namespace, name string) (*corev1.ServiceAccount, *corev1.Secret, error) {
	ch := make(chan *corev1.ServiceAccount)
	errCh := make(chan error)

	go func() {
		if sa, err := h.saCache.Get(namespace, name); err != nil && !apierrors.IsNotFound(err) {
			errCh <- err
			return
		} else if err == nil {
			ch <- sa
			return
		}

		// We watch the serviceAccount because we have to wait for the associated secret be created
		w, err := h.saClient.Watch(namespace, metav1.ListOptions{FieldSelector: fmt.Sprintf("metadata.name=%s", name)})
		if err != nil {
			errCh <- err
			return
		}
		_, err = h.saClient.Create(&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			}})
		if err != nil {
			errCh <- err
			return
		}
		for event := range w.ResultChan() {
			if event.Type != watch.Modified {
				continue
			}
			sa, ok := event.Object.(*corev1.ServiceAccount)
			if !ok {
				errCh <- fmt.Errorf("fail to get secret")
				return
			}
			if len(sa.Secrets) == 0 {
				continue
			}
			ch <- sa
			return
		}
	}()

	select {
	case <-time.After(defaultTimeout):
		return nil, nil, fmt.Errorf("get serviceAccount and secret timeout")
	case sa := <-ch:
		secret, err := h.secretCache.Get(namespace, sa.Secrets[0].Name)
		if err != nil {
			return nil, nil, err
		}
		return sa, secret, nil
	case err := <-errCh:
		return nil, nil, err
	}
}

func (h *GenerateHandler) generateKubeConfig(secret *corev1.Secret, server string) (string, error) {
	cluster, user, context := "default", "default", "default"
	ca, token, err := h.getCaAndToken(secret)
	if err != nil {
		return "", err
	}
	config := clientcmdapi.NewConfig()
	config.Kind = "Config"
	config.APIVersion = clientcmdlatest.Version
	config.Clusters[cluster] = &clientcmdapi.Cluster{
		Server:                   server,
		CertificateAuthorityData: ca,
	}
	config.AuthInfos[user] = &clientcmdapi.AuthInfo{
		Token: string(token),
	}
	config.Contexts[context] = &clientcmdapi.Context{
		Cluster:   cluster,
		Namespace: secret.Namespace,
		AuthInfo:  user,
	}
	config.CurrentContext = context

	b := new(strings.Builder)
	if err := clientcmdlatest.Codec.Encode(config, b); err != nil {
		return "", nil
	}

	return b.String(), nil
}

func (h *GenerateHandler) getCaAndToken(secret *corev1.Secret) ([]byte, []byte, error) {
	ca, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, nil, fmt.Errorf("not find ca.crt in secret %s", secret.Name)
	}
	token, ok := secret.Data["token"]
	if !ok {
		return nil, nil, fmt.Errorf("not find token in secret %s", secret.Name)
	}

	return ca, token, nil
}
