package api_test

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	"github.com/tidwall/gjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	. "github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/framework/helper"
)

var _ = Describe("verify settings APIs", func() {

	Context("operate via steve API", func() {

		var (
			settingsAPI, resourceVersion, APIUIVersionVal string
		)

		BeforeEach(func() {
			settingsAPI = helper.BuildAPIURL("v1", "harvesterhci.io.settings", options.HTTPSListenPort)
		})

		Specify("view all the settings", func() {

			By("check all the settings")
			respCode, respBody, err := helper.GetObject(settingsAPI, nil)
			MustRespCodeIs(http.StatusOK, "retrieve all the settings", err, respCode, respBody)
		})

		Specify("edit api-ui-version", func() {

			By("check on api-ui-version")
			versionAPI := fmt.Sprintf("%s/%s", settingsAPI, "api-ui-version")
			respCode, respBody, err := helper.GetObject(versionAPI, nil)
			MustRespCodeIs(http.StatusOK, "view api-ui-version info", err, respCode, respBody)

			By("update api-ui-version's value")
			resourceVersion = gjson.GetBytes(respBody, "metadata.resourceVersion").Str
			APIUIVersion := harvesterv1.Setting{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Setting",
					APIVersion: "harvesterhci.io/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "api-ui-version",
					ResourceVersion: resourceVersion,
				},
				Value: "v0.0.0",
			}
			respCode, respBody, err = helper.PutObject(versionAPI, APIUIVersion)
			MustRespCodeIs(http.StatusOK, "update the api-ui-version", err, respCode, respBody)

			By("check if the api-ui-version has been updated")
			APIUIVersionVal = gjson.GetBytes(respBody, "value").Str
			respCode, respBody, err = helper.GetObject(versionAPI, nil)
			MustRespCodeIs(http.StatusOK, "get the updated api-ui-version", err, respCode, respBody)
			MustEqual(APIUIVersionVal, "v0.0.0")
		})
	})
})
