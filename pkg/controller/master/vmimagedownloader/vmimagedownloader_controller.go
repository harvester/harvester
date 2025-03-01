package vmimagedownloader

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	ctlappsv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	utilHelm "github.com/harvester/harvester/pkg/util/helm"
)

var (
	kindDeployment = "Deployment"
	boolTrue       = true
)

// storageProfileHandler dynamically manages storage profiles
type vmImageDownloaderHandler struct {
	clientSet                   *kubernetes.Clientset
	vmImageClient               v1beta1.VirtualMachineImageClient
	deploymentClient            ctlappsv1.DeploymentClient
	vmImageDownloaders          v1beta1.VirtualMachineImageDownloaderClient
	vmImageDownloaderController v1beta1.VirtualMachineImageDownloaderController
}

func (h *vmImageDownloaderHandler) OnChanged(_ string, downloader *harvesterv1.VirtualMachineImageDownloader) (*harvesterv1.VirtualMachineImageDownloader, error) {
	if downloader == nil || downloader.DeletionTimestamp != nil {
		return downloader, nil
	}

	// check vm image really here
	vmImage, err := h.vmImageClient.Get(downloader.GetNamespace(), downloader.Spec.ImageName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("Corresponding VM Image %s not found, skip this", downloader.Spec.ImageName)
			return downloader, nil
		}
		logrus.Errorf("failed to get vm image %s: %v", downloader.Spec.ImageName, err)
		return downloader, err
	}

	deploymentName := fmt.Sprintf("%s-downloader", downloader.Name)
	// get or create deployment
	deployment, err := h.getOrCreateDownloaderDeployment(deploymentName, vmImage, downloader)
	if err != nil {
		return downloader, err
	}
	logrus.Debugf("Deployment (%s/%s), Spec.Replica: %v, Status.ReadyReplica: %v", deployment.Namespace, deployment.Name, *deployment.Spec.Replicas, deployment.Status.ReadyReplicas)
	if deployment.Status.ReadyReplicas != *deployment.Spec.Replicas {
		time.Sleep(1 * time.Second) // small jitter
		downloaderCpy := downloader.DeepCopy()
		conds := harvesterv1.VirtualMachineImageDownloaderCondition{
			Type:               harvesterv1.DownloaderCondsReconciling,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "VM Image Downloader Reconciling",
			Message:            "Waiting for the corresponding deployment to be ready",
		}
		updatedConditions := updateConds(downloaderCpy.Status.Conditions, conds)
		downloaderCpy.Status.Conditions = updatedConditions
		downloaderCpy.Status.Status = harvesterv1.ImageDownloaderStatusProgressing
		if !reflect.DeepEqual(downloader, downloaderCpy) {
			return h.vmImageDownloaders.UpdateStatus(downloaderCpy)
		}
		return downloader, fmt.Errorf("Deployment %s is not ready, wanted replicas: %d, ready replicas: %d", deployment.Name, deployment.Spec.Replicas, deployment.Status.ReadyReplicas)
	}

	serviceTemplate := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: deploymentName,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         corev1.SchemeGroupVersion.String(),
					Kind:               kindDeployment,
					Name:               deployment.Name,
					UID:                deployment.GetUID(),
					BlockOwnerDeletion: &boolTrue,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": deploymentName,
			},
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(80),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
	if _, err := h.clientSet.CoreV1().Services(downloader.Namespace).Create(context.TODO(), serviceTemplate, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return downloader, fmt.Errorf("failed to create the download server service with VM Image(%s): %v", downloader.Name, err)
	}
	downloadURL := fmt.Sprintf("http://%s.%s/images/%s.%s", deploymentName, downloader.Namespace, vmImage.Name, downloader.Spec.CompressType)
	downloaderCpy := downloader.DeepCopy()
	conds := harvesterv1.VirtualMachineImageDownloaderCondition{
		Type:               harvesterv1.DownloaderCondsReady,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "VM Image Downloader Ready",
		Message:            "The corresponding deployment and service are ready",
	}
	updatedConditions := updateConds(downloaderCpy.Status.Conditions, conds)
	downloaderCpy.Status.Conditions = updatedConditions
	downloaderCpy.Status.Status = harvesterv1.ImageDownloaderStatusReady
	downloaderCpy.Status.DownloadURL = downloadURL
	if !reflect.DeepEqual(downloader, downloaderCpy) {
		return h.vmImageDownloaders.UpdateStatus(downloaderCpy)
	}
	return downloader, nil
}

func (h *vmImageDownloaderHandler) OnRemoved(_ string, downloader *harvesterv1.VirtualMachineImageDownloader) (*harvesterv1.VirtualMachineImageDownloader, error) {
	if downloader == nil {
		return nil, nil
	}

	deploymentName := fmt.Sprintf("%s-downloader", downloader.Name)
	if err := h.deploymentClient.Delete(downloader.Namespace, deploymentName, &metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("The corresponding deployment %s not found, skip this", deploymentName)
		} else {
			return downloader, fmt.Errorf("failed to delete deployment %s: %v", deploymentName, err)
		}
	}

	if err := h.clientSet.CoreV1().Services(downloader.Namespace).Delete(context.TODO(), deploymentName, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("The corresponding service %s not found, skip this", deploymentName)
		} else {
			return downloader, fmt.Errorf("failed to delete service %s: %v", deploymentName, err)
		}
	}
	return downloader, nil
}

func (h *vmImageDownloaderHandler) getOrCreateDownloaderDeployment(deploymentName string, vmImage *harvesterv1.VirtualMachineImage, downloader *harvesterv1.VirtualMachineImageDownloader) (*appsv1.Deployment, error) {
	deployment, err := h.deploymentClient.Get(downloader.Namespace, deploymentName, metav1.GetOptions{})
	if err != nil {
		logrus.Infof("Failed to get deployment %s: %v", deploymentName, err)
		if apierrors.IsNotFound(err) {
			return h.createDownloaderDeployment(deploymentName, vmImage, downloader)
		}
		return nil, fmt.Errorf("failed to get deployment %s: %v", deploymentName, err)
	}
	return deployment, nil
}

func (h *vmImageDownloaderHandler) createDownloaderDeployment(deploymentName string, vmImage *harvesterv1.VirtualMachineImage, downloader *harvesterv1.VirtualMachineImageDownloader) (*appsv1.Deployment, error) {
	convertCmd := fmt.Sprintf("qemu-img convert -t none -T none -W -m 8 -f raw /tmp/image-vol -O qcow2 -c -S 4K /image-dir/%s.qcow2", vmImage.Name)
	virtImageStr := h.getVirtHandlerImage()
	clusterRepoImageStr := h.getClusterRepoImage()

	replicaNum := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: downloader.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicaNum, // Start with 1 replica
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": deploymentName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": deploymentName,
					},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "image-vol",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: vmImage.Name,
								},
							},
						},
						{
							Name: "image-dir",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:            "image-coverter",
							Image:           virtImageStr,
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeDevices: []corev1.VolumeDevice{
								{
									Name:       "image-vol",
									DevicePath: "/tmp/image-vol",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "image-dir",
									MountPath: "/image-dir",
								},
							},
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{convertCmd},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "image-exporter",
							Image:           clusterRepoImageStr,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "image-dir",
									MountPath: "/srv/www/htdocs/images/",
								},
							},
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{"nginx -g 'daemon off;' && while true; do sleep 3600; done"},
						},
					},
				},
			},
		},
	}
	return h.deploymentClient.Create(deployment)
}

func (h *vmImageDownloaderHandler) getVirtHandlerImage() string {
	image, err := utilHelm.FetchImageFromHelmValues(h.clientSet,
		util.HarvesterSystemNamespaceName,
		util.HarvesterChartReleaseName,
		[]string{"kubevirt-operator", "containers", "handler", "image"})
	if err != nil {
		return ""
	}
	targetImage := fmt.Sprintf("%s:%s", image.Repository, image.Tag)
	return targetImage
}

func (h *vmImageDownloaderHandler) getClusterRepoImage() string {
	deployContent, err := h.clientSet.AppsV1().Deployments("cattle-system").Get(context.TODO(), "harvester-cluster-repo", metav1.GetOptions{})
	if err != nil {
		logrus.Errorf("Failed to get the harvester-cluster-repo deployment: %v", err)
		return ""
	}

	// ensure the harvester-cluster-repo deployment has only one container
	containerItem := deployContent.Spec.Template.Spec.Containers[0]
	if strings.HasPrefix(containerItem.Image, "rancher/harvester-cluster-repo") {
		return containerItem.Image
	}
	logrus.Errorf("Failed to get the harvester-cluster-repo Image: %v", containerItem.Image)
	return ""
}

func updateConds(curConds []harvesterv1.VirtualMachineImageDownloaderCondition, c harvesterv1.VirtualMachineImageDownloaderCondition) []harvesterv1.VirtualMachineImageDownloaderCondition {
	found := false
	var pod = 0
	for id, cond := range curConds {
		if cond.Type == c.Type {
			found = true
			pod = id
			break
		}
	}

	if found {
		curConds[pod] = c
	} else {
		curConds = append(curConds, c)
	}
	return curConds

}
