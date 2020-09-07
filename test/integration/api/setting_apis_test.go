package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"github.com/guonaihong/gout"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/settings"
	. "github.com/rancher/harvester/test/framework/envtest/dsl"
	"github.com/rancher/harvester/test/framework/fuzz"
)

var _ = Describe("verify settings apis", func() {

	Cleanup(func() {

		var settingCli = harvester.Scaled().HarvesterFactory.Harvester().V1alpha1().Setting()
		var list, err = settingCli.List(metav1.ListOptions{LabelSelector: labels.FormatLabels(map[string]string{"test.harvester.cattle.io": "for-test"})})
		if err != nil {
			GinkgoT().Logf("failed to list tested settings, %v", err)
			return
		}
		for _, item := range list.Items {
			var err = settingCli.Delete(item.Name, &metav1.DeleteOptions{})
			if err != nil {
				GinkgoT().Logf("failed to delete tested setting %s/%s, %v", item.Namespace, item.Name, err)
			}
		}

	})

	Context("operate via steve api", func() {

		var settingsAPI string

		BeforeEach(func() {

			settingsAPI = fmt.Sprintf("https://localhost:%d/v1/harvester.cattle.io.settings", config.HTTPSListenPort)

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
			var err = gout.
				New(&http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}).
				GET(fmt.Sprintf("%s/%s", settingsAPI, setting.Name)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusOK {
				GinkgoT().Errorf("failed to get setting, response with %d, %v", respCode, string(respBody))
			}

			By("when modify the api-ui-version setting")
			var updatedSettingValue = "v0.0.0"
			respBodyUpdated, err := sjson.SetBytes(respBody, "value", updatedSettingValue)
			Expect(err).ShouldNot(HaveOccurred())
			err = gout.
				New(&http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}).
				PUT(fmt.Sprintf("%s/%s", settingsAPI, setting.Name)).
				SetBody(respBodyUpdated).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusOK {
				GinkgoT().Errorf("failed to put setting, response with %d, %v", respCode, string(respBody))
			}

			By("then we still can get the value from preset ENV")
			Expect(setting.Get()).Should(Equal(gjson.GetBytes(respBody, "default").String()))
			Expect(setting.Get()).ShouldNot(Equal(updatedSettingValue))

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
			var err = gout.
				New(&http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}).
				POST(settingsAPI).
				SetJSON(setting).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusCreated {
				GinkgoT().Errorf("failed to post setting, response with %d, %v", respCode, string(respBody))
			}

			By("then succeeded")
			Eventually(func() bool {
				var (
					respCode int
					respBody []byte
				)
				var err = gout.
					New(&http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						},
					}).
					GET(fmt.Sprintf("%s/%s", settingsAPI, settingName)).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				if err != nil {
					GinkgoT().Logf("failed to get setting, %v", err)
					return false
				}
				if respCode != http.StatusOK {
					GinkgoT().Logf("failed to get setting, response with %d, %v", respCode, string(respBody))
					return false
				}
				return gjson.GetBytes(respBody, "value").String() == settingValue
			}, 10, 1).Should(BeTrue())

			By("when delete the setting")
			err = gout.
				New(&http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}).
				DELETE(fmt.Sprintf("%s/%s", settingsAPI, settingName)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusNoContent {
				GinkgoT().Errorf("failed to delete setting, response with %d, %v", respCode, string(respBody))
			}

			By("then succeeded")
			Eventually(func() bool {
				var (
					respCode int
				)
				var err = gout.
					New(&http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						},
					}).
					GET(fmt.Sprintf("%s/%s", settingsAPI, settingName)).
					Code(&respCode).
					Do()
				if err != nil {
					GinkgoT().Logf("failed to get setting, %v", err)
					return false
				}
				return respCode == http.StatusNotFound
			}, 10, 1).Should(BeTrue())
		})

	})

})
