package api_test

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/util"
)

var _ = Describe("verify secrets", func() {
	It("verify update cpu manager policy failed", func() {
		cpuManagerPolicies := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      util.NodeCPUManagerPoliciesSecretName,
				Namespace: util.CattleSystemNamespaceName,
			},
			StringData: map[string]string{
				util.NodeCPUManagerPoliciesSecretName: `{"foo": "bar"}`,
			},
			Data: map[string][]byte{
				util.NodeCPUManagerPoliciesSecretName: []byte(`{}`),
			},
		}

		secret := harvester.Scaled().CoreFactory.Core().V1().Secret()
		_, err := secret.Update(cpuManagerPolicies)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).To(HavePrefix(`admission webhook "validator.harvesterhci.io" denied the request: json parse error: json: unknown field "foo"`))
	})
})
