package upgrade

import (
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
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
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
)

type HarvesterRelease struct {
	Harvester            string `yaml:"harvester,omitempty"`
	HarvesterChart       string `yaml:"harvesterChart,omitempty"`
	OS                   string `yaml:"os,omitempty"`
	Kubernetes           string `yaml:"kubernetes,omitempty"`
	Rancher              string `yaml:"rancher,omitempty"`
	MonitoringChart      string `yaml:"monitoringChart,omitempty"`
	MinUpgradableVersion string `yaml:"minUpgradableVersion,omitempty"`
}

type RepoInfo struct {
	Release HarvesterRelease
}

func (info *RepoInfo) Marshall() (string, error) {
	out, err := yaml.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (info *RepoInfo) Load(data string) error {
	return yaml.Unmarshal([]byte(data), info)
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
		return errors.New("Upgrade repo image is not provided")
	}
	upgradeImage, err := r.GetImage(image)
	if err != nil {
		return err
	}

	_, err = r.createVM(upgradeImage)
	if err != nil {
		return err
	}

	_, err = r.createService()
	return err
}

func getISODisplayNameImageName(upgradeName string, version string) string {
	return fmt.Sprintf("%s-%s", upgradeName, version)
}

func (r *Repo) CreateImageFromISO(isoURL string, checksum string) (*harvesterv1.VirtualMachineImage, error) {
	imageSpec := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    harvesterSystemNamespace,
			GenerateName: "harvester-iso-",
			Labels: map[string]string{
				harvesterUpgradeLabel: r.upgrade.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(r.upgrade),
			},
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			DisplayName: getISODisplayNameImageName(r.upgrade.Name, r.upgrade.Spec.Version),
			SourceType:  v1beta1.VirtualMachineImageSourceTypeDownload,
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
		return nil, fmt.Errorf("Invalid image format %s", imageName)
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

func (r *Repo) createVM(image *harvesterv1.VirtualMachineImage) (*kubevirtv1.VirtualMachine, error) {
	vmName := r.getVMName()
	vmRun := true
	var bootOrder uint = 1
	evictionStrategy := kubevirtv1.EvictionStrategyLiveMigrate

	disk0Claim := fmt.Sprintf("%s-disk-0", vmName)
	volumeMode := corev1.PersistentVolumeBlock
	storageClassName := image.Status.StorageClassName
	pvcSpec := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: disk0Claim,
			Annotations: map[string]string{
				"harvesterhci.io/imageId": fmt.Sprintf("%s/%s", image.Namespace, image.Name),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.ResourceRequirements{
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
				"harvesterhci.io/creator":      "harvester",
				harvesterUpgradeLabel:          r.upgrade.Name,
				harvesterUpgradeComponentLabel: upgradeComponentRepo,
			},
			Annotations: map[string]string{
				"harvesterhci.io/volumeClaimTemplates": string(pvc),
				"network.harvesterhci.io/ips":          "[]",
				util.RemovedPVCsAnnotationKey:          disk0Claim,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(r.upgrade),
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Running: &vmRun,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"harvesterhci.io/creator":      "harvester",
						"harvesterhci.io/vmName":       vmName,
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
						},
						Devices: kubevirtv1.Devices{
							Disks: []kubevirtv1.Disk{
								{
									BootOrder: &bootOrder,
									DiskDevice: kubevirtv1.DiskDevice{
										CDRom: &kubevirtv1.CDRomTarget{
											Bus: "sata",
										},
									},
									Name: "disk-0",
								},
								{
									DiskDevice: kubevirtv1.DiskDevice{
										CDRom: &kubevirtv1.CDRomTarget{
											Bus: "sata",
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
						Machine: &kubevirtv1.Machine{
							Type: "q35",
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

func (r *Repo) deleteVM() error {
	vmName := r.getVMName()

	vm, err := r.h.vmCache.Get(upgradeNamespace, vmName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.deleteImage("")
		}
		return err
	}

	if pvcName, ok := vm.Annotations[util.RemovedPVCsAnnotationKey]; ok {
		if err := r.deleteImage(pvcName); err != nil {
			return err
		}
	}

	logrus.Infof("Delete upgrade repo VM %s/%s", vm.Namespace, vm.Name)
	return r.h.vmClient.Delete(vm.Namespace, vm.Name, &metav1.DeleteOptions{})
}

// deleteImage deletes a repo image
// (1) If a PVC based on the image exists, it adds the PVC as the OwnerReference to the repo image.
// Once the repo VM is deleted and VM's PVC is deleted, the repo image will be garbage collected.
// (2) If there is no PVCs based on the repo image. We delete the image directly.
func (r *Repo) deleteImage(pvcName string) error {
	imageID := r.upgrade.Status.ImageID
	if imageID == "" {
		logrus.Error("Upgrade repo image is not provided")
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
				"harvesterhci.io/creator":      "harvester",
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

func (r *Repo) getInfo() (*RepoInfo, error) {
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
		return nil, fmt.Errorf("Unexpect status: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	info := RepoInfo{}
	err = yaml.Unmarshal(body, &info.Release)
	if err != nil {
		return nil, err
	}
	return &info, nil
}
