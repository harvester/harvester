package v110to111

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strconv"

	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/longhorn/longhorn-manager/types"
	upgradeutil "github.com/longhorn/longhorn-manager/upgrade/util"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
)

const (
	upgradeLogPrefix = "upgrade from v1.1.0 to v1.1.1: "
)

// This upgrade allow Longhorn switch to the new CPU settings as well as
// deprecating the old setting automatically:
// https://github.com/longhorn/longhorn/issues/2207

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) (err error) {
	// Upgrade Longhorn resources
	if err := upgradeLonghornNodes(namespace, lhClient); err != nil {
		return err
	}
	if err := upgradeInstanceManagers(namespace, lhClient); err != nil {
		return err
	}
	if err := upgradeShareManagers(namespace, lhClient); err != nil {
		return err
	}
	if err := upgradeEngineImages(namespace, lhClient); err != nil {
		return err
	}

	// Upgrade Kubernetes resources
	if err := upgradeInstanceManagerPods(namespace, kubeClient); err != nil {
		return err
	}
	if err := upgradeShareManagerPods(namespace, lhClient, kubeClient); err != nil {
		return err
	}
	if err := upgradeCSIServicesLabels(kubeClient, namespace); err != nil {
		return err
	}
	if err := upgradeShareManagerServicesLabels(kubeClient, namespace); err != nil {
		return err
	}
	if err := upgradeCSIDeploymentsLabels(kubeClient, namespace); err != nil {
		return err
	}
	if err := upgradeCSIDaemonSetsLabels(kubeClient, namespace); err != nil {
		return err
	}
	if err := updateCSIDeploymentsLastAppliedTolerationsAnnotation(kubeClient, namespace); err != nil {
		return err
	}
	if err := updateCSIDaemonSetsLastAppliedTolerationsAnnotation(kubeClient, namespace); err != nil {
		return err
	}

	return nil
}

func upgradeLonghornNodes(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade longhorn node failed")
	}()

	deprecatedCPUSetting, err := lhClient.LonghornV1beta2().Settings(namespace).Get(context.TODO(), string(types.SettingNameGuaranteedEngineCPU), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if deprecatedCPUSetting.Value == "" {
		return nil
	}

	requestedCPU, err := strconv.ParseFloat(deprecatedCPUSetting.Value, 64)
	if err != nil {
		return errors.Wrapf(err, upgradeLogPrefix+"failed to convert the deprecated setting %v value %v to an float", types.SettingNameGuaranteedEngineCPU, deprecatedCPUSetting.Value)
	}
	// Convert to milli value
	requestedMilliCPU := int(math.Round(requestedCPU * 1000))

	nodeList, err := lhClient.LonghornV1beta2().Nodes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, upgradeLogPrefix+"failed to list all existing Longhorn nodes")
	}

	for _, node := range nodeList.Items {
		updateRequired := false
		if node.Spec.EngineManagerCPURequest == 0 {
			node.Spec.EngineManagerCPURequest = requestedMilliCPU
			updateRequired = true
		}
		if node.Spec.ReplicaManagerCPURequest == 0 {
			node.Spec.ReplicaManagerCPURequest = requestedMilliCPU
			updateRequired = true
		}
		if updateRequired {
			if _, err := lhClient.LonghornV1beta2().Nodes(namespace).Update(context.TODO(), &node, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	deprecatedCPUSetting.Value = ""
	if _, err := lhClient.LonghornV1beta2().Settings(namespace).Update(context.TODO(), deprecatedCPUSetting, metav1.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}

func upgradeInstanceManagers(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade instance manager failed")
	}()

	imList, err := lhClient.LonghornV1beta2().InstanceManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, im := range imList.Items {
		if err := upgradeLabelsForInstanceManager(&im, lhClient, namespace); err != nil {
			return err
		}
	}
	return nil
}
func upgradeLabelsForInstanceManager(im *longhorn.InstanceManager, lhClient *lhclientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "upgradeLabelsForInstanceManager failed")
	}()
	metadata, err := meta.Accessor(im)
	if err != nil {
		return err
	}
	instanceManagerLabels := metadata.GetLabels()
	newInstanceManagerLabels := upgradeutil.MergeStringMaps(instanceManagerLabels, types.GetInstanceManagerLabels(im.Spec.NodeID, im.Spec.Image, im.Spec.Type))
	if reflect.DeepEqual(instanceManagerLabels, newInstanceManagerLabels) {
		return nil
	}

	metadata.SetLabels(newInstanceManagerLabels)
	if _, err := lhClient.LonghornV1beta2().InstanceManagers(namespace).Update(context.TODO(), im, metav1.UpdateOptions{}); err != nil {
		return errors.Wrapf(err, upgradeLogPrefix+"failed to update the spec for instance manager %v during the instance managers upgrade", im.Name)
	}
	return nil
}

func upgradeShareManagers(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade share manager failed")
	}()
	smList, err := lhClient.LonghornV1beta2().ShareManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, sm := range smList.Items {
		if err := upgradeLabelsForShareManager(&sm, lhClient, namespace); err != nil {
			return err
		}
	}
	return nil
}

func upgradeLabelsForShareManager(sm *longhorn.ShareManager, lhClient *lhclientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "upgradeLabelsForShareManager failed")
	}()
	metadata, err := meta.Accessor(sm)
	if err != nil {
		return err
	}
	shareManagerLabels := metadata.GetLabels()
	newShareManagerLabels := upgradeutil.MergeStringMaps(shareManagerLabels, types.GetShareManagerLabels(sm.Name, sm.Spec.Image))
	if reflect.DeepEqual(shareManagerLabels, newShareManagerLabels) {
		return nil
	}
	metadata.SetLabels(newShareManagerLabels)
	if _, err := lhClient.LonghornV1beta2().ShareManagers(namespace).Update(context.TODO(), sm, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func upgradeEngineImages(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade engine image failed")
	}()

	eiList, err := lhClient.LonghornV1beta2().EngineImages(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, ei := range eiList.Items {
		if err := upgradeLabelsForEngineImage(&ei, lhClient, namespace); err != nil {
			return err
		}
	}
	return nil
}

func upgradeLabelsForEngineImage(ei *longhorn.EngineImage, lhClient *lhclientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "upgradeLabelsForEngineImage failed")
	}()
	metadata, err := meta.Accessor(ei)
	if err != nil {
		return err
	}
	engineImageLabels := metadata.GetLabels()
	newEngineImageLabels := upgradeutil.MergeStringMaps(engineImageLabels, types.GetEngineImageLabels(ei.Name))
	if reflect.DeepEqual(engineImageLabels, newEngineImageLabels) {
		return nil
	}
	metadata.SetLabels(newEngineImageLabels)
	if _, err := lhClient.LonghornV1beta2().EngineImages(namespace).Update(context.TODO(), ei, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func upgradeShareManagerPods(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade share manager pods failed")
	}()
	smPods, err := upgradeutil.ListShareManagerPods(namespace, kubeClient)
	if err != nil {
		return err
	}
	for _, pod := range smPods {
		sm, err := lhClient.LonghornV1beta2().ShareManagers(namespace).Get(context.TODO(), types.GetShareManagerNameFromShareManagerPodName(pod.Name), metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if err := upgradeShareManagerPodsLabels(&pod, sm, kubeClient, namespace); err != nil {
			return err
		}
	}
	return nil
}

func upgradeShareManagerPodsLabels(pod *v1.Pod, sm *longhorn.ShareManager, kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "upgradeShareManagerPodsLabels failed")
	}()
	metadata, err := meta.Accessor(pod)
	if err != nil {
		return err
	}
	podLabels := metadata.GetLabels()
	newPodLabels := upgradeutil.MergeStringMaps(podLabels, types.GetShareManagerLabels(sm.Name, sm.Spec.Image))
	if reflect.DeepEqual(podLabels, newPodLabels) {
		return nil
	}
	metadata.SetLabels(newPodLabels)
	if _, err := kubeClient.CoreV1().Pods(namespace).Update(context.TODO(), pod, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func upgradeInstanceManagerPods(namespace string, kubeClient *clientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade instance manager pods failed")
	}()
	imPods, err := upgradeutil.ListIMPods(namespace, kubeClient)
	if err != nil {
		return err
	}
	for _, pod := range imPods {
		if err := updateIMPodLastAppliedTolerationsAnnotation(&pod, kubeClient, namespace); err != nil {
			return err
		}
		// Note that we already upgrade instance manager pods' labels in v100to101, so no need to upgrade it here
	}
	return nil
}

func updateIMPodLastAppliedTolerationsAnnotation(pod *v1.Pod, kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "updateIMPodLastAppliedTolerationsAnnotation failed")
	}()
	needToUpdate, err := needToUpdateTolerationAnnotation(pod)
	if err != nil {
		return err
	}
	if !needToUpdate {
		return nil
	}
	appliedTolerations := getNonDefaultTolerationList(pod.Spec.Tolerations)
	appliedTolerationsByte, err := json.Marshal(appliedTolerations)
	if err != nil {
		return err
	}
	if err := util.SetAnnotation(pod, types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix), string(appliedTolerationsByte)); err != nil {
		return err
	}
	if _, err = kubeClient.CoreV1().Pods(namespace).Update(context.TODO(), pod, metav1.UpdateOptions{}); err != nil {
		return errors.Wrapf(err, "failed to update toleration annotation for instance manager pod %v", pod.GetName())
	}
	return nil
}

func upgradeCSIDeploymentsLabels(kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade CSI Deployments labels failed")
	}()
	for _, dpName := range []string{types.CSIAttacherName, types.CSIProvisionerName, types.CSIResizerName, types.CSISnapshotterName} {
		dp, err := kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), dpName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if err := upgradeLabelsForDeployment(dp, kubeClient, namespace); err != nil {
			return err
		}
	}
	return nil
}

func upgradeLabelsForDeployment(dp *appsv1.Deployment, kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "upgradeLabelsForDeployment failed")
	}()
	metadata, err := meta.Accessor(dp)
	if err != nil {
		return err
	}
	dpLabels := metadata.GetLabels()
	newDPLabels := upgradeutil.MergeStringMaps(dpLabels, types.GetBaseLabelsForSystemManagedComponent())
	if reflect.DeepEqual(dpLabels, newDPLabels) {
		return nil
	}
	metadata.SetLabels(newDPLabels)
	if _, err := kubeClient.AppsV1().Deployments(namespace).Update(context.TODO(), dp, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func upgradeCSIDaemonSetsLabels(kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade CSI DaemonSets labels failed")
	}()
	dsList := []*appsv1.DaemonSet{}

	eiDaemonSetList, err := kubeClient.AppsV1().DaemonSets(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.Set(types.GetEngineImageComponentLabel()).String(),
	})
	if err != nil {
		return err
	}
	for _, ds := range eiDaemonSetList.Items {
		dsList = append(dsList, &ds)
	}

	csiPluginDaemonSet, err := kubeClient.AppsV1().DaemonSets(namespace).Get(context.TODO(), types.CSIPluginName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		dsList = append(dsList, csiPluginDaemonSet)
	}

	for _, ds := range dsList {
		if err := upgradeLabelsForDaemonSet(ds, kubeClient, namespace); err != nil {
			return err
		}
	}
	return nil
}

func upgradeLabelsForDaemonSet(ds *appsv1.DaemonSet, kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "upgradeLabelsForDaemonSet failed")
	}()
	metadata, err := meta.Accessor(ds)
	if err != nil {
		return err
	}
	dsLabels := metadata.GetLabels()
	newDSLabels := upgradeutil.MergeStringMaps(dsLabels, types.GetBaseLabelsForSystemManagedComponent())
	if reflect.DeepEqual(dsLabels, newDSLabels) {
		return nil
	}
	metadata.SetLabels(newDSLabels)
	if _, err := kubeClient.AppsV1().DaemonSets(namespace).Update(context.TODO(), ds, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func upgradeCSIServicesLabels(kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade CSI services labels failed")
	}()
	for _, svName := range []string{types.CSIAttacherName, types.CSIProvisionerName, types.CSIResizerName, types.CSISnapshotterName} {
		sv, err := kubeClient.CoreV1().Services(namespace).Get(context.TODO(), svName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if err := upgradeLabelsForService(sv, kubeClient, namespace); err != nil {
			return err
		}
	}
	return nil
}

func upgradeShareManagerServicesLabels(kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade ShareManager services labels failed")
	}()
	svList, err := kubeClient.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: types.GetLonghornLabelKey(types.LonghornLabelShareManager),
	})
	if err != nil {
		return err
	}
	for _, sv := range svList.Items {
		if err := upgradeLabelsForService(&sv, kubeClient, namespace); err != nil {
			return err
		}
	}
	return nil
}

func upgradeLabelsForService(sv *v1.Service, kubeClient *clientset.Clientset, namespace string) (err error) {
	metadata, err := meta.Accessor(sv)
	if err != nil {
		return err
	}
	svLabels := metadata.GetLabels()
	newSVLabels := upgradeutil.MergeStringMaps(svLabels, types.GetBaseLabelsForSystemManagedComponent())
	if reflect.DeepEqual(svLabels, newSVLabels) {
		return nil
	}
	metadata.SetLabels(newSVLabels)
	if _, err := kubeClient.CoreV1().Services(namespace).Update(context.TODO(), sv, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func updateCSIDaemonSetsLastAppliedTolerationsAnnotation(kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"update CSI DaemonSet sLastAppliedTolerations Annotation failed")
	}()
	daemonsetList, err := kubeClient.AppsV1().DaemonSets(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.Set(types.GetBaseLabelsForSystemManagedComponent()).String(),
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list Longhorn daemonsets for toleration annotation update")
	}
	for _, ds := range daemonsetList.Items {
		dsp := &ds
		needToUpdate, err := needToUpdateTolerationAnnotation(dsp)
		if err != nil {
			return err
		}
		if !needToUpdate {
			continue
		}
		appliedTolerations := getNonDefaultTolerationList(dsp.Spec.Template.Spec.Tolerations)
		appliedTolerationsByte, err := json.Marshal(appliedTolerations)
		if err != nil {
			return err
		}
		if err := util.SetAnnotation(dsp, types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix), string(appliedTolerationsByte)); err != nil {
			return err
		}
		if _, err := kubeClient.AppsV1().DaemonSets(namespace).Update(context.TODO(), dsp, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func updateCSIDeploymentsLastAppliedTolerationsAnnotation(kubeClient *clientset.Clientset, namespace string) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"update CSI Deployments LastAppliedTolerationsAnnotation failed")
	}()
	deploymentList, err := kubeClient.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.Set(types.GetBaseLabelsForSystemManagedComponent()).String(),
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list Longhorn deployments for toleration annotation update")
	}

	for _, dp := range deploymentList.Items {
		dpp := &dp
		needToUpdate, err := needToUpdateTolerationAnnotation(dpp)
		if err != nil {
			return err
		}
		if !needToUpdate {
			continue
		}
		appliedTolerations := getNonDefaultTolerationList(dpp.Spec.Template.Spec.Tolerations)
		appliedTolerationsByte, err := json.Marshal(appliedTolerations)
		if err != nil {
			return err
		}
		if err := util.SetAnnotation(dpp, types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix), string(appliedTolerationsByte)); err != nil {
			return err
		}
		if _, err := kubeClient.AppsV1().Deployments(namespace).Update(context.TODO(), dpp, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func getNonDefaultTolerationList(tolerations []v1.Toleration) []v1.Toleration {
	result := []v1.Toleration{}
	for _, t := range tolerations {
		if !util.IsKubernetesDefaultToleration(t) {
			result = append(result, t)
		}
	}
	return result
}

func needToUpdateTolerationAnnotation(obj runtime.Object) (bool, error) {
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return false, err
	}

	annos := objMeta.GetAnnotations()
	if annos == nil {
		return true, nil
	}

	_, ok := annos[types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix)]
	if ok {
		return false, nil
	}

	return true, nil
}
