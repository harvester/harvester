package upgrade

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade/repoinfo"
	"github.com/harvester/harvester/pkg/util"
)

const (
	repoVMNamePrefix = "upgrade-repo-"
	repoVMUserData   = `name: "enable repo mode"
stages:
  rootfs:
  - commands:
    - echo > /sysroot/harvester-serve-iso
`
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

	logrus.Infof("Create upgrade repo vm with image %v and service", image)

	_, err = r.createVM(upgradeImage)
	if err != nil {
		return err
	}

	_, err = r.createService()
	return err
}

// Clean repo related resources
func (r *Repo) Cleanup() error {
	logrus.Infof("Delete upgrade repo vm, image and service")
	if err := r.deleteVM(); err != nil {
		logrus.Warnf("%s", err.Error())
		return err
	}
	if err := r.deleteService(); err != nil {
		logrus.Warnf("%s", err.Error())
		return err
	}
	return nil
}

func getISODisplayNameImageName(upgradeName string, version string) string {
	return fmt.Sprintf("%s-%s", upgradeName, version)
}

func (r *Repo) CreateImageFromISO(isoURL string, checksum string) (*harvesterv1.VirtualMachineImage, error) {
	imageSpec := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: harvesterSystemNamespace,
			Name:      r.upgrade.Name,
			Labels: map[string]string{
				harvesterUpgradeLabel: r.upgrade.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(r.upgrade),
			},
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			DisplayName: getISODisplayNameImageName(r.upgrade.Name, r.upgrade.Spec.Version),
			SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
			URL:         isoURL,
			Checksum:    checksum,
			Retry:       3,
		},
	}

	return r.h.vmImageClient.Create(imageSpec)
}

func (r *Repo) GetImage(imageName string) (*harvesterv1.VirtualMachineImage, error) {
	tokens := strings.Split(imageName, "/")
	if len(tokens) != 2 {
		return nil, fmt.Errorf("invalid image format %s", imageName)
	}

	image, err := r.h.vmImageCache.Get(tokens[0], tokens[1])
	if err != nil {
		return nil, err
	}
	return image, nil
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

func (r *Repo) createVM(image *harvesterv1.VirtualMachineImage) (*kubevirtv1.VirtualMachine, error) {
	vmName := r.getVMName()
	runStrategy := kubevirtv1.RunStrategyRerunOnFailure // the default strategy used by Harvester to create new VMs
	var bootOrder uint = 1
	evictionStrategy := kubevirtv1.EvictionStrategyLiveMigrateIfPossible

	disk0Claim := fmt.Sprintf("%s-disk-0", vmName)
	volumeMode := corev1.PersistentVolumeBlock
	storageClassName := image.Status.StorageClassName
	pvcSpec := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: disk0Claim,
			Annotations: map[string]string{
				util.AnnotationImageID: fmt.Sprintf("%s/%s", image.Namespace, image.Name),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					"storage": resource.MustParse("10Gi"),
				},
			},
			VolumeMode:       &volumeMode,
			StorageClassName: &storageClassName,
		},
	}
	pvc, err := json.Marshal([]corev1.PersistentVolumeClaim{pvcSpec})
	if err != nil {
		return nil, err
	}

	vm := kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: upgradeNamespace,
			Labels: map[string]string{
				util.LabelVMCreator:            "harvester",
				harvesterUpgradeLabel:          r.upgrade.Name,
				harvesterUpgradeComponentLabel: upgradeComponentRepo,
			},
			Annotations: map[string]string{
				util.AnnotationVolumeClaimTemplates: string(pvc),
				"network.harvesterhci.io/ips":       "[]",
				util.RemovedPVCsAnnotationKey:       disk0Claim,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(r.upgrade),
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &runStrategy,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelVMCreator:            "harvester",
						util.LabelVMName:               vmName,
						harvesterUpgradeLabel:          r.upgrade.Name,
						harvesterUpgradeComponentLabel: upgradeComponentRepo,
					},
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						CPU: &kubevirtv1.CPU{
							Cores:   1,
							Sockets: 1,
							Threads: 1,
							Model:   kubevirtv1.CPUModeHostPassthrough,
						},
						Firmware: &kubevirtv1.Firmware{
							Bootloader: &kubevirtv1.Bootloader{
								EFI: &kubevirtv1.EFI{
									SecureBoot: pointer.Bool(false),
								},
							},
						},
						Devices: kubevirtv1.Devices{
							Disks: []kubevirtv1.Disk{
								{
									BootOrder: &bootOrder,
									DiskDevice: kubevirtv1.DiskDevice{
										CDRom: &kubevirtv1.CDRomTarget{
											Bus: "scsi",
										},
									},
									Name: "disk-0",
								},
								{
									DiskDevice: kubevirtv1.DiskDevice{
										CDRom: &kubevirtv1.CDRomTarget{
											Bus: "scsi",
										},
									},
									Name: "cloudinitdisk",
								},
							},
							Inputs: []kubevirtv1.Input{
								{
									Bus:  "usb",
									Name: "tablet",
									Type: "tablet",
								},
							},
							Interfaces: []kubevirtv1.Interface{
								{
									InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
										Masquerade: &kubevirtv1.InterfaceMasquerade{},
									},
									Model: "virtio",
									Name:  "default",
								},
							},
						},
						Resources: kubevirtv1.ResourceRequirements{
							Limits: corev1.ResourceList{
								"cpu":    resource.MustParse("1"),
								"memory": resource.MustParse("1G"),
							},
							Requests: corev1.ResourceList{
								"cpu":    resource.MustParse("1"),
								"memory": resource.MustParse("1G"),
							},
						},
					},
					EvictionStrategy: &evictionStrategy,
					Hostname:         vmName,
					Networks: []kubevirtv1.Network{
						{
							Name: "default",
							NetworkSource: kubevirtv1.NetworkSource{
								Pod: &kubevirtv1.PodNetwork{},
							},
						},
					},
					Volumes: []kubevirtv1.Volume{
						{
							Name: "disk-0",
							VolumeSource: kubevirtv1.VolumeSource{
								PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: disk0Claim,
									},
								},
							},
						},
						{
							Name: "cloudinitdisk",
							VolumeSource: kubevirtv1.VolumeSource{
								CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
									UserData: repoVMUserData,
								},
							},
						},
					},
					ReadinessProbe: &kubevirtv1.Probe{
						Handler: kubevirtv1.Handler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/harvester-iso/harvester-release.yaml",
								Port: intstr.FromInt(80),
							},
						},
						TimeoutSeconds:   30,
						FailureThreshold: 5,
					},
				},
			},
		},
	}

	return r.h.vmClient.Create(&vm)
}

// (r *Repo) startVM() is done via func (h *upgradeHandler) startVM(ctx context.Context, vm *kubevirtv1.VirtualMachine)

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

	body, err := ioutil.ReadAll(resp.Body)
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
