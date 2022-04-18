package v100to101

import (
	"context"
	"reflect"

	"github.com/pkg/errors"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/longhorn/longhorn-manager/types"
	upgradeutil "github.com/longhorn/longhorn-manager/upgrade/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
)

// This upgrade is needed because we changed from using full image name (in v1.0.0)
// to use the checksum of image name (in v1.0.1) for instance manager labels.
// Therefore, we need to update all existing instance manager labels so that
// the updated Longhorn manager can correctly find them.
// Link to the original issue: https://github.com/longhorn/longhorn/issues/1323

const (
	upgradeLogPrefix = "upgrade from v1.0.0 to v1.0.1: "
)

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) error {
	if err := doInstanceManagerUpgrade(namespace, lhClient); err != nil {
		return err
	}

	if err := doInstanceManagerPodUpgrade(namespace, lhClient, kubeClient); err != nil {
		return err
	}

	return nil
}

func doInstanceManagerUpgrade(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade instance manager failed")
	}()

	imList, err := lhClient.LonghornV1beta2().InstanceManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, upgradeLogPrefix+"failed to list all existing instance managers during the instance managers upgrade")
	}

	for _, im := range imList.Items {
		if err := upgradeInstanceManagersLabels(&im, lhClient, namespace); err != nil {
			return err
		}
	}
	return nil
}

func upgradeInstanceManagersLabels(im *longhorn.InstanceManager, lhClient *lhclientset.Clientset, namespace string) (err error) {
	metadata, err := meta.Accessor(im)
	if err != nil {
		return err
	}
	oldInstanceManagerLabels := metadata.GetLabels()
	newInstanceManagerLabels := types.GetInstanceManagerLabels(im.Spec.NodeID, im.Spec.Image, im.Spec.Type)
	imImageLabelKey := types.GetLonghornLabelKey(types.LonghornLabelInstanceManagerImage)

	if oldInstanceManagerLabels[imImageLabelKey] == newInstanceManagerLabels[imImageLabelKey] {
		return nil
	}

	metadata.SetLabels(newInstanceManagerLabels)
	if im, err = lhClient.LonghornV1beta2().InstanceManagers(namespace).Update(context.TODO(), im, metav1.UpdateOptions{}); err != nil {
		return errors.Wrapf(err, upgradeLogPrefix+"failed to update the spec for instance manager %v during the instance managers upgrade", im.Name)
	}
	return nil
}

func doInstanceManagerPodUpgrade(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade instance manager pods failed")
	}()

	imList, err := lhClient.LonghornV1beta2().InstanceManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, upgradeLogPrefix+"failed to list all existing instance managers during the instance managers pods upgrade")
	}

	for _, im := range imList.Items {
		imPodsList, err := kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
			FieldSelector: "metadata.name=" + im.Name,
		})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return errors.Wrapf(err, upgradeLogPrefix+"failed to find pod for instance manager %v during the instance managers pods upgrade", im.Name)
		}
		for _, pod := range imPodsList.Items {
			if err := upgradeInstanceMangerPodLabel(&pod, &im, kubeClient, namespace); err != nil {
				return err
			}
		}
	}
	return nil
}

func upgradeInstanceMangerPodLabel(pod *v1.Pod, im *longhorn.InstanceManager, kubeClient *clientset.Clientset, namespace string) (err error) {
	metadata, err := meta.Accessor(pod)
	if err != nil {
		return err
	}
	podLabels := metadata.GetLabels()
	newPodLabels := upgradeutil.MergeStringMaps(podLabels, types.GetInstanceManagerLabels(im.Spec.NodeID, im.Spec.Image, im.Spec.Type))
	if reflect.DeepEqual(podLabels, newPodLabels) {
		return nil
	}
	metadata.SetLabels(newPodLabels)
	if _, err := kubeClient.CoreV1().Pods(namespace).Update(context.TODO(), pod, metav1.UpdateOptions{}); err != nil {
		return errors.Wrapf(err, upgradeLogPrefix+"failed to update the spec for instance manager pod %v during the instance managers upgrade", pod.Name)
	}
	return nil
}
