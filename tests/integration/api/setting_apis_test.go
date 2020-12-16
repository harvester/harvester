package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/guonaihong/gout"
	. "github.com/onsi/ginkgo"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/settings"
	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/fuzz"
	"github.com/rancher/harvester/tests/framework/helper"
)

var _ = Describe("verify settings apis", func() {

	Cleanup(func() {

		scaled := config.ScaledWithContext(harvester.Context)
		settingController := scaled.HarvesterFactory.Harvester().V1alpha1().Setting()
		settingList, err := settingController.List(metav1.ListOptions{LabelSelector: labels.FormatLabels(testResourceLabels)})
		if err != nil {
			GinkgoT().Logf("failed to list tested settings, %v", err)
			return
		}
		for _, item := range settingList.Items {
			if err := settingController.Delete(item.Name, &metav1.DeleteOptions{}); err != nil {
				GinkgoT().Logf("failed to delete tested setting %s/%s, %v", item.Namespace, item.Name, err)
			}
		}

	})

	Context("operate via steve api", func() {

		var settingsAPI string

		BeforeEach(func() {

			settingsAPI = helper.BuildAPIURL("v1", "harvester.cattle.io.settings")

		})

		It("should get the value from preset ENV", func() {

			// NB(thxCode): in order to test the preset ENV setting,
			// we need to inject an environment variable in the BeforeSuite.

			By("given a preset setting: api-ui-version")
			var setting = settings.APIUIVersion
			var (
				respCode int
				respBody []byte
			)
			var err = helper.NewHTTPClient().
				GET(fmt.Sprintf("%s/%s", settingsAPI, setting.Name)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusOK, "get setting", err, respCode, respBody)

			By("when modify the api-ui-version setting")
			var updatedSettingValue = "v0.0.0"
			respBodyUpdated, err := sjson.SetBytes(respBody, "value", updatedSettingValue)
			MustNotError(err)
			err = helper.NewHTTPClient().
				PUT(fmt.Sprintf("%s/%s", settingsAPI, setting.Name)).
				SetBody(respBodyUpdated).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusOK, "put setting", err, respCode, respBody)

			By("then we still can get the value from preset ENV")
			MustChanged(setting.Get(), gjson.GetBytes(respBody, "default").String(), updatedSettingValue)
		})

		Specify("create a custom setting", func() {

			By("given a random setting")
			var settingName = strings.ToLower(fuzz.String(5))
			var settingValue = fuzz.String(5)
			var setting = gout.H{
				"metadata": map[string]interface{}{
					"name": settingName,
					"labels": map[string]string{
						"test.harvester.cattle.io": "for-test",
					},
				},
				"value": settingValue,
			}

			By("when create the setting")
			var (
				respCode int
				respBody []byte
			)
			var err = helper.NewHTTPClient().
				POST(settingsAPI).
				SetJSON(setting).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusCreated, "post setting", err, respCode, respBody)

			By("then succeeded")
			MustFinallyBeTrue(func() bool {
				var (
					respCode int
					respBody []byte
				)
				var err = helper.NewHTTPClient().
					GET(fmt.Sprintf("%s/%s", settingsAPI, settingName)).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				return CheckRespCodeIs(http.StatusOK, "get setting", err, respCode, respBody) &&
					gjson.GetBytes(respBody, "value").String() == settingValue
			})

			By("when delete the setting")
			err = helper.NewHTTPClient().
				DELETE(fmt.Sprintf("%s/%s", settingsAPI, settingName)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusNoContent, "delete setting", err, respCode, respBody)

			By("then succeeded")
			MustFinallyBeTrue(func() bool {
				var (
					respCode int
				)
				err := helper.NewHTTPClient().
					GET(fmt.Sprintf("%s/%s", settingsAPI, settingName)).
					Code(&respCode).
					Do()
				return CheckRespCodeIs(http.StatusNotFound, "get setting", err, respCode, respBody)
			})
		})

	})

})
