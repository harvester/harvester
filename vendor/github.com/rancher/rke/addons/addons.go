package addons

import (
	"fmt"
	"strconv"

	"k8s.io/client-go/transport"

	"github.com/blang/semver"
	"github.com/rancher/rke/k8s"
	"github.com/rancher/rke/templates"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetAddonsExecuteJob(addonName, nodeName, image, k8sVersion string) (string, error) {
	return getAddonJob(addonName, nodeName, image, k8sVersion, false)
}

func GetAddonsDeleteJob(addonName, nodeName, image, k8sVersion string) (string, error) {
	return getAddonJob(addonName, nodeName, image, k8sVersion, true)
}

func getAddonJob(addonName, nodeName, image, k8sVersion string, isDelete bool) (string, error) {
	OSLabel := "beta.kubernetes.io/os"
	toMatch, err := semver.Make(k8sVersion[1:])
	if err != nil {
		return "", fmt.Errorf("Cluster version [%s] can not be parsed as semver: %v", k8sVersion, err)
	}

	logrus.Debugf("Checking addon job OS label for cluster version [%s]", k8sVersion)
	// kubernetes.io/os should be used 1.22.0 and up
	OSLabelRange, err := semver.ParseRange(">=1.22.0-rancher0")
	if err != nil {
		return "", fmt.Errorf("Failed to parse semver range for checking OS label for addon job: %v", err)
	}
	if OSLabelRange(toMatch) {
		logrus.Debugf("Cluster version [%s] needs to use new OS label", k8sVersion)
		OSLabel = "kubernetes.io/os"
	}

	jobConfig := map[string]string{
		"AddonName": addonName,
		"NodeName":  nodeName,
		"Image":     image,
		"DeleteJob": strconv.FormatBool(isDelete),
		"OSLabel":   OSLabel,
	}
	template, err := templates.CompileTemplateFromMap(templates.AddonJobTemplate, jobConfig)
	logrus.Tracef("template for [%s] is: [%s]", addonName, template)
	return template, err
}

func AddonJobExists(addonJobName, kubeConfigPath string, k8sWrapTransport transport.WrapperFunc) (bool, error) {
	k8sClient, err := k8s.NewClient(kubeConfigPath, k8sWrapTransport)
	if err != nil {
		return false, err
	}
	addonJobStatus, err := k8s.GetK8sJobStatus(k8sClient, addonJobName, metav1.NamespaceSystem)
	if err != nil {
		return false, fmt.Errorf("Failed to get job [%s] status: %v", addonJobName, err)
	}
	return addonJobStatus.Created, nil
}
