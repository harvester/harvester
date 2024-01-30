package csi

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhornclient "github.com/longhorn/longhorn-manager/client"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhornmeta "github.com/longhorn/longhorn-manager/meta"
)

const (
	maxRetryCountForMountPropagationCheck = 10
	durationSleepForMountPropagationCheck = 5 * time.Second
	maxRetryForDeletion                   = 120
)

func getCommonDeployment(commonName, namespace, serviceAccount, image, rootDir string, args []string, replicaCount int32,
	tolerations []corev1.Toleration, tolerationsString, priorityClass, registrySecret string, imagePullPolicy corev1.PullPolicy, nodeSelector map[string]string) *appsv1.Deployment {

	deploymentLabels := types.GetBaseLabelsForSystemManagedComponent()
	deploymentLabels["app"] = commonName

	commonDeploymentSpec := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        commonName,
			Namespace:   namespace,
			Annotations: map[string]string{types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix): tolerationsString},
			Labels:      deploymentLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": commonName},
			},
			Replicas: &replicaCount,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": commonName},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccount,
					Tolerations:        tolerations,
					NodeSelector:       nodeSelector,
					PriorityClassName:  priorityClass,
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 1,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "app",
													Operator: metav1.LabelSelectorOpIn,
													Values: []string{
														commonName,
													},
												},
											},
										},

										TopologyKey: corev1.LabelHostname,
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            commonName,
							Image:           image,
							Args:            args,
							ImagePullPolicy: imagePullPolicy,
							Env: []corev1.EnvVar{
								{
									Name:  "ADDRESS",
									Value: GetInContainerCSISocketFilePath(),
								},
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "socket-dir",
									MountPath: GetInContainerCSISocketDir(),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "socket-dir",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: GetCSISocketDir(rootDir),
									Type: &HostPathDirectoryOrCreate,
								},
							},
						},
					},
				},
			},
		},
	}

	if registrySecret != "" {
		commonDeploymentSpec.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: registrySecret,
			},
		}
	}

	return commonDeploymentSpec
}

type resourceCreateFunc func(kubeClient *clientset.Clientset, obj runtime.Object) error
type resourceDeleteFunc func(kubeClient *clientset.Clientset, name, namespace string) error

func deploy(kubeClient *clientset.Clientset, obj runtime.Object, resource string,
	createFunc resourceCreateFunc, deleteFunc resourceDeleteFunc, getFunc util.ResourceGetFunc) (err error) {

	kubeVersion, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get Kubernetes server version")
	}

	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("invalid object for deploy %v: %v", obj, err)
	}
	annos := objMeta.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}
	annos[AnnotationCSIGitCommit] = longhornmeta.GitCommit
	annos[AnnotationCSIVersion] = longhornmeta.Version
	annos[AnnotationKubernetesVersion] = kubeVersion.GitVersion
	objMeta.SetAnnotations(annos)
	name := objMeta.GetName()
	namespace := objMeta.GetNamespace()

	defer func() {
		err = errors.Wrapf(err, "failed to deploy %v %v", resource, name)
	}()

	existing, err := getFunc(kubeClient, name, namespace)
	if err == nil {
		existingMeta, err := meta.Accessor(existing)
		if err != nil {
			return err
		}
		annos := objMeta.GetAnnotations()
		existingAnnos := existingMeta.GetAnnotations()
		if annos[AnnotationCSIGitCommit] == existingAnnos[AnnotationCSIGitCommit] &&
			annos[AnnotationCSIVersion] == existingAnnos[AnnotationCSIVersion] &&
			annos[AnnotationKubernetesVersion] == existingAnnos[AnnotationKubernetesVersion] &&
			existingMeta.GetDeletionTimestamp() == nil &&
			!needToUpdateImage(existing, obj) {
			// deployment of correct version already deployed
			logrus.Infof("Detected %v %v CSI Git commit %v version %v Kubernetes version %v has already been deployed",
				resource, name, annos[AnnotationCSIGitCommit], annos[AnnotationCSIVersion], annos[AnnotationKubernetesVersion])
			return nil
		}
	}
	// otherwise clean up the old deployment
	if err := cleanup(kubeClient, obj, resource, deleteFunc, getFunc); err != nil {
		return err
	}
	logrus.Infof("Creating %s %s", resource, name)
	err = createFunc(kubeClient, obj)
	return err
}

func needToUpdateImage(existingObj, newObj runtime.Object) bool {
	_, isDeploy := existingObj.(*appsv1.Deployment)
	if isDeploy {
		return needToUpdateDeployImage(existingObj, newObj)
	}

	_, isDaemonSet := existingObj.(*appsv1.DaemonSet)
	if isDaemonSet {
		return needToUpdateDaemonSetImage(existingObj, newObj)
	}

	return false
}

func needToUpdateDeployImage(existingObj, newObj runtime.Object) bool {
	existingDeployment, ok := existingObj.(*appsv1.Deployment)
	if !ok {
		return false
	}

	newDeployment, ok := newObj.(*appsv1.Deployment)
	if !ok {
		return false
	}

	existingImages := make(map[string]bool)
	for _, container := range existingDeployment.Spec.Template.Spec.Containers {
		existingImages[container.Image] = true
	}

	newImages := make(map[string]bool)
	for _, container := range newDeployment.Spec.Template.Spec.Containers {
		newImages[container.Image] = true
	}

	return !reflect.DeepEqual(existingImages, newImages)
}

func needToUpdateDaemonSetImage(existingObj, newObj runtime.Object) bool {
	existingDaemonSet, ok := existingObj.(*appsv1.DaemonSet)
	if !ok {
		return false
	}

	newDaemonSet, ok := newObj.(*appsv1.DaemonSet)
	if !ok {
		return false
	}

	existingImages := make(map[string]bool)
	for _, container := range existingDaemonSet.Spec.Template.Spec.Containers {
		existingImages[container.Image] = true
	}

	newImages := make(map[string]bool)
	for _, container := range newDaemonSet.Spec.Template.Spec.Containers {
		newImages[container.Image] = true
	}

	return !reflect.DeepEqual(existingImages, newImages)
}

func cleanup(kubeClient *clientset.Clientset, obj runtime.Object, resource string,
	deleteFunc resourceDeleteFunc, getFunc util.ResourceGetFunc) (err error) {

	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("invalid object for cleanup %v: %v", obj, err)
	}
	name := objMeta.GetName()
	namespace := objMeta.GetNamespace()

	defer func() {
		err = errors.Wrapf(err, "failed to cleanup %v %v", resource, name)
	}()

	existing, err := getFunc(kubeClient, name, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	existingMeta, err := meta.Accessor(existing)
	if err != nil {
		return err
	}
	if existingMeta.GetDeletionTimestamp() != nil {
		return util.WaitForResourceDeletion(kubeClient, name, namespace, resource, maxRetryForDeletion, getFunc)
	}
	logrus.Infof("Deleting existing %s %s", resource, name)
	if err := deleteFunc(kubeClient, name, namespace); err != nil {
		return err
	}
	return util.WaitForResourceDeletion(kubeClient, name, namespace, resource, maxRetryForDeletion, getFunc)
}

func serviceCreateFunc(kubeClient *clientset.Clientset, obj runtime.Object) error {
	o, ok := obj.(*corev1.Service)
	if !ok {
		return fmt.Errorf("failed to convert back the object")
	}
	_, err := kubeClient.CoreV1().Services(o.Namespace).Create(context.TODO(), o, metav1.CreateOptions{})
	return err
}

func serviceDeleteFunc(kubeClient *clientset.Clientset, name, namespace string) error {
	propagation := metav1.DeletePropagationForeground
	return kubeClient.CoreV1().Services(namespace).Delete(
		context.TODO(),
		name,
		metav1.DeleteOptions{PropagationPolicy: &propagation},
	)
}

func serviceGetFunc(kubeClient *clientset.Clientset, name, namespace string) (runtime.Object, error) {
	return kubeClient.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func deploymentCreateFunc(kubeClient *clientset.Clientset, obj runtime.Object) error {
	o, ok := obj.(*appsv1.Deployment)
	if !ok {
		return fmt.Errorf("failed to convert back the object")
	}
	_, err := kubeClient.AppsV1().Deployments(o.Namespace).Create(context.TODO(), o, metav1.CreateOptions{})
	return err
}

func deploymentDeleteFunc(kubeClient *clientset.Clientset, name, namespace string) error {
	propagation := metav1.DeletePropagationForeground
	return kubeClient.AppsV1().Deployments(namespace).Delete(
		context.TODO(),
		name,
		metav1.DeleteOptions{PropagationPolicy: &propagation},
	)
}

func deploymentGetFunc(kubeClient *clientset.Clientset, name, namespace string) (runtime.Object, error) {
	return kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func daemonSetCreateFunc(kubeClient *clientset.Clientset, obj runtime.Object) error {
	o, ok := obj.(*appsv1.DaemonSet)
	if !ok {
		return fmt.Errorf("failed to convert back the object")
	}
	_, err := kubeClient.AppsV1().DaemonSets(o.Namespace).Create(context.TODO(), o, metav1.CreateOptions{})
	return err
}

func daemonSetDeleteFunc(kubeClient *clientset.Clientset, name, namespace string) error {
	propagation := metav1.DeletePropagationForeground
	return kubeClient.AppsV1().DaemonSets(namespace).Delete(
		context.TODO(),
		name,
		metav1.DeleteOptions{PropagationPolicy: &propagation},
	)
}

func daemonSetGetFunc(kubeClient *clientset.Clientset, name, namespace string) (runtime.Object, error) {
	return kubeClient.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func csiDriverObjectCreateFunc(kubeClient *clientset.Clientset, obj runtime.Object) error {
	o, ok := obj.(*storagev1.CSIDriver)
	if !ok {
		return fmt.Errorf("failed to convert back the object")
	}
	_, err := kubeClient.StorageV1().CSIDrivers().Create(context.TODO(), o, metav1.CreateOptions{})
	return err
}

func csiDriverObjectDeleteFunc(kubeClient *clientset.Clientset, name, namespace string) error {
	return kubeClient.StorageV1().CSIDrivers().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func csiDriverObjectGetFunc(kubeClient *clientset.Clientset, name, namespace string) (runtime.Object, error) {
	return kubeClient.StorageV1().CSIDrivers().Get(context.TODO(), name, metav1.GetOptions{})
}

// CheckMountPropagationWithNode https://github.com/kubernetes/kubernetes/issues/66086#issuecomment-404346854
func CheckMountPropagationWithNode(managerURL string) error {
	clientOpts := &longhornclient.ClientOpts{Url: managerURL}
	apiClient, err := longhornclient.NewRancherClient(clientOpts)
	if err != nil {
		return err
	}
	nodeCollection, err := apiClient.Node.List(&longhornclient.ListOpts{})
	for _, node := range nodeCollection.Data {
		con := node.Conditions[string(longhorn.NodeConditionTypeMountPropagation)]
		var condition map[string]interface{}
		if con != nil {
			condition = con.(map[string]interface{})
		}
		for i := 0; i < maxRetryCountForMountPropagationCheck; i++ {
			if condition != nil && condition["status"] != nil && condition["status"].(string) != string(longhorn.ConditionStatusUnknown) {
				break
			}
			time.Sleep(durationSleepForMountPropagationCheck)
			retryNode, err := apiClient.Node.ById(node.Name)
			if err != nil {
				return err
			}
			if retryNode.Conditions[string(longhorn.NodeConditionTypeMountPropagation)] != nil {
				condition = retryNode.Conditions[string(longhorn.NodeConditionTypeMountPropagation)].(map[string]interface{})
			}
		}
		if condition == nil || condition["status"] == nil || condition["status"].(string) != string(longhorn.ConditionStatusTrue) {
			return fmt.Errorf("node %s is not support mount propagation", node.Name)
		}
	}

	return nil
}

func GetInContainerCSISocketDir() string {
	return DefaultInContainerCSISocketDir
}

func GetInContainerCSISocketFilePath() string {
	return filepath.Join(GetInContainerCSISocketDir(), DefaultCSISocketFileName)
}

func GetInContainerCSIRegistrationDir() string {
	return DefaultInContainerCSIRegistrationDir
}

func GetCSIPodsDir(kubeletRootDir string) string {
	return filepath.Join(kubeletRootDir, "/pods")
}

func GetCSIKubernetesDir(kubeletRootDir string) string {
	return filepath.Join(GetCSIPluginsDir(kubeletRootDir), DefaultKubernetesCSIDirSuffix)
}

func GetCSISocketDir(kubeletRootDir string) string {
	return filepath.Join(GetCSIPluginsDir(kubeletRootDir), types.LonghornDriverName)
}

func GetCSISocketFilePath(kubeletRootDir string) string {
	return filepath.Join(GetCSISocketDir(kubeletRootDir), DefaultCSISocketFileName)
}

func GetCSIRegistrationDir(kubeletRootDir string) string {
	return filepath.Join(kubeletRootDir, DefaultCSIRegistrationDirSuffix)
}

func GetCSIPluginsDir(kubeletRootDir string) string {
	return filepath.Join(kubeletRootDir, DefaultCSIPluginsDirSuffix)
}

func GetCSIEndpoint() string {
	return "unix://" + GetInContainerCSISocketFilePath()
}
