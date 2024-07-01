package secret

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator() types.Validator {
	return &secretValidator{}
}

type secretValidator struct {
	types.DefaultValidator
}

func (v *secretValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{corev1.ResourceSecrets.String()},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.Secret{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

type nodeCPUManagerPolicies struct {
	Configs []nodeCPUManagerPolicy `json:"configs"`
}

type nodeCPUManagerPolicy struct {
	Name   string `json:"name"`
	Policy string `json:"policy"`
}

func (v *secretValidator) Create(_ *types.Request, newObj runtime.Object) error {
	secret := newObj.(*corev1.Secret)
	if secret.Name == util.NodeCPUManagerPoliciesSecretName {
		return validateNodeCPUManagerPolicies(secret)
	}
	return nil
}

func (v *secretValidator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) error {
	secret := newObj.(*corev1.Secret)
	if secret.Name == util.NodeCPUManagerPoliciesSecretName {
		return validateNodeCPUManagerPolicies(secret)
	}
	return nil
}

func validateNodeCPUManagerPolicies(secret *corev1.Secret) error {
	if secret.Namespace != util.CattleSystemNamespaceName {
		return werror.NewInvalidError(fmt.Sprintf("%s namespace should be %s", util.NodeCPUManagerPoliciesSecretName, util.CattleSystemNamespaceName), "")
	}

	stringData, stringDataOK := secret.StringData[util.NodeCPUManagerPoliciesSecretName]
	data, dataOk := secret.Data[util.NodeCPUManagerPoliciesSecretName]
	if !stringDataOK && !dataOk {
		return werror.NewInvalidError("node cpu manager policies is nil", "")
	}

	// If NodeCPUManagerPoliciesSecretName exists in both StringData and Data, StringData takes precedence.
	// See https://kubernetes.io/docs/tasks/configmap-secret/managing-secret-using-config-file/#specify-both-data-and-stringdata.
	// Therefore, only verify StringData if NodeCPUManagerPoliciesSecretName exists in both StringData and Data.
	var decoder *json.Decoder
	if stringDataOK {
		decoder = json.NewDecoder(strings.NewReader(stringData))
	} else {
		decoder = json.NewDecoder(bytes.NewReader(data))
	}
	decoder.DisallowUnknownFields()

	var policies nodeCPUManagerPolicies
	if err := decoder.Decode(&policies); err != nil {
		return werror.NewInvalidError(fmt.Sprintf("json parse error: %s", err.Error()), "")
	}
	for _, config := range policies.Configs {
		if config.Policy != "static" && config.Policy != "none" {
			return werror.NewInvalidError("cpu manager policy should be either none or static", "")
		}
	}
	return nil
}
