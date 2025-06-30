package setting

import (
	"encoding/json"
	"reflect"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	utilHelm "github.com/harvester/harvester/pkg/util/helm"
)

func UpdateSupportBundleImage(clientSet *kubernetes.Clientset, settingClient v1beta1.SettingClient) error {
	supportBundleImage, err := utilHelm.FetchImageFromHelmValues(
		clientSet,
		util.FleetLocalNamespaceName,
		util.HarvesterChartReleaseName,
		[]string{"support-bundle-kit", "image"},
	)
	if err != nil {
		logrus.WithError(err).Error("failed to get support bundle image")
		return err
	}
	imageBytes, err := json.Marshal(&supportBundleImage)
	if err != nil {
		return err
	}
	supportBundleImageSetting, err := settingClient.Get(settings.SupportBundleImageName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	supportBundleImageSettingCpy := supportBundleImageSetting.DeepCopy()
	supportBundleImageSettingCpy.Default = string(imageBytes)

	if reflect.DeepEqual(supportBundleImage, supportBundleImageSettingCpy) {
		return nil
	}
	_, err = settingClient.Update(supportBundleImageSettingCpy)
	return err
}
