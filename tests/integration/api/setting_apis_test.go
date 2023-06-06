package api_test

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlmgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
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

var _ = Describe("verify mcm related settings", func() {
	Context("operate via steve API", func() {
		const (
			rancherManagerSupportSetting = "rancher-manager-support"
			clusterRegistrationSetting   = "cluster-registration-url"
		)
		var (
			settingsAPI, resourceVersion, SettingValue string
			featureController                          ctlmgmtv3.FeatureController
			mcmFeature                                 *mgmtv3.Feature
			local, second                              *mgmtv3.Cluster
			clusterController                          ctlmgmtv3.ClusterController
		)

		BeforeEach(func() {
			settingsAPI = helper.BuildAPIURL("v1", "harvesterhci.io.settings", options.HTTPSListenPort)
			mcmFeature = &mgmtv3.Feature{
				ObjectMeta: metav1.ObjectMeta{
					Name: "multi-cluster-management",
				},
				Spec: mgmtv3.FeatureSpec{},
				Status: mgmtv3.FeatureStatus{
					Default:     true,
					Dynamic:     false,
					LockedValue: &[]bool{true}[0],
					Description: "Multi-cluster provisioning and management of Kubernetes clusters.",
				},
			}

			local = &mgmtv3.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local",
				},
			}

			second = &mgmtv3.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "second",
				},
			}
			featureController = harvester.Scaled().RancherManagementFactory.Management().V3().Feature()
			clusterController = harvester.Scaled().RancherManagementFactory.Management().V3().Cluster()
			Eventually(func() error {
				_, err := featureController.Create(mcmFeature)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())

			Eventually(func() error {
				_, err := clusterController.Create(local)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())

			Eventually(func() error {
				_, err := clusterController.Create(second)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())

		})

		Specify("edit rancher-manager-support", func() {

			By("check on rancher-support-manager")
			rancherSupportManagerAPI := fmt.Sprintf("%s/%s", settingsAPI, rancherManagerSupportSetting)
			respCode, respBody, err := helper.GetObject(rancherSupportManagerAPI, nil)
			MustRespCodeIs(http.StatusOK, "view rancher-manager-support info", err, respCode, respBody)

			By("update rancher-support-manager value")
			resourceVersion = gjson.GetBytes(respBody, "metadata.resourceVersion").Str
			updatedSetting := harvesterv1.Setting{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Setting",
					APIVersion: "harvesterhci.io/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            rancherManagerSupportSetting,
					ResourceVersion: resourceVersion,
				},
				Value: "true",
			}
			respCode, respBody, err = helper.PutObject(rancherSupportManagerAPI, updatedSetting)
			MustRespCodeIs(http.StatusOK, "enable rancher-manager-support", err, respCode, respBody)

			By("check if the rancher-manager-support has been updated")
			SettingValue = gjson.GetBytes(respBody, "value").Str
			respCode, respBody, err = helper.GetObject(rancherSupportManagerAPI, nil)
			MustRespCodeIs(http.StatusOK, "get the updated rancher-manager-support", err, respCode, respBody)
			MustEqual(SettingValue, "true")

			By("fetching rancher-support-manager before attempted disable")
			rancherSupportManagerAPI = fmt.Sprintf("%s/%s", settingsAPI, rancherManagerSupportSetting)
			respCode, respBody, err = helper.GetObject(rancherSupportManagerAPI, nil)
			MustRespCodeIs(http.StatusOK, "view rancher-manager-support info", err, respCode, respBody)

			By("disabling rancher-support-manager setting")
			resourceVersion = gjson.GetBytes(respBody, "metadata.resourceVersion").Str
			updatedSetting = harvesterv1.Setting{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Setting",
					APIVersion: "harvesterhci.io/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            rancherManagerSupportSetting,
					ResourceVersion: resourceVersion,
				},
				Value: "false",
			}
			respCode, respBody, err = helper.PutObject(rancherSupportManagerAPI, updatedSetting)
			MustRespCodeIs(http.StatusMethodNotAllowed, "disable rancher-manager-support", err, respCode, respBody)

			By("get current cluster registration url")
			clusterRegistrationAPI := fmt.Sprintf("%s/%s", settingsAPI, clusterRegistrationSetting)
			respCode, respBody, err = helper.GetObject(clusterRegistrationAPI, nil)
			MustRespCodeIs(http.StatusOK, "view rancher-manager-support info", err, respCode, respBody)

			By("attempting to update cluster-registration-url")
			resourceVersion = gjson.GetBytes(respBody, "metadata.resourceVersion").Str
			updatedSetting = harvesterv1.Setting{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Setting",
					APIVersion: "harvesterhci.io/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            clusterRegistrationSetting,
					ResourceVersion: resourceVersion,
				},
				Value: "http://localhost/",
			}

			respCode, respBody, err = helper.PutObject(clusterRegistrationAPI, updatedSetting)
			MustRespCodeIs(http.StatusMethodNotAllowed, "enable cluster-registration-url", err, respCode, respBody)
		})
	})
})
