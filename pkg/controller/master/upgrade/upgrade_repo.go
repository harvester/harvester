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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	isoVolumeName = "iso"
	isoMountPath  = "/srv/www/htdocs/harvester-iso"
)

var errGetExistingRepoResource = errors.New("failed to get existing repo resource")

// Images in the list will be retained during the image pruning process at
// the end of an upgrade.
var imageRetainList = []string{
	// Matches both rancher/harvester-upgrade and <registry>/harvester/harvester-upgrade.
	"/harvester-upgrade:",
	"longhornio/longhorn-engine",
	"longhornio/longhorn-instance-manager",
	"rancher/mirrored-banzaicloud-fluentd",
	"rancher/mirrored-fluent-fluent-bit",
}

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
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	if apierrors.IsAlreadyExists(err) {
		deployment, err := r.h.deploymentClient.Get(upgradeNamespace, r.GetDeploymentName(), metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("%w: deployment %s/%s: %w", errGetExistingRepoResource, upgradeNamespace, r.GetDeploymentName(), err)
		}
		if err := r.validateUpgradeOwner(deployment); err != nil {
			return fmt.Errorf("invalid existing repo deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
		}
	}

	// Ensure the Service is created even if the Deployment already exists.
	_, err = r.createService()
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	if apierrors.IsAlreadyExists(err) {
		service, err := r.h.serviceClient.Get(upgradeNamespace, r.getRepoServiceName(), metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("%w: service %s/%s: %w", errGetExistingRepoResource, upgradeNamespace, r.getRepoServiceName(), err)
		}
		if err := r.validateUpgradeOwner(service); err != nil {
			return fmt.Errorf("invalid existing repo service %s/%s: %w", service.Namespace, service.Name, err)
		}
	}
	return nil
}

func (r *Repo) validateUpgradeOwner(obj metav1.Object) error {
	if r.hasUpgradeOwner(obj) {
		return nil
	}
	return fmt.Errorf("missing owner reference to upgrade %s/%s with UID %s", r.upgrade.Namespace, r.upgrade.Name, r.upgrade.UID)
}

func (r *Repo) hasUpgradeOwner(obj metav1.Object) bool {
	// Upgrade owner references do not set the Controller flag.
	expectedOwner := upgradeReference(r.upgrade)
	for _, owner := range obj.GetOwnerReferences() {
		if owner.UID == expectedOwner.UID && owner.Name == expectedOwner.Name &&
			owner.Kind == expectedOwner.Kind && owner.APIVersion == expectedOwner.APIVersion {
			return true
		}
	}
	return false
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
	// Cleanup legacy storage class from v1.7.0.
	// In v1.7.0, a temporary storage class was created for upgrade VM image.
	// From v1.7.1 onwards, the upgrade-named storage class is no longer created.
	// From v1.10 onwards, the upgrade repo PVC uses the cluster default StorageClass.
	// This cleanup can be removed once most users have migrated past v1.7.x.
	if err := r.deleteStorageClass(); err != nil {
		logrus.Warnf("%s", err.Error())
		return err
	}
	return nil
}

func getISODisplayNameImageName(upgradeName string, version string) string {
	return fmt.Sprintf("%s-%s", upgradeName, version)
}

// Deprecated: deleteStorageClass deletes the temporary storage class created in v1.7.0 for upgrade VM image.
func (r *Repo) deleteStorageClass() error {
	scName := r.upgrade.Name
	sc, err := r.h.scClient.Get(scName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get storage class %s: %w", scName, err)
	}
	// Only delete if this SC was created by the v1.7.0 upgrade flow (identified by its upgrade label).
	if sc.Labels[harvesterUpgradeLabel] != r.upgrade.Name {
		return nil
	}
	err = r.h.scClient.Delete(scName, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete storage class %s: %w", scName, err)
	}
	return nil
}

// repoStorageClassName returns the StorageClass for the upgrade repo PVC.
// shortcut: annotation only, no spec field; revisit when UpgradePlan (v2) CRD is introduced
func (r *Repo) repoStorageClassName() string {
	return r.upgrade.Annotations[util.AnnotationUpgradeStorageClass]
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
			Backend:                harvesterv1.VMIBackendCDI,
			TargetStorageClassName: r.repoStorageClassName(),
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

	// A terminating VM can still carry the PVC annotation needed for the image
	// ownership handoff above. Only skip the duplicate VM deletion.
	if vm.DeletionTimestamp != nil {
		return nil
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
	service, err := r.h.serviceClient.Get(upgradeNamespace, nm, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get service %s/%s for cleanup: %w", upgradeNamespace, nm, err)
	}
	if service.DeletionTimestamp != nil {
		return nil
	}
	if !r.hasUpgradeOwner(service) {
		logrus.Warnf("Skip deleting service %s/%s because it is not owned by upgrade %s/%s", upgradeNamespace, nm, r.upgrade.Namespace, r.upgrade.Name)
		return nil
	}
	err = r.h.serviceClient.Delete(upgradeNamespace, nm, &metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &service.UID, ResourceVersion: &service.ResourceVersion},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete service %s/%s error %w", upgradeNamespace, nm, err)
	}
	return nil
}

// deleteImage deletes a repo image
// (1) If a PVC based on the image exists, it adds the PVC as the OwnerReference to the repo image.
// Once the repo VM is deleted and VM's PVC is deleted, the repo image will be garbage collected.
// (2) Without a PVC, delete the image directly only if the current Upgrade owns it.
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
		return r.deleteImageWithoutPVC(image)
	}

	pvc, err := r.h.pvcClient.Get(upgradeNamespace, pvcName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.deleteImageWithoutPVC(image)
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

func (r *Repo) deleteImageWithoutPVC(image *harvesterv1.VirtualMachineImage) error {
	// Cleanup can run again after ownership has been handed off to the VM's PVC.
	if !r.hasUpgradeOwner(image) {
		logrus.Warnf("Skip deleting repo image %s/%s because it is not owned by upgrade %s/%s", image.Namespace, image.Name, r.upgrade.Namespace, r.upgrade.Name)
		return nil
	}
	logrus.Infof("Delete upgrade repo image %s/%s", image.Namespace, image.Name)
	return r.h.vmImageClient.Delete(image.Namespace, image.Name, &metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &image.UID, ResourceVersion: &image.ResourceVersion},
	})
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

// applyISOMount configures c to mount the upgrade ISO from a PVC of the given volume mode.
// Filesystem: CDI writes the ISO as disk.img, mounted via loop device.
// Block: CDI writes the raw ISO directly; mounted as iso9660.
func applyISOMount(c *corev1.Container, mode corev1.PersistentVolumeMode) {
	c.Command = []string{"sh", "-c"}
	switch mode {
	case corev1.PersistentVolumeBlock:
		c.VolumeDevices = []corev1.VolumeDevice{{Name: isoVolumeName, DevicePath: "/dev/iso"}}
		c.Args = []string{`set -e
mkdir -p /srv/www/htdocs/harvester-iso
mount -t iso9660 -o ro /dev/iso /srv/www/htdocs/harvester-iso
exec nginx -g 'daemon off;'`}
	default: // Filesystem: CDI writes disk.img
		c.VolumeMounts = []corev1.VolumeMount{{Name: isoVolumeName, MountPath: "/iso"}}
		c.Args = []string{`set -e
mkdir -p /srv/www/htdocs/harvester-iso
mount -t iso9660 -o loop,ro /iso/disk.img /srv/www/htdocs/harvester-iso
exec nginx -g 'daemon off;'`}
	}
}

func (r *Repo) CreateDeployment(vmImage *harvesterv1.VirtualMachineImage) (*appsv1.Deployment, error) {
	pvc, err := r.h.pvcClient.Get(upgradeNamespace, vmImage.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get upgrade repo PVC %s: %w", vmImage.Name, err)
	}
	// VolumeMode defaults to Filesystem when nil, matching the Kubernetes API server default.
	volumeMode := corev1.PersistentVolumeFilesystem
	if pvc.Spec.VolumeMode != nil {
		volumeMode = *pvc.Spec.VolumeMode
	}
	return r.h.deploymentClient.Create(r.repoDeployment(vmImage.Name, volumeMode))
}

// repoContainer builds the nginx-iso-server container for the upgrade repo deployment.
// The source-version upgrade image is used because the target image hasn't been imported yet.
func repoContainer(image string, mode corev1.PersistentVolumeMode) corev1.Container {
	c := corev1.Container{
		Name:            "nginx-iso-server",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{ContainerPort: 80, Protocol: corev1.ProtocolTCP},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: ptr.To(true),
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"sh", "-c", "cat " + isoMountPath + "/harvester-release.yaml >/dev/null 2>&1"},
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
	}
	applyISOMount(&c, mode)
	return c
}

func (r *Repo) repoDeployment(pvcName string, mode corev1.PersistentVolumeMode) *appsv1.Deployment {
	labels := map[string]string{
		harvesterUpgradeLabel:          r.upgrade.Name,
		harvesterUpgradeComponentLabel: upgradeComponentRepo,
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            r.GetDeploymentName(),
			Namespace:       upgradeNamespace,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{upgradeReference(r.upgrade)},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{repoContainer(util.GetPreviousUpgradeImage(r.upgrade), mode)},
					DNSPolicy:  corev1.DNSClusterFirst,
					Volumes: []corev1.Volume{{
						Name: isoVolumeName,
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: pvcName,
							},
						},
					}},
				},
			},
		},
	}
}

func (r *Repo) deleteDeployment() error {
	namespace := r.GetVMNamespace()
	name := r.GetDeploymentName()
	deployment, err := r.h.deploymentClient.Get(namespace, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get deployment %s/%s for cleanup: %w", namespace, name, err)
	}
	if deployment.DeletionTimestamp != nil {
		return nil
	}
	if !r.hasUpgradeOwner(deployment) {
		logrus.Warnf("Skip deleting deployment %s/%s because it is not owned by upgrade %s/%s", namespace, name, r.upgrade.Namespace, r.upgrade.Name)
		return nil
	}
	err = r.h.deploymentClient.Delete(namespace, name, &metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &deployment.UID, ResourceVersion: &deployment.ResourceVersion},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment %s/%s error %w", r.GetVMNamespace(), r.GetDeploymentName(), err)
	}
	return nil
}

func (r *Repo) GetDeploymentName() string {
	return repoDeploymentName(r.upgrade.Name)
}

func repoDeploymentName(upgradeName string) string {
	return repoServiceNamePrefix + upgradeName
}
