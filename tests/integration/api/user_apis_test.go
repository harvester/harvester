package api_test

import (
	"net/http"

	. "github.com/onsi/ginkgo"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	. "github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

const (
	namespace = "harvester-system"
)

var _ = Describe("verify users APIs", func() {

	var (
		scaled                   *config.Scaled
		secretController         ctlcorev1.SecretController
		serviceAccountController ctlcorev1.ServiceAccountController
	)

	BeforeEach(func() {
		scaled = harvester.Scaled()
		secretController = scaled.CoreFactory.Core().V1().Secret()
		serviceAccountController = scaled.CoreFactory.Core().V1().ServiceAccount()
	})

	Context("operate via steve API", func() {

		var (
			loginAPI string
		)

		BeforeEach(func() {

			var port = options.HTTPSListenPort
			loginAPI = helper.BuildAPIURL("", "v1-public/auth?action=login", port)
		})

		Specify("login in by using serviceAccount name", func() {

			By("get the service account name")
			sa, err := serviceAccountController.Get(namespace, "harvester", metav1.GetOptions{})
			MustNotError(err)
			MustEqual(len(sa.Secrets), 1)

			By("get token")
			secretName := sa.Secrets[0].Name
			secret, err := secretController.Get(namespace, secretName, metav1.GetOptions{})
			MustNotError(err)

			By("login in with token")
			loginToken := harvesterv1.Login{
				Token: string(secret.Data["token"]),
			}
			respCode, respBody, err := helper.PostObject(loginAPI, loginToken)
			MustRespCodeIs(http.StatusOK, "login", err, respCode, respBody)

			By("use invalid service account secret to login")
			loginInvalidToken := harvesterv1.Login{
				Token: fuzz.String(16),
			}
			respCode, respBody, err = helper.PostObject(loginAPI, loginInvalidToken)
			MustRespCodeIs(http.StatusUnauthorized, "invalid token login", err, respCode, respBody)
		})

		Specify("login in by using kubeconfig", func() {

			By("get valid kubeConfig")
			rawConfig, err := KubeClientConfig.RawConfig()
			MustNotError(err)
			validKubeConfig, err := clientcmd.Write(rawConfig)
			MustNotError(err)

			By("login with valid kubeConfig")
			loginValidConfig := harvesterv1.Login{
				KubeConfig: string(validKubeConfig),
			}
			respCode, respBody, err := helper.PostObject(loginAPI, loginValidConfig)
			MustRespCodeIs(http.StatusOK, "login with kubeConfig", err, respCode, respBody)

			By("login with invalid kubeConfig")
			loginInvalidConfig := harvesterv1.Login{
				KubeConfig: fuzz.String(20),
			}
			respCode, respBody, err = helper.PostObject(loginAPI, loginInvalidConfig)
			MustRespCodeIs(http.StatusUnauthorized, "login with invalid kubeConfig", err, respCode, respBody)
		})

	})

})
