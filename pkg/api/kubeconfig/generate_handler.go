package kubeconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	ctlrbacv1 "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/master/rancher"
	"github.com/harvester/harvester/pkg/util"
)

const (
	defaultTimeout        = time.Second * 3
	defaultTickerInterval = time.Second

	port = "6443"
)

type GenerateHandler struct {
	context           context.Context
	saClient          ctlcorev1.ServiceAccountClient
	saCache           ctlcorev1.ServiceAccountCache
	clusterRoleCache  ctlrbacv1.ClusterRoleCache
	roleBindingClient ctlrbacv1.RoleBindingClient
	roleBindingCache  ctlrbacv1.RoleBindingCache
	secretCache       ctlcorev1.SecretCache
	secretClient      ctlcorev1.SecretClient
	configMapCache    ctlcorev1.ConfigMapCache
	namespace         string
}

func NewGenerateHandler(scaled *config.Scaled, option config.Options) *GenerateHandler {
	return &GenerateHandler{
		context:           scaled.Ctx,
		saClient:          scaled.CoreFactory.Core().V1().ServiceAccount(),
		saCache:           scaled.CoreFactory.Core().V1().ServiceAccount().Cache(),
		clusterRoleCache:  scaled.RbacFactory.Rbac().V1().ClusterRole().Cache(),
		roleBindingClient: scaled.RbacFactory.Rbac().V1().RoleBinding(),
		roleBindingCache:  scaled.RbacFactory.Rbac().V1().RoleBinding().Cache(),
		secretCache:       scaled.CoreFactory.Core().V1().Secret().Cache(),
		secretClient:      scaled.CoreFactory.Core().V1().Secret(),
		configMapCache:    scaled.CoreFactory.Core().V1().ConfigMap().Cache(),
		namespace:         option.Namespace,
	}
}

type req struct {
	ClusterRoleName string `json:"clusterRoleName"`
	Namespace       string `json:"namespace"`
	SaName          string `json:"serviceAccountName"`
}

func decodeRequest(r *http.Request) (*req, error) {
	if r == nil {
		return nil, fmt.Errorf("request can not be nil")
	}

	var req req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}

	if req.ClusterRoleName == "" || req.Namespace == "" || req.SaName == "" {
		return nil, fmt.Errorf("invalid request")
	}

	return &req, nil
}

func (h *GenerateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// TODO authentication
	req, err := decodeRequest(r)
	if err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to decode request"))
		return
	}

	if _, err := h.clusterRoleCache.Get(req.ClusterRoleName); err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrapf(err, "clusterRole %s is not found", req.ClusterRoleName))
		return
	}

	sa, secret, err := h.ensureSaAndSecret(req.Namespace, req.SaName)
	if err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to create serviceAccount"))
		return
	}

	if _, err := h.createRoleBindingIfNotExit(req.ClusterRoleName, sa); err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, errors.Wrap(err, "fail to create roleBinding"))
		return
	}

	serverURL, err := h.getServerURL()
	if err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, errors.Wrap(err, "failed to get server url"))
		return
	}

	kubeConfig, err := h.generateKubeConfig(secret, serverURL)
	if err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, errors.Wrap(err, "fail to generate kubeconfig"))
		return
	}

	util.ResponseOKWithBody(rw, kubeConfig)
}

func (h *GenerateHandler) getServerURL() (string, error) {
	vipCm, err := h.configMapCache.Get(h.namespace, rancher.VipConfigmapName)
	if err != nil {
		return "", err
	}

	vipConfig := rancher.VIPConfig{}
	if err := mapstructure.Decode(vipCm.Data, &vipConfig); err != nil {
		return "", err
	}

	vip := vipConfig.IP
	if ip := net.ParseIP(vip); ip == nil {
		return "", fmt.Errorf("invalid vip %s", vip)
	}

	return "https://" + vip + ":" + port, nil
}

func (h *GenerateHandler) createRoleBindingIfNotExit(clusterRoleName string, sa *corev1.ServiceAccount) (*rbacv1.RoleBinding, error) {
	namespace := sa.Namespace
	name := sa.Namespace + "-" + sa.Name
	roleBinding, err := h.roleBindingCache.Get(namespace, name)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		return roleBinding, nil
	}

	return h.roleBindingClient.Create(&rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       rbacv1.ServiceAccountKind,
					Name:       sa.Name,
					UID:        sa.UID,
				},
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Namespace: namespace,
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

// ensureSaAndSecret returns the serviceAccount and the associated secret
func (h *GenerateHandler) ensureSaAndSecret(namespace, name string) (*corev1.ServiceAccount, *corev1.Secret, error) {
	sa, err := h.saCache.Get(namespace, name)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, err
	}
	if apierrors.IsNotFound(err) {
		sa, err = h.saClient.Create(&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			}})
		if err != nil {
			return nil, nil, err
		}
	}

	secretName := sa.Name + "-token"
	secretNamespace := sa.Namespace
	secret, err := h.secretCache.Get(secretNamespace, secretName)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, err
	}
	if apierrors.IsNotFound(err) {
		secret, err = h.secretClient.Create(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: rbacv1.SchemeGroupVersion.Version,
						Kind:       rbacv1.ServiceAccountKind,
						Name:       sa.Name,
						UID:        sa.UID,
					},
				},
				Annotations: map[string]string{
					corev1.ServiceAccountNameKey: sa.Name,
					corev1.ServiceAccountUIDKey:  string(sa.UID),
				},
			},
			Type: corev1.SecretTypeServiceAccountToken,
		})
		if err != nil {
			return nil, nil, err
		}
	}
	timer := time.NewTimer(defaultTimeout)
	defer timer.Stop()

	ticker := time.NewTicker(defaultTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return nil, nil, fmt.Errorf("timeout while waiting for secret")
		case <-ticker.C:
			secret, err = h.secretCache.Get(secretNamespace, secretName)
			if err != nil {
				return nil, nil, err
			}
			_, caOk := secret.Data["ca.crt"]
			_, tokenOk := secret.Data["token"]
			if caOk && tokenOk {
				return sa, secret, nil
			}
		}
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
		return "", err
	}

	return b.String(), nil
}

func (h *GenerateHandler) getCaAndToken(secret *corev1.Secret) ([]byte, []byte, error) {
	ca, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, nil, fmt.Errorf("ca.crt is not found in secret %s", secret.Name)
	}
	token, ok := secret.Data["token"]
	if !ok {
		return nil, nil, fmt.Errorf("token is not found in secret %s", secret.Name)
	}

	return ca, token, nil
}
