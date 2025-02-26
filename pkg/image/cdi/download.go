package cdi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gocommon "github.com/harvester/go-common/common"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	utilHelm "github.com/harvester/harvester/pkg/util/helm"
)

type Downloader struct {
	httpClient http.Client
	clientSet  *kubernetes.Clientset
	vmio       common.VMIOperator
}

func GetDownloader(httpClient http.Client, clientSet *kubernetes.Clientset, vmio common.VMIOperator) backend.Downloader {
	return &Downloader{
		httpClient: httpClient,
		clientSet:  clientSet,
		vmio:       vmio,
	}
}

func (cd *Downloader) Do(vmImg *harvesterv1.VirtualMachineImage, rw http.ResponseWriter, req *http.Request) error {
	vmImgName := cd.vmio.GetName(vmImg)
	vmImgNamespace := cd.vmio.GetNamespace(vmImg)
	podName := fmt.Sprintf("%s-%s-%s", vmImgNamespace, vmImgName, getRandomSuffix())

	podTemplate := cd.generateDownlodServerPod(podName, vmImgName)
	if _, err := cd.clientSet.CoreV1().Pods(vmImgNamespace).Create(context.TODO(), podTemplate, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create the download server pod with VM Image(%s): %w", vmImgName, err)
	}
	defer func() {
		if err := cd.clientSet.CoreV1().Pods(vmImgNamespace).Delete(context.TODO(), podName, metav1.DeleteOptions{}); err != nil {
			logrus.Errorf("Failed to delete the download server pod(%s): %v", podName, err)
		}
	}()

	// wait pod status running
	if err := wait.PollUntilContextTimeout(context.Background(), tickPolling, tickTimeout, true, func(context.Context) (bool, error) {
		return cd.waitPodRunning(podName, vmImgNamespace)
	}); err != nil {
		return fmt.Errorf("failed to wait for VMImage status: %v", err)
	}
	pod, err := cd.clientSet.CoreV1().Pods(vmImgNamespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get the download server pod(%s): %w", podName, err)
	}

	var boolTrue = true
	serviceTemplate := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("export-%s-", vmImgName),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         corev1.SchemeGroupVersion.String(),
					Kind:               kindPod,
					Name:               pod.Name,
					UID:                pod.UID,
					BlockOwnerDeletion: &boolTrue,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": podName,
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
	service, err := cd.clientSet.CoreV1().Services(vmImgNamespace).Create(context.TODO(), serviceTemplate, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create the download server service with VM Image(%s): %w", vmImgName, err)
	}

	targetFileName := fmt.Sprintf("%s.qcow2", vmImgName)
	downloadURL := fmt.Sprintf("http://%s.%s/images/%s", service.Name, vmImgNamespace, targetFileName)

	// ensure the target file is ready
	if err := wait.PollUntilContextTimeout(context.Background(), tickPolling, tickTimeout, true, func(context.Context) (bool, error) {
		return cd.waitEndPointReady(downloadURL)
	}); err != nil {
		return fmt.Errorf("failed to wait for service ready")
	}

	downloadReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create the download request with VM Image(%s): %w", targetFileName, err)
	}

	downloadResp, err := cd.httpClient.Do(downloadReq)
	if err != nil {
		return fmt.Errorf("failed to send the download request with VM Image(%s): %w", targetFileName, err)
	}
	defer downloadResp.Body.Close()

	rw.Header().Set("Content-Disposition", "attachment; filename="+targetFileName)
	contentType := downloadResp.Header.Get("Content-Type")
	if contentType != "" {
		rw.Header().Set("Content-Type", contentType)
	}

	if _, err := io.Copy(rw, downloadResp.Body); err != nil {
		return fmt.Errorf("failed to copy download content to target(%s), err: %w", targetFileName, err)
	}

	return nil
}

func (cd *Downloader) generateDownlodServerPod(podName, vmImageName string) *corev1.Pod {
	virtImageStr := cd.getVirtHandlerImage()
	clusterRepoImageStr := cd.getClusterRepoImage()

	convertCmd := fmt.Sprintf("qemu-img convert -t none -T none -W -m 8 -f raw /tmp/image-vol -O qcow2 -c -S 4K /image-dir/%s.qcow2", vmImageName)

	targetPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				"app": podName,
			},
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "image-vol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: vmImageName,
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
					Args: []string{
						convertCmd,
					},
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
					Args: []string{
						"nginx -g 'daemon off;' && while true; do sleep 3600; done",
					},
				},
			},
		},
	}

	return targetPod
}

func (cd *Downloader) getVirtHandlerImage() string {
	image, err := utilHelm.FetchImageFromHelmValues(cd.clientSet,
		util.HarvesterSystemNamespaceName,
		util.HarvesterChartReleaseName,
		[]string{"kubevirt-operator", "containers", "handler", "image"})
	if err != nil {
		return ""
	}
	targetImage := fmt.Sprintf("%s:%s", image.Repository, image.Tag)
	return targetImage
}

func (cd *Downloader) getClusterRepoImage() string {
	deployContent, err := cd.clientSet.AppsV1().Deployments("cattle-system").Get(context.TODO(), "harvester-cluster-repo", metav1.GetOptions{})
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

func getRandomSuffix() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 5

	var result strings.Builder
	for i := 0; i < length; i++ {
		randIdx, err := gocommon.GenRandNumber(int64(len(charset)))
		if err != nil {
			// even if we fail here, give the fake random string
			logrus.Errorf("Failed to generate random number: %v", err)
			return "qwert"
		}
		result.WriteByte(charset[randIdx])
	}

	return result.String()
}

func (cd *Downloader) waitPodRunning(podName, podNs string) (bool, error) {
	pod, err := cd.clientSet.CoreV1().Pods(podNs).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		logrus.Errorf("Failed to Get the Pod(%s/%s): %v, try again", podNs, podName, err)
		return false, nil
	}

	logrus.Debugf("Pod(%s/%s) status: %s, wanted status: %s", podNs, podName, pod.Status.Phase, corev1.PodRunning)
	if pod.Status.Phase == corev1.PodRunning {
		return true, nil
	}

	return false, nil
}

func (cd *Downloader) waitEndPointReady(targetURL string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel() // Make sure to cancel the context once the function is done

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		logrus.Errorf("error creating HTTP request: %v", err)
		return false, nil
	}

	resp, err := cd.httpClient.Do(req)
	if err != nil {
		logrus.Errorf("error sending HTTP request: %v, try again.", err)
		return false, nil
	}
	defer resp.Body.Close()

	// Check if the status code is 200 OK
	logrus.Debugf("Current status code: %v, wanted: %v", resp.StatusCode, http.StatusOK)
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	return false, nil
}
