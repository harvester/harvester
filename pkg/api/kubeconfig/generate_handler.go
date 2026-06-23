package kubeconfig

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlrbacv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/v3/pkg/name"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/master/rancher"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
)

const (
	defaultTimeout        = time.Second * 3
	defaultTickerInterval = time.Second

	port = "6443"

	csiRole = "harvesterhci.io:csi-driver"
	hcpRole = "harvesterhci.io:cloudprovider"

	outputFormatYaml = "yaml"

	// cloud-provider cloud-config write-file path
	legacyPath = "/etc/kubernetes/cloud-config"
	newPath    = "/var/lib/rancher/rke2/etc/config-files/cloud-provider-config"
)

type GenerateHandler struct {
	context                  context.Context
	saClient                 ctlcorev1.ServiceAccountClient
	saCache                  ctlcorev1.ServiceAccountCache
	clusterRoleCache         ctlrbacv1.ClusterRoleCache
	clusterRoleBindingClient ctlrbacv1.ClusterRoleBindingClient
	clusterRoleBindingCache  ctlrbacv1.ClusterRoleBindingCache
	roleBindingClient        ctlrbacv1.RoleBindingClient
	roleBindingCache         ctlrbacv1.RoleBindingCache
	secretCache              ctlcorev1.SecretCache
	secretClient             ctlcorev1.SecretClient
	configMapCache           ctlcorev1.ConfigMapCache
	namespace                string
}

func NewGenerateHandler(scaled *config.Scaled, option config.Options) *GenerateHandler {
	return &GenerateHandler{
		context:                  scaled.Ctx,
		saClient:                 scaled.CoreFactory.Core().V1().ServiceAccount(),
		saCache:                  scaled.CoreFactory.Core().V1().ServiceAccount().Cache(),
		clusterRoleCache:         scaled.RbacFactory.Rbac().V1().ClusterRole().Cache(),
		clusterRoleBindingClient: scaled.RbacFactory.Rbac().V1().ClusterRoleBinding(),
		clusterRoleBindingCache:  scaled.RbacFactory.Rbac().V1().ClusterRoleBinding().Cache(),
		roleBindingClient:        scaled.RbacFactory.Rbac().V1().RoleBinding(),
		roleBindingCache:         scaled.RbacFactory.Rbac().V1().RoleBinding().Cache(),
		secretCache:              scaled.CoreFactory.Core().V1().Secret().Cache(),
		secretClient:             scaled.CoreFactory.Core().V1().Secret(),
		configMapCache:           scaled.CoreFactory.Core().V1().ConfigMap().Cache(),
		namespace:                option.Namespace,
	}
}

type req struct {
	CSIClusterRoleName string `json:"csiclusterRoleName"`
	ClusterRoleName    string `json:"clusterRoleName"`
	Namespace          string `json:"namespace"`
	SaName             string `json:"serviceAccountName"`
	OutputFormat       string `json:"outputFormat"`
}

func decodeRequest(r *http.Request) (*req, error) {
	if r == nil {
		return nil, fmt.Errorf("request can not be nil")
	}

	var req req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}

	// only hcpRole is supported
	if req.ClusterRoleName == "" {
		req.ClusterRoleName = hcpRole
	} else if req.ClusterRoleName != hcpRole {
		return nil, fmt.Errorf("invalid request: clusterRoleName can only be %s", hcpRole)
	}

	if req.Namespace == "" || req.SaName == "" {
		return nil, fmt.Errorf("invalid request: namespace and serviceAccountName cannot be empty")
	}

	return &req, nil
}

func (h *GenerateHandler) Do(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	req := ctx.Req()
	if req.Method == http.MethodGet {
		return h.doGet(ctx)
	} else if req.Method == http.MethodPost {
		return h.doPost(ctx)
	}

	return nil, apierror.NewAPIError(validation.InvalidAction, fmt.Sprintf("Unsupported method %s", req.Method))
}

func (h *GenerateHandler) doPost(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	r := ctx.Req()

	req, err := decodeRequest(r)
	if err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, errors.Wrap(err, "fail to decode request").Error())
	}

	if _, err := h.clusterRoleCache.Get(req.ClusterRoleName); err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, errors.Wrapf(err, "clusterRole %s is not found", req.ClusterRoleName).Error())
	}

	serverURL, err := h.getServerURL()
	if err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "failed to get server url").Error())
	}

	sa, secret, err := h.ensureSaAndSecret(req.Namespace, req.SaName)
	if err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, errors.Wrap(err, "fail to create serviceAccount").Error())
	}

	if _, err := h.createRoleBindingIfNotExists(sa); err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "fail to create roleBinding").Error())
	}

	if _, err := h.createClusterRoleBindingIfNotExists(sa); err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "fail to create clusterRoleBinding").Error())
	}

	kubeConfig, err := h.generateKubeConfig(secret, serverURL, req.OutputFormat)
	if err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "fail to generate kubeconfig").Error())
	}

	logrus.Infof("kubeconfig for service account %s/%s is created", req.Namespace, req.SaName)

	return h.output(ctx, kubeConfig, req.OutputFormat)
}

func (h *GenerateHandler) output(ctx *harvesterServer.Ctx, kubeConfig, outputFormat string) (harvesterServer.ResponseBody, error) {
	if outputFormat == outputFormatYaml {
		//  cloud-init yaml format
		return h.writeYamlOutput(ctx, kubeConfig)
	}

	// legacy (empty) or unknown format
	ctx.SetStatusOK()
	return kubeConfig, nil
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

	return "https://" + net.JoinHostPort(vip, port), nil
}

func (h *GenerateHandler) createRoleBindingIfNotExists(sa *corev1.ServiceAccount) (*rbacv1.RoleBinding, error) {
	namespace := sa.Namespace
	rbName := name.SafeConcatName(sa.Namespace, sa.Name)

	roleBinding, err := h.roleBindingCache.Get(namespace, rbName)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		return roleBinding, nil
	}

	return h.roleBindingClient.Create(&rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      rbName,
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
			Name:     hcpRole,
		},
	})
}

func (h *GenerateHandler) createClusterRoleBindingIfNotExists(sa *corev1.ServiceAccount) (*rbacv1.ClusterRoleBinding, error) {
	namespace := sa.Namespace
	crbName := name.SafeConcatName(sa.Namespace, sa.Name)
	clusterRoleBinding, err := h.clusterRoleBindingCache.Get(crbName)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		return clusterRoleBinding, nil
	}

	return h.clusterRoleBindingClient.Create(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: crbName,
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
			Name:     csiRole,
		},
	})
}

// ensureSaAndSecret returns the serviceAccount and the associated secret
func (h *GenerateHandler) ensureSaAndSecret(namespace, saName string) (*corev1.ServiceAccount, *corev1.Secret, error) {
	sa, err := h.saCache.Get(namespace, saName)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, err
	}
	if apierrors.IsNotFound(err) {
		sa, err = h.saClient.Create(&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      saName,
			}})
		if err != nil {
			return nil, nil, err
		}
	}

	secretName := name.SafeConcatName(sa.Name, "token")
	secretNamespace := sa.Namespace

	_, err = h.secretCache.Get(secretNamespace, secretName)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, err
	}
	if apierrors.IsNotFound(err) {
		_, err = h.secretClient.Create(&corev1.Secret{
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
			secret, err := h.secretCache.Get(secretNamespace, secretName)
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

func (h *GenerateHandler) generateKubeConfig(secret *corev1.Secret, server string, outputFormat string) (string, error) {
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

func decodeGetRequest(r *http.Request) (*req, error) {
	if r == nil {
		return nil, fmt.Errorf("request can not be nil")
	}

	req := &req{
		Namespace:       r.URL.Query().Get("namespace"),
		SaName:          r.URL.Query().Get("serviceAccountName"),
		OutputFormat:    r.URL.Query().Get("outputFormat"),
		ClusterRoleName: hcpRole,
	}
	if req.Namespace == "" || req.SaName == "" {
		return nil, fmt.Errorf("invalid request: namespace and serviceAccountName cannot be empty")
	}

	return req, nil
}

func (h *GenerateHandler) doGet(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	r := ctx.Req()

	req, err := decodeGetRequest(r)
	if err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, errors.Wrap(err, "fail to decode request").Error())
	}

	if _, err := h.clusterRoleCache.Get(req.ClusterRoleName); err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, errors.Wrapf(err, "clusterRole %s is not found", req.ClusterRoleName).Error())
	}

	serverURL, err := h.getServerURL()
	if err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "failed to get server url").Error())
	}

	sa, secret, err := h.getSaAndSecret(req.Namespace, req.SaName)
	if err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, errors.Wrap(err, "fail to get serviceAccount").Error())
	}

	if _, err := h.getRoleBinding(sa); err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "fail to get roleBinding").Error())
	}

	if _, err := h.getClusterRoleBinding(sa); err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "fail to get clusterRoleBinding").Error())
	}

	kubeConfig, err := h.generateKubeConfig(secret, serverURL, req.OutputFormat)
	if err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "fail to generate kubeconfig").Error())
	}

	logrus.Infof("kubeconfig for service account %s/%s is fetched", req.Namespace, req.SaName)

	return h.output(ctx, kubeConfig, req.OutputFormat)
}

func (h *GenerateHandler) writeYamlOutput(ctx *harvesterServer.Ctx, kubeConfig string) (harvesterServer.ResponseBody, error) {
	encodedKubeConfig := base64.StdEncoding.EncodeToString([]byte(kubeConfig))

	// Tell the framework don't reply OK again, app has replied directly
	ctx.SkipAutoResponse()

	rw := ctx.RespWriter()
	rw.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	rw.WriteHeader(http.StatusOK)

	// output both legacy and new cloud-config path
	yamlOutput := fmt.Sprintf(`########## cloud-init user data ############
write_files:
- encoding: b64
  content: %[1]s
  owner: root:root
  path: %[2]s
  permissions: '0644'
- encoding: b64
  content: %[1]s
  owner: root:root
  path: %[3]s
  permissions: '0644'
`, encodedKubeConfig, legacyPath, newPath)

	if _, err := rw.Write([]byte(yamlOutput)); err != nil {
		err := fmt.Errorf("failed to write output %s, err: %w", outputFormatYaml, err)
		logrus.Warnf("%s", err.Error())
		return nil, err
	}

	return nil, nil
}

// validateOwner performs a simple structural validation of the owner reference.
// It checks explicit metadata fields rather than running a complex validation
// that traces back through the reverse path of the creation process.
func validateOwner(owner []metav1.OwnerReference, sa *corev1.ServiceAccount) error {
	if len(owner) != 1 || owner[0].Kind != rbacv1.ServiceAccountKind || owner[0].Name != sa.Name || owner[0].UID != sa.UID {
		return fmt.Errorf("is not owned by serviceAccount name:%s UID:%s", sa.Name, sa.UID)
	}

	return nil
}

func (h *GenerateHandler) getRoleBinding(sa *corev1.ServiceAccount) (*rbacv1.RoleBinding, error) {
	roleBinding, err := h.roleBindingCache.Get(sa.Namespace, name.SafeConcatName(sa.Namespace, sa.Name))
	if err != nil {
		return nil, err
	}
	if err := validateOwner(roleBinding.OwnerReferences, sa); err != nil {
		return nil, fmt.Errorf("rolebinding %s/%s %w, try to delete and recreate", roleBinding.Namespace, roleBinding.Name, err)
	}
	return roleBinding, nil
}

func (h *GenerateHandler) getClusterRoleBinding(sa *corev1.ServiceAccount) (*rbacv1.ClusterRoleBinding, error) {
	clusterRoleBinding, err := h.clusterRoleBindingCache.Get(name.SafeConcatName(sa.Namespace, sa.Name))
	if err != nil {
		return nil, err
	}
	if err := validateOwner(clusterRoleBinding.OwnerReferences, sa); err != nil {
		return nil, fmt.Errorf("clusterrolebinding %s %w, try to delete and recreate", clusterRoleBinding.Name, err)
	}
	return clusterRoleBinding, nil
}

// getSaAndSecret returns the serviceAccount and the associated secret
func (h *GenerateHandler) getSaAndSecret(namespace, saName string) (*corev1.ServiceAccount, *corev1.Secret, error) {
	sa, err := h.saCache.Get(namespace, saName)
	if err != nil {
		return nil, nil, err
	}

	se, err := h.secretCache.Get(sa.Namespace, name.SafeConcatName(sa.Name, "token"))
	if err != nil {
		return nil, nil, err
	}
	return sa, se, nil
}
