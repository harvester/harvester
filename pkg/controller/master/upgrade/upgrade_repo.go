package upgrade

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade/repoinfo"
	"github.com/harvester/harvester/pkg/util"
)

const (
	repoVMNamePrefix      = "upgrade-repo-"
	repoServiceNamePrefix = "upgrade-repo-"
	currentVersion        = "current"
)

var (
	// Images in the list will be retained during the image pruning process at
	// the end of an upgrade.
	imageRetainList = []string{
		"rancher/harvester-upgrade",
		"longhornio/longhorn-engine",
		"longhornio/longhorn-instance-manager",
		"rancher/mirrored-banzaicloud-fluentd",
		"rancher/mirrored-fluent-fluent-bit",
	}
)

type Repo struct {
	ctx     context.Context
	upgrade *harvesterv1.Upgrade
	h       *upgradeHandler

	httpClient *http.Client
}

func NewUpgradeRepo(ctx context.Context, upgrade *harvesterv1.Upgrade, upgradeHandler *upgradeHandler) *Repo {
	return &Repo{
		ctx:     ctx,
		upgrade: upgrade,
		h:       upgradeHandler,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (r *Repo) Bootstrap() error {
	image := r.upgrade.Status.ImageID
	if image == "" {
		return errors.New("upgrade repo image is not provided")
	}
	upgradeImage, err := r.GetImage(image)
	if err != nil {
		return err
	}

	logrus.Infof("Create upgrade repo with image %v and service", image)

	_, err = r.CreateDeployment(upgradeImage)
	if err != nil {
		return err
	}

	_, err = r.createService()
	return err
}

// Clean repo related resources
func (r *Repo) Cleanup() error {
	logrus.Infof("Delete upgrade related resources")
	if err := r.deleteVM(); err != nil {
		logrus.Warnf("%s", err.Error())
		return err
	}
	if err := r.deleteDeployment(); err != nil {
		logrus.Warnf("%s", err.Error())
		return err
	}
	if err := r.deleteService(); err != nil {
		logrus.Warnf("%s", err.Error())
		return err
	}
	if err := r.deleteStorageClass(); err != nil {
		logrus.Warnf("%s", err.Error())
		return err
	}
	return nil
}

func getISODisplayNameImageName(upgradeName string, version string) string {
	return fmt.Sprintf("%s-%s", upgradeName, version)
}

func (r *Repo) getStorageClassName() string {
	return r.upgrade.Name
}

// CreateStorageClass create the storage class for repo deployment to store the ISO image
// we cannot use "harvester-longhorn" storage class because it includes "migratable: true",
// which is not supported for RWX volume.
func (r *Repo) CreateStorageClass() error {
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	volumeBindingMode := storagev1.VolumeBindingImmediate
	allowVolumeExpansion := true
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.upgrade.Name,
			Labels: map[string]string{
				harvesterUpgradeLabel: r.upgrade.Name,
			},
		},
		Provisioner:          util.CSIProvisionerLonghorn,
		ReclaimPolicy:        &reclaimPolicy,
		VolumeBindingMode:    &volumeBindingMode,
		AllowVolumeExpansion: &allowVolumeExpansion,
	}

	_, err := r.h.scClient.Create(sc)
	return err
}

func (r *Repo) deleteStorageClass() error {
	scName := r.getStorageClassName()
	err := r.h.scClient.Delete(scName, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete storage class %s error %w", scName, err)
	}
	return nil
}

func (r *Repo) CreateImageFromISO(isoURL string, checksum string) (*harvesterv1.VirtualMachineImage, error) {
	imageSpec := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: harvesterSystemNamespace,
			Name:      r.upgrade.Name,
			Labels: map[string]string{
				harvesterUpgradeLabel: r.upgrade.Name,
			},
			Annotations: map[string]string{
				util.AnnotationUpgradeImage: "True",
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(r.upgrade),
			},
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			DisplayName:            getISODisplayNameImageName(r.upgrade.Name, r.upgrade.Spec.Version),
			SourceType:             harvesterv1.VirtualMachineImageSourceTypeDownload,
			URL:                    isoURL,
			Checksum:               checksum,
			Retry:                  3,
			TargetStorageClassName: r.getStorageClassName(),
			Backend:                harvesterv1.VMIBackendCDI,
		},
	}

	return r.h.vmImageClient.Create(imageSpec)
}

func (r *Repo) GetImage(imageName string) (*harvesterv1.VirtualMachineImage, error) {
	return util.GetUpgradeImage(imageName, r.h.vmImageCache)
}

func (r *Repo) getVMName() string {
	return fmt.Sprintf("%s%s", repoVMNamePrefix, r.upgrade.Name)
}

func (r *Repo) GetVMName() string {
	return r.getVMName()
}

func (r *Repo) GetVMNamespace() string {
	return upgradeNamespace
}

func (r *Repo) deleteVM() error {
	vmName := r.getVMName()

	vm, err := r.h.vmCache.Get(upgradeNamespace, vmName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.deleteImage("")
		}
		return fmt.Errorf("failed to get repo vm %s/%s error %w", vm.Namespace, vm.Name, err)
	}

	if pvcName, ok := vm.Annotations[util.RemovedPVCsAnnotationKey]; ok {
		if err := r.deleteImage(pvcName); err != nil {
			return err
		}
	}

	err = r.h.vmClient.Delete(vm.Namespace, vm.Name, &metav1.DeleteOptions{})
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		return nil
	}
	return fmt.Errorf("failed to delete repo vm %s/%s error %w", vm.Namespace, vm.Name, err)
}

func (r *Repo) deleteService() error {
	nm := r.getRepoServiceName()
	// no serviceCache, delete via client directly
	err := r.h.serviceClient.Delete(upgradeNamespace, nm, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete service %s/%s error %w", upgradeNamespace, nm, err)
	}
	return nil
}

// deleteImage deletes a repo image
// (1) If a PVC based on the image exists, it adds the PVC as the OwnerReference to the repo image.
// Once the repo VM is deleted and VM's PVC is deleted, the repo image will be garbage collected.
// (2) If there is no PVCs based on the repo image. We delete the image directly.
func (r *Repo) deleteImage(pvcName string) error {
	imageID := r.upgrade.Status.ImageID
	if imageID == "" {
		logrus.Warnf("Upgrade repo image is not provided on upgrade status, skip")
		return nil
	}

	image, err := r.GetImage(imageID)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if pvcName == "" {
		logrus.Infof("Delete upgrade repo image %s", imageID)
		return r.h.vmImageClient.Delete(image.Namespace, image.Name, &metav1.DeleteOptions{})
	}

	pvc, err := r.h.pvcClient.Get(upgradeNamespace, pvcName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("Delete upgrade repo image %s", imageID)
			return r.h.vmImageClient.Delete(image.Namespace, image.Name, &metav1.DeleteOptions{})
		}
		return err
	}

	logrus.Infof("Add OwnerReference %s to upgrade repo image %s", pvcName, imageID)
	toUpdate := image.DeepCopy()
	toUpdate.OwnerReferences = []metav1.OwnerReference{
		{
			Name:       pvc.Name,
			Kind:       pvc.Kind,
			UID:        pvc.UID,
			APIVersion: pvc.APIVersion,
		},
	}

	_, err = r.h.vmImageClient.Update(toUpdate)
	return err
}

func (r *Repo) getRepoServiceName() string {
	return fmt.Sprintf("%s%s", repoServiceNamePrefix, r.upgrade.Name)
}

func (r *Repo) createService() (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: upgradeNamespace,
			Name:      r.getRepoServiceName(),
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(r.upgrade),
			},
			Labels: map[string]string{
				util.LabelVMCreator:            "harvester",
				harvesterUpgradeLabel:          r.upgrade.Name,
				harvesterUpgradeComponentLabel: upgradeComponentRepo,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				harvesterUpgradeLabel:          r.upgrade.Name,
				harvesterUpgradeComponentLabel: upgradeComponentRepo,
			},
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(80),
				},
			},
		},
	}

	return r.h.serviceClient.Create(&service)
}

func (r *Repo) getInfo() (*repoinfo.RepoInfo, error) {
	releaseURL := fmt.Sprintf("http://%s.%s/harvester-iso/harvester-release.yaml", r.getRepoServiceName(), upgradeNamespace)

	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	info := repoinfo.RepoInfo{}
	err = yaml.Unmarshal(body, &info.Release)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (r *Repo) getImageList(version string, imageList map[string]bool) error {
	imageListURL := fmt.Sprintf("http://%s.%s/harvester-iso/bundle/harvester/images-lists-archive/%s/image_list_all.txt",
		r.getRepoServiceName(),
		upgradeNamespace,
		version,
	)

	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, imageListURL, nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		imageName := scanner.Text()
		imageList[imageName] = true
	}

	return scanner.Err()
}

func applyImageRetainList(imageList []string) []string {
	for _, retainedImage := range imageRetainList {
		for i, image := range imageList {
			if strings.Contains(image, retainedImage) {
				imageList = removeItemFromSlice(imageList, i)
				break
			}
		}
	}
	return imageList
}

func (r *Repo) getImagesDiffList() ([]string, error) {
	previousImageList := make(map[string]bool)
	currentImageList := make(map[string]bool)

	backoff := wait.Backoff{
		Steps:    30,
		Duration: 10 * time.Second,
		Factor:   1.0,
		Jitter:   0.1,
	}

	if err := retry.OnError(backoff, util.IsRetriableNetworkError, func() error {
		logrus.Infof("Trying to get %s image list", r.upgrade.Status.PreviousVersion)
		err := r.getImageList(r.upgrade.Status.PreviousVersion, previousImageList)
		if err != nil {
			return err
		}
		logrus.Infof("Trying to get %s image list", currentVersion)
		return r.getImageList(currentVersion, currentImageList)
	}); err != nil {
		return nil, err
	}

	diffList := applyImageRetainList(difference(previousImageList, currentImageList))
	logrus.Infof("Diff: %v", diffList)

	return diffList, nil
}

func (r *Repo) CreateDeployment(vmImage *harvesterv1.VirtualMachineImage) (*appsv1.Deployment, error) {
	replicas, err := r.getDeploymentReplicaCount()
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.GetDeploymentName(),
			Namespace: upgradeNamespace,
			Labels: map[string]string{
				harvesterUpgradeLabel:          r.upgrade.Name,
				harvesterUpgradeComponentLabel: upgradeComponentRepo,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(r.upgrade),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					harvesterUpgradeLabel:          r.upgrade.Name,
					harvesterUpgradeComponentLabel: upgradeComponentRepo,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						harvesterUpgradeLabel:          r.upgrade.Name,
						harvesterUpgradeComponentLabel: upgradeComponentRepo,
					},
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												harvesterUpgradeLabel:          r.upgrade.Name,
												harvesterUpgradeComponentLabel: upgradeComponentRepo,
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
							Name: "nginx-iso-server",
							// The newer rancher/harvester-upgrade image hasn’t been imported yet, so we’re still using the old one.
							// what we need is just the nginx server to serve the ISO, so it’s fine to use the previous version here.
							Image:           fmt.Sprintf("%s:%s", util.HarvesterUpgradeImageRepository, r.upgrade.Status.PreviousVersion),
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"sh", "-c"},
							Args: []string{
								`echo "Mounting ISO and starting Nginx..."
mkdir -p /srv/www/htdocs/harvester-iso
mount -o loop,ro /iso/disk.img /srv/www/htdocs/harvester-iso
echo "iso mounted successfully to /srv/www/htdocs/harvester-iso"
nginx -g "daemon off;"`,
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptr.To(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "iso",
									MountPath: "/iso",
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"sh", "-c",
											"cat /srv/www/htdocs/harvester-iso/harvester-release.yaml 2>&1 /dev/null",
										},
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/harvester-iso/harvester-release.yaml",
										Port:   intstr.FromInt(80),
										Scheme: corev1.URISchemeHTTP,
									},
								},
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
					DNSPolicy: corev1.DNSClusterFirst,
					Volumes: []corev1.Volume{
						{
							Name: "iso",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									// the pvc name is the same as the vm image name
									ClaimName: vmImage.Name,
								},
							},
						},
					},
				},
			},
		},
	}

	return r.h.deploymentClient.Create(deployment)
}

func (r *Repo) deleteDeployment() error {
	err := r.h.deploymentClient.Delete(r.GetVMNamespace(), r.GetDeploymentName(), &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment %s/%s error %w", r.GetVMNamespace(), r.GetDeploymentName(), err)
	}
	return nil
}

func (r *Repo) GetDeploymentName() string {
	return fmt.Sprintf("%s%s", repoServiceNamePrefix, r.upgrade.Name)
}

func (r *Repo) getDeploymentReplicaCount() (*int32, error) {
	nodes, err := r.h.nodeCache.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	nonWitnessNodes := util.ExcludeWitnessNodes(nodes)
	if len(nonWitnessNodes) == 0 {
		return nil, errors.New("no non-witness nodes found in the cluster")
	}

	var replicas int32
	if len(nonWitnessNodes) == 1 {
		replicas = 1
	} else {
		replicas = 2
	}

	return &replicas, nil
}
