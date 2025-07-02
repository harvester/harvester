// Provides the capabilities (or features) for some well known storage provisioners.

package storagecapabilities

import (
	"context"
	"strings"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storagehelpers "k8s.io/component-helpers/storage/volume"

	"sigs.k8s.io/controller-runtime/pkg/client"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/containerized-data-importer/pkg/util"
)

// StorageCapabilities is a simple holder of storage capabilities (accessMode etc.)
type StorageCapabilities struct {
	AccessMode v1.PersistentVolumeAccessMode
	VolumeMode v1.PersistentVolumeMode
}

const (
	rwo = v1.ReadWriteOnce
	rox = v1.ReadOnlyMany
	rwx = v1.ReadWriteMany

	block = v1.PersistentVolumeBlock
	file  = v1.PersistentVolumeFilesystem
)

// CapabilitiesByProvisionerKey defines default capabilities for different storage classes
var CapabilitiesByProvisionerKey = map[string][]StorageCapabilities{
	// hostpath-provisioner
	"kubevirt.io.hostpath-provisioner": {{rwo, file}},
	"kubevirt.io/hostpath-provisioner": {{rwo, file}},
	"k8s.io/minikube-hostpath":         {{rwo, file}},
	// nfs-csi
	"nfs.csi.k8s.io": {{rwx, file}},
	"k8s-sigs.io/nfs-subdir-external-provisioner": {{rwx, file}},
	// ceph-rbd
	"kubernetes.io/rbd":                  createRbdCapabilities(),
	"rbd.csi.ceph.com":                   createRbdCapabilities(),
	"rook-ceph.rbd.csi.ceph.com":         createRbdCapabilities(),
	"openshift-storage.rbd.csi.ceph.com": createRbdCapabilities(),
	// ceph-fs
	"cephfs.csi.ceph.com":                   {{rwx, file}},
	"openshift-storage.cephfs.csi.ceph.com": {{rwx, file}},
	// LINSTOR
	"linstor.csi.linbit.com": createAllButRWXFileCapabilities(),
	// DELL Unity XT
	"csi-unity.dellemc.com":     createAllButRWXFileCapabilities(),
	"csi-unity.dellemc.com/nfs": createAllFSCapabilities(),
	// DELL PowerFlex
	"csi-vxflexos.dellemc.com":     createDellPowerFlexCapabilities(),
	"csi-vxflexos.dellemc.com/nfs": createAllFSCapabilities(),
	// DELL PowerScale
	"csi-isilon.dellemc.com": createAllFSCapabilities(),
	// DELL PowerMax
	"csi-powermax.dellemc.com":     createDellPowerMaxCapabilities(),
	"csi-powermax.dellemc.com/nfs": createAllFSCapabilities(),
	// DELL PowerStore
	"csi-powerstore.dellemc.com":     createDellPowerStoreCapabilities(),
	"csi-powerstore.dellemc.com/nfs": createAllFSCapabilities(),
	// storageos
	"kubernetes.io/storageos": {{rwo, file}},
	"storageos":               {{rwo, file}},
	// AWSElasticBlockStore
	"kubernetes.io/aws-ebs": {{rwo, block}},
	"ebs.csi.aws.com":       {{rwo, block}},
	"ebs.csi.aws.com/io2":   {{rwx, block}, {rwo, block}, {rwo, file}},
	// AWSElasticFileSystem
	"efs.csi.aws.com": {{rwx, file}, {rwo, file}},
	// Azure disk
	"kubernetes.io/azure-disk": {{rwo, block}},
	"disk.csi.azure.com":       {{rwo, block}},
	// Azure file
	"kubernetes.io/azure-file": {{rwx, file}},
	"file.csi.azure.com":       {{rwx, file}},
	// GCE Persistent Disk
	"kubernetes.io/gce-pd":  {{rwo, block}},
	"pd.csi.storage.gke.io": {{rwo, block}},
	// Hitachi
	"hspc.csi.hitachi.com": {{rwx, block}, {rwo, block}, {rwo, file}},
	// HPE
	"csi.hpe.com": {{rwx, block}, {rwo, block}, {rwo, file}},
	// IBM HCI/GPFS2 (Spectrum Scale / Spectrum Fusion)
	"spectrumscale.csi.ibm.com": {{rwx, file}, {rwo, file}},
	// IBM block arrays (FlashSystem)
	"block.csi.ibm.com": {{rwo, block}, {rwo, file}},
	// Portworx in-tree CSI
	"kubernetes.io/portworx-volume/shared": {{rwx, file}},
	"kubernetes.io/portworx-volume":        {{rwo, file}},
	// Portworx CSI
	"pxd.openstorage.org/shared": createOpenStorageSharedVolumeCapabilities(),
	"pxd.openstorage.org":        createOpenStorageSharedVolumeCapabilities(),
	"pxd.portworx.com/shared":    createOpenStorageSharedVolumeCapabilities(),
	"pxd.portworx.com":           createOpenStorageSharedVolumeCapabilities(),
	// Trident
	"csi.trident.netapp.io/ontap-nas": {{rwx, file}, {rwo, file}},
	"csi.trident.netapp.io/ontap-san": {{rwx, block}},
	// topolvm
	"topolvm.cybozu.com": createTopoLVMCapabilities(),
	"topolvm.io":         createTopoLVMCapabilities(),
	// OpenStack Cinder
	"cinder.csi.openstack.org": createRWOBlockAndFilesystemCapabilities(),
	// OpenStack manila
	"manila.csi.openstack.org": {{rwx, file}},
	// ovirt csi
	"csi.ovirt.org": createRWOBlockAndFilesystemCapabilities(),
	// Infinidat
	"infinibox-csi-driver/iscsiorfibrechannel": {{rwx, block}, {rwo, block}, {rwo, file}},
	"infinibox-csi-driver/nfs":                 {{rwx, file}, {rwo, file}},
	// vSphere
	"csi.vsphere.vmware.com":     {{rwo, block}, {rwo, file}},
	"csi.vsphere.vmware.com/nfs": {{rwx, file}, {rwo, block}, {rwo, file}},
	// huawei
	"csi.huawei.com":     createAllButRWXFileCapabilities(),
	"csi.huawei.com/nfs": createAllFSCapabilities(),
	// KubeSAN
	"kubesan.gitlab.io": {{rwx, block}, {rox, block}, {rwo, block}, {rwo, file}},
	// Longhorn
	"driver.longhorn.io":            {{rwo, block}},
	"driver.longhorn.io/migratable": {{rwx, block}, {rwo, block}},
}

// SourceFormatsByProvisionerKey defines the advised data import cron source format
// Certain storage provisioners will scale better cloning from a single source VolumeSnapshot source
var SourceFormatsByProvisionerKey = map[string]cdiv1.DataImportCronSourceFormat{
	"rook-ceph.rbd.csi.ceph.com":         cdiv1.DataImportCronSourceFormatSnapshot,
	"openshift-storage.rbd.csi.ceph.com": cdiv1.DataImportCronSourceFormatSnapshot,
	"csi.trident.netapp.io/ontap-nas":    cdiv1.DataImportCronSourceFormatSnapshot,
	"csi.trident.netapp.io/ontap-san":    cdiv1.DataImportCronSourceFormatSnapshot,
}

// CloneStrategyByProvisionerKey defines the advised clone strategy for a provisioner
var CloneStrategyByProvisionerKey = map[string]cdiv1.CDICloneStrategy{
	"csi-vxflexos.dellemc.com":                 cdiv1.CloneStrategyCsiClone,
	"csi-isilon.dellemc.com":                   cdiv1.CloneStrategyCsiClone,
	"csi-powermax.dellemc.com":                 cdiv1.CloneStrategyCsiClone,
	"csi-powerstore.dellemc.com":               cdiv1.CloneStrategyCsiClone,
	"hspc.csi.hitachi.com":                     cdiv1.CloneStrategyCsiClone,
	"csi.hpe.com":                              cdiv1.CloneStrategyCsiClone,
	"spectrumscale.csi.ibm.com":                cdiv1.CloneStrategyCsiClone,
	"rook-ceph.rbd.csi.ceph.com":               cdiv1.CloneStrategyCsiClone,
	"openshift-storage.rbd.csi.ceph.com":       cdiv1.CloneStrategyCsiClone,
	"cephfs.csi.ceph.com":                      cdiv1.CloneStrategyCsiClone,
	"openshift-storage.cephfs.csi.ceph.com":    cdiv1.CloneStrategyCsiClone,
	"pxd.openstorage.org/shared":               cdiv1.CloneStrategyCsiClone,
	"pxd.openstorage.org":                      cdiv1.CloneStrategyCsiClone,
	"pxd.portworx.com/shared":                  cdiv1.CloneStrategyCsiClone,
	"pxd.portworx.com":                         cdiv1.CloneStrategyCsiClone,
	"topolvm.cybozu.com":                       cdiv1.CloneStrategyHostAssisted,
	"topolvm.io":                               cdiv1.CloneStrategyHostAssisted,
	"infinibox-csi-driver/iscsiorfibrechannel": cdiv1.CloneStrategyCsiClone,
	"infinibox-csi-driver/nfs":                 cdiv1.CloneStrategyCsiClone,
	"csi.trident.netapp.io/ontap-nas":          cdiv1.CloneStrategySnapshot,
	"csi.trident.netapp.io/ontap-san":          cdiv1.CloneStrategySnapshot,
	"kubesan.gitlab.io":                        cdiv1.CloneStrategyCsiClone,
}

const (
	// ProvisionerNoobaa is the provisioner string for the Noobaa object bucket provisioner which does not work with CDI
	ProvisionerNoobaa = "openshift-storage.noobaa.io/obc"
	// ProvisionerOCSBucket is the provisioner string for the downstream ODF/OCS provisoner for buckets which does not work with CDI
	ProvisionerOCSBucket = "openshift-storage.ceph.rook.io/bucket"
	// ProvisionerRookCephBucket is the provisioner string for the upstream Rook Ceph provisoner for buckets which does not work with CDI
	ProvisionerRookCephBucket = "rook-ceph.ceph.rook.io/bucket"
	// ProvisionerStorkSnapshot is the provisioner string for the Stork snapshot provisoner which does not work with CDI
	ProvisionerStorkSnapshot = "stork-snapshot"
)

// UnsupportedProvisioners is a hash of provisioners which are known not to work with CDI
var UnsupportedProvisioners = map[string]struct{}{
	// The following provisioners may be found in Rook/Ceph deployments and are related to object storage
	ProvisionerOCSBucket:                   {},
	ProvisionerRookCephBucket:              {},
	ProvisionerNoobaa:                      {},
	ProvisionerStorkSnapshot:               {},
	storagehelpers.NotSupportedProvisioner: {},
}

// GetCapabilities finds and returns a predefined StorageCapabilities for a given StorageClass
func GetCapabilities(cl client.Client, sc *storagev1.StorageClass) ([]StorageCapabilities, bool) {
	provisionerKey := storageProvisionerKey(sc)
	if provisionerKey == storagehelpers.NotSupportedProvisioner {
		return capabilitiesForNoProvisioner(cl, sc)
	}
	capabilities, found := CapabilitiesByProvisionerKey[provisionerKey]
	return capabilities, found
}

// GetAdvisedSourceFormat finds and returns the advised format for dataimportcron sources
func GetAdvisedSourceFormat(sc *storagev1.StorageClass) (cdiv1.DataImportCronSourceFormat, bool) {
	provisionerKey := storageProvisionerKey(sc)
	format, found := SourceFormatsByProvisionerKey[provisionerKey]
	return format, found
}

// GetAdvisedCloneStrategy finds and returns the advised clone strategy
func GetAdvisedCloneStrategy(sc *storagev1.StorageClass) (cdiv1.CDICloneStrategy, bool) {
	provisionerKey := storageProvisionerKey(sc)
	strategy, found := CloneStrategyByProvisionerKey[provisionerKey]
	return strategy, found
}

func capabilitiesForNoProvisioner(cl client.Client, sc *storagev1.StorageClass) ([]StorageCapabilities, bool) {
	pvs := &v1.PersistentVolumeList{}
	err := cl.List(context.TODO(), pvs)
	if err != nil {
		return []StorageCapabilities{}, false
	}
	capabilities := []StorageCapabilities{}
	for _, pv := range pvs.Items {
		if pv.Spec.StorageClassName == sc.Name {
			for _, accessMode := range pv.Spec.AccessModes {
				capabilities = append(capabilities, StorageCapabilities{
					AccessMode: accessMode,
					VolumeMode: util.ResolveVolumeMode(pv.Spec.VolumeMode),
				})
			}
		}
	}
	capabilities = uniqueCapabilities(capabilities)
	return capabilities, len(capabilities) > 0
}

func uniqueCapabilities(input []StorageCapabilities) []StorageCapabilities {
	capabilitiesMap := make(map[StorageCapabilities]bool)
	for _, capability := range input {
		capabilitiesMap[capability] = true
	}
	output := []StorageCapabilities{}
	for capability := range capabilitiesMap {
		output = append(output, capability)
	}
	return output
}

func storageProvisionerKey(sc *storagev1.StorageClass) string {
	keyMapper, found := storageClassToProvisionerKeyMapper[sc.Provisioner]
	if found {
		return keyMapper(sc)
	}
	// by default the Provisioner name is the key
	return sc.Provisioner
}

var storageClassToProvisionerKeyMapper = map[string]func(sc *storagev1.StorageClass) string{
	"pxd.openstorage.org": func(sc *storagev1.StorageClass) string {
		// https://docs.portworx.com/portworx-install-with-kubernetes/storage-operations/create-pvcs/create-shared-pvcs/
		val := sc.Parameters["shared"]
		if val == "true" {
			return "pxd.openstorage.org/shared"
		}
		return "pxd.openstorage.org"
	},
	"kubernetes.io/portworx-volume": func(sc *storagev1.StorageClass) string {
		val := sc.Parameters["shared"]
		if val == "true" {
			return "kubernetes.io/portworx-volume/shared"
		}
		return "kubernetes.io/portworx-volume"
	},
	"pxd.portworx.com": func(sc *storagev1.StorageClass) string {
		// https://docs.portworx.com/portworx-install-with-kubernetes/storage-operations/csi/volumelifecycle/#create-shared-csi-enabled-volumes
		val := sc.Parameters["shared"]
		if val == "true" {
			return "pxd.portworx.com/shared"
		}
		return "pxd.portworx.com"
	},
	"csi.trident.netapp.io": func(sc *storagev1.StorageClass) string {
		// https://netapp-trident.readthedocs.io/en/stable-v20.04/kubernetes/concepts/objects.html#kubernetes-storageclass-objects
		val := sc.Parameters["backendType"]
		if strings.HasPrefix(val, "ontap-nas") {
			return "csi.trident.netapp.io/ontap-nas"
		}
		if strings.HasPrefix(val, "ontap-san") {
			return "csi.trident.netapp.io/ontap-san"
		}
		return "UNKNOWN"
	},
	"infinibox-csi-driver": func(sc *storagev1.StorageClass) string {
		// https://github.com/Infinidat/infinibox-csi-driver/tree/develop/deploy/examples
		switch sc.Parameters["storage_protocol"] {
		case "iscsi", "fc":
			return "infinibox-csi-driver/iscsiorfibrechannel"
		case "nfs", "nfs_treeq":
			return "infinibox-csi-driver/nfs"
		default:
			return "UNKNOWN"
		}
	},
	"csi.vsphere.vmware.com": func(sc *storagev1.StorageClass) string {
		fsType := getFSType(sc)
		if strings.Contains(fsType, "nfs") {
			return "csi.vsphere.vmware.com/nfs"
		}
		return "csi.vsphere.vmware.com"
	},
	"csi-unity.dellemc.com": func(sc *storagev1.StorageClass) string {
		// https://github.com/dell/csi-unity/blob/1f42af327f4130df65c5532f6029559e4ab579b5/samples/storageclass
		switch strings.ToLower(sc.Parameters["protocol"]) {
		case "nfs":
			return "csi-unity.dellemc.com/nfs"
		default:
			return "csi-unity.dellemc.com"
		}
	},
	"csi-powerstore.dellemc.com": func(sc *storagev1.StorageClass) string {
		// https://github.com/dell/csi-powerstore/blob/76e2cb671bd3cb28aa860e9057649d1d911e1deb/samples/storageclass
		fsType := getFSType(sc)
		switch fsType {
		case "nfs":
			return "csi-powerstore.dellemc.com/nfs"
		default:
			return "csi-powerstore.dellemc.com"
		}
	},
	"csi-vxflexos.dellemc.com": func(sc *storagev1.StorageClass) string {
		// https://github.com/dell/csi-powerflex/tree/main/samples/storageclass
		fsType := getFSType(sc)
		switch fsType {
		case "nfs":
			return "csi-vxflexos.dellemc.com/nfs"
		default:
			return "csi-vxflexos.dellemc.com"
		}
	},
	"csi-powermax.dellemc.com": func(sc *storagev1.StorageClass) string {
		// https://github.com/dell/csi-powermax/tree/main/samples/storageclass
		fsType := getFSType(sc)
		switch fsType {
		case "nfs":
			return "csi-powermax.dellemc.com/nfs"
		default:
			return "csi-powermax.dellemc.com"
		}
	},
	"csi.huawei.com": func(sc *storagev1.StorageClass) string {
		switch sc.Parameters["protocol"] {
		case "nfs":
			return "csi.huawei.com/nfs"
		default:
			return "csi.huawei.com"
		}
	},
	"driver.longhorn.io": func(sc *storagev1.StorageClass) string {
		migratable := sc.Parameters["migratable"]
		if migratable == "true" {
			return "driver.longhorn.io/migratable"
		}
		return "driver.longhorn.io"
	},
	"ebs.csi.aws.com": func(sc *storagev1.StorageClass) string {
		val := sc.Parameters["type"]
		if val == "io2" {
			return "ebs.csi.aws.com/io2"
		}
		return "ebs.csi.aws.com"
	},
}

func getFSType(sc *storagev1.StorageClass) string {
	return strings.ToLower(sc.Parameters["csi.storage.k8s.io/fstype"])
}

func createRbdCapabilities() []StorageCapabilities {
	return []StorageCapabilities{
		{rwx, block},
		{rwo, block},
		{rwo, file},
	}
}

func createAllButRWXFileCapabilities() []StorageCapabilities {
	return []StorageCapabilities{
		{rwx, block},
		{rwo, block},
		{rwo, file},
		{rox, block},
		{rox, file},
	}
}

func createDellPowerMaxCapabilities() []StorageCapabilities {
	return []StorageCapabilities{
		{rwx, block},
		{rwo, block},
		{rwo, file},
		{rox, block},
	}
}

func createDellPowerFlexCapabilities() []StorageCapabilities {
	return []StorageCapabilities{
		{rwx, block},
		{rwo, block},
		{rwo, file},
		{rox, block},
		{rox, file},
	}
}

func createAllFSCapabilities() []StorageCapabilities {
	return []StorageCapabilities{
		{rwx, file},
		{rwo, file},
		{rox, file},
	}
}

func createDellPowerStoreCapabilities() []StorageCapabilities {
	return []StorageCapabilities{
		{rwx, block},
		{rwo, block},
		{rwo, file},
		{rox, block},
	}
}

func createTopoLVMCapabilities() []StorageCapabilities {
	return []StorageCapabilities{
		{rwo, block},
		{rwo, file},
	}
}

func createOpenStorageSharedVolumeCapabilities() []StorageCapabilities {
	return []StorageCapabilities{
		{rwx, file},
		{rwo, block},
		{rwo, file},
	}
}

func createRWOBlockAndFilesystemCapabilities() []StorageCapabilities {
	return []StorageCapabilities{
		{rwo, block},
		{rwo, file},
	}
}
