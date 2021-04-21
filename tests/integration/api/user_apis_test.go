package api_test

import (
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/bcrypt"
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
			userID, userName, password, updatedPassword,
			usersKubeAPI, usersAPI, loginAPI, resourceVersion string
		)

		userName = strings.ToLower(fuzz.String(5))
		password = fuzz.String(6)

		BeforeEach(func() {

			var port = options.HTTPSListenPort
			usersAPI = helper.BuildAPIURL("v1", "harvesterhci.io.users", port)
			usersKubeAPI = helper.BuildAPIURL("", "apis/harvesterhci.io/v1beta1/users", port)
			loginAPI = helper.BuildAPIURL("", "v1-public/auth?action=login", port)
		})

		Specify("create a user", func() {

			By("given a username & password")
			user := harvesterv1.User{
				Username: userName,
				Password: password,
				IsAdmin:  true,
			}

			By("when calling harvesterhci.io.users")
			respCode, respBody, err := helper.PostObject(usersAPI, user)
			MustRespCodeIs(http.StatusCreated, "create user", err, respCode, respBody)
			userID = gjson.GetBytes(respBody, "metadata.name").Str
			resourceVersion = gjson.GetBytes(respBody, "metadata.resourceVersion").Str

		})

		Specify("list users to check if the user is created or not", func() {

			By("when calling harvesterhci.io.users to get user list")
			respCode, respBody, err := helper.GetObject(usersAPI, nil)
			MustRespCodeIs(http.StatusOK, "get user list", err, respCode, respBody)

			By("get user json")
			selectUserJSONPath := fmt.Sprintf("data.#(username==\"%s\")", userName)
			userJSON := gjson.GetBytes(respBody, selectUserJSONPath)
			MustEqual(userJSON.Exists(), true)

		})

		Specify("login in by using local user", func() {

			By("login in with the created user and invalid password")
			login := harvesterv1.Login{
				Username: userName,
				Password: fuzz.String(8),
			}
			respCode, respBody, err := helper.PostObject(loginAPI, login)
			MustRespCodeIs(http.StatusUnauthorized, "password incorrect", err, respCode, respBody)

			By("login in with invalid username and password")
			login = harvesterv1.Login{
				Username: fuzz.String(6),
				Password: fuzz.String(10),
			}
			respCode, respBody, err = helper.PostObject(loginAPI, login)
			MustRespCodeIs(http.StatusUnauthorized, "user name or password incorrect", err, respCode, respBody)

			By("login in with the correct username and password")
			login = harvesterv1.Login{
				Username: userName,
				Password: password,
			}
			respCode, respBody, err = helper.PostObject(loginAPI, login)
			MustRespCodeIs(http.StatusOK, "sign in correctly", err, respCode, respBody)

		})

		Specify("update a user's password", func() {

			By("update the user's password")
			updateAPI := fmt.Sprintf("%s/%s", usersAPI, userID)
			updatedPassword = fuzz.String(8)
			user := harvesterv1.User{
				TypeMeta: metav1.TypeMeta{
					Kind:       "User",
					APIVersion: "harvesterhci.io/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: resourceVersion,
					Name:            userID,
				},
				Username: userName,
				Password: updatedPassword,
				IsAdmin:  true,
			}
			respCode, respBody, err := helper.PutObject(updateAPI, user)
			MustRespCodeIs(http.StatusOK, "update user's password", err, respCode, respBody)

			By("get the updated user info")
			respCode, respBody, err = helper.GetObject(fmt.Sprintf("%s/%s", usersKubeAPI, userID), nil)
			MustRespCodeIs(http.StatusOK, "get updated user info", err, respCode, respBody)

			By("get password")
			passwordJSON := gjson.GetBytes(respBody, "password")
			MustEqual(passwordJSON.Exists(), true)

			By("compare password")
			err = bcrypt.CompareHashAndPassword([]byte(passwordJSON.Str), []byte(updatedPassword))
			MustNotError(err)

		})

		Specify("delete a user", func() {

			By("delete a user via user id")
			respCode, respBody, err := helper.DeleteObject(fmt.Sprintf("%s/%s", usersAPI, userID))
			MustRespCodeIs(http.StatusNoContent, "delete a user successfully", err, respCode, respBody)

			By("the deleted user cannot be retrieved anymore")
			respCode, respBody, err = helper.GetObject(fmt.Sprintf("%s/%s", usersAPI, userID), nil)
			MustRespCodeIs(http.StatusNotFound, "retrieve",
				err, respCode, respBody)

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
