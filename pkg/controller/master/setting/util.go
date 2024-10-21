package setting

import (
	"encoding/json"

	"github.com/sirupsen/logrus"
	"helm.sh/helm/v3/pkg/action"
	corev1 "k8s.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

func (h *Handler) syncImageFromHelmValues(setting *harvesterv1.Setting, helmKey string) error {
	getValues := action.NewGetValues(&h.helmConfiguration)
	getValues.AllValues = true
	helmValues, err := getValues.Run("harvester")
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"helm.name": "harvester",
		}).Error("failed to get helm values")
		return err
	}

	if helmValues[helmKey] == nil {
		logrus.WithFields(logrus.Fields{
			"helm.name": "harvester",
			"key":       helmKey,
		}).Warn("key is not set in helm values")
		return nil
	}

	keyValues := helmValues[helmKey].(map[string]interface{})
	if keyValues["image"] == nil {
		logrus.WithFields(logrus.Fields{
			"helm.name": "harvester",
			"key":       helmKey + ".image",
		}).Warn("key not set in helm values")
		return nil
	}

	imageValues := keyValues["image"].(map[string]interface{})
	repository := imageValues["repository"].(string)
	tag := imageValues["tag"].(string)
	imagePullPolicy := imageValues["imagePullPolicy"].(string)
	if repository == "" || tag == "" || imagePullPolicy == "" {
		logrus.WithFields(logrus.Fields{
			"helm.name":       "harvester",
			"key":             helmKey + ".image",
			"repository":      repository,
			"tag":             tag,
			"imagePullPolicy": imagePullPolicy,
		}).Warn("key has empty subfield")
		return nil
	}

	image := &settings.Image{
		Repository:      repository,
		Tag:             tag,
		ImagePullPolicy: corev1.PullPolicy(imagePullPolicy),
	}
	imageStr, err := json.Marshal(image)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"helm.name": "harvester",
			"key":       helmKey + ".image",
			"image":     image,
		}).Error("failed to marshal image")
		return err
	}

	if setting.Default == string(imageStr) {
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"name":  setting.Name,
		"image": string(imageStr),
	}).Info("Updating setting default value")
	settingCopy := setting.DeepCopy()
	settingCopy.Default = string(imageStr)
	_, err = h.settings.Update(settingCopy)
	return err
}
