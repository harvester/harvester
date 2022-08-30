package util

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	"github.com/longhorn/longhorn-manager/meta"
	"github.com/longhorn/longhorn-manager/types"
)

type ProgressMonitor struct {
	description                 string
	targetValue                 int
	currentValue                int
	currentProgressInPercentage float64
	mutex                       *sync.RWMutex
}

func NewProgressMonitor(description string, currentValue, targetValue int) *ProgressMonitor {
	pm := &ProgressMonitor{
		description:                 description,
		targetValue:                 targetValue,
		currentValue:                currentValue,
		currentProgressInPercentage: math.Floor(float64(currentValue*100) / float64(targetValue)),
		mutex:                       &sync.RWMutex{},
	}
	pm.logCurrentProgress()
	return pm
}

func (pm *ProgressMonitor) logCurrentProgress() {
	logrus.Infof("%v: current progress %v%% (%v/%v)", pm.description, pm.currentProgressInPercentage, pm.currentValue, pm.targetValue)
}

func (pm *ProgressMonitor) Inc() int {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	oldProgressInPercentage := pm.currentProgressInPercentage

	pm.currentValue++
	pm.currentProgressInPercentage = math.Floor(float64(pm.currentValue*100) / float64(pm.targetValue))
	if pm.currentProgressInPercentage != oldProgressInPercentage {
		pm.logCurrentProgress()
	}
	return pm.currentValue
}

func (pm *ProgressMonitor) SetCurrentValue(newValue int) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	oldProgressInPercentage := pm.currentProgressInPercentage

	pm.currentValue = newValue
	pm.currentProgressInPercentage = math.Floor(float64(pm.currentValue*100) / float64(pm.targetValue))
	if pm.currentProgressInPercentage != oldProgressInPercentage {
		pm.logCurrentProgress()
	}
}

func (pm *ProgressMonitor) GetCurrentProgress() (int, int, float64) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	return pm.currentValue, pm.targetValue, pm.currentProgressInPercentage
}

func ListShareManagerPods(namespace string, kubeClient *clientset.Clientset) ([]v1.Pod, error) {
	smPodsList, err := kubeClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.Set(types.GetShareManagerComponentLabel()).String(),
	})
	if err != nil {
		return nil, err
	}
	return smPodsList.Items, nil
}

func ListIMPods(namespace string, kubeClient *clientset.Clientset) ([]v1.Pod, error) {
	imPodsList, err := kubeClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", types.GetLonghornLabelComponentKey(), types.LonghornLabelInstanceManager),
	})
	if err != nil {
		return nil, err
	}
	return imPodsList.Items, nil
}

func MergeStringMaps(baseMap, overwriteMap map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range baseMap {
		result[k] = v
	}
	for k, v := range overwriteMap {
		result[k] = v
	}
	return result
}

func GetCurrentLonghornVersion(namespace string, lhClient *lhclientset.Clientset) (string, error) {
	currentLHVersionSetting, err := lhClient.LonghornV1beta2().Settings(namespace).Get(context.TODO(), string(types.SettingNameCurrentLonghornVersion), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	return currentLHVersionSetting.Value, nil
}

func CreateOrUpdateLonghornVersionSetting(namespace string, lhClient *lhclientset.Clientset) error {
	s, err := lhClient.LonghornV1beta2().Settings(namespace).Get(context.TODO(), string(types.SettingNameCurrentLonghornVersion), metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		s = &longhorn.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: string(types.SettingNameCurrentLonghornVersion),
			},
			Value: meta.Version,
		}
		_, err := lhClient.LonghornV1beta2().Settings(namespace).Create(context.TODO(), s, metav1.CreateOptions{})
		return err
	}

	if s.Value != meta.Version {
		s.Value = meta.Version
		_, err = lhClient.LonghornV1beta2().Settings(namespace).Update(context.TODO(), s, metav1.UpdateOptions{})
		return err
	}
	return nil
}
