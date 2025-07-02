package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	whereaboutscniv1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	loggingv1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	storagesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	controllergen "github.com/rancher/wrangler/v3/pkg/controller-gen"
	"github.com/rancher/wrangler/v3/pkg/controller-gen/args"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdiuploadv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/upload/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/harvester/harvester/pkg/generated",
		Boilerplate:   "scripts/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"harvesterhci.io": {
				Types: []interface{}{
					harvesterv1.KeyPair{},
					harvesterv1.Preference{},
					harvesterv1.Setting{},
					harvesterv1.Upgrade{},
					harvesterv1.UpgradeLog{},
					harvesterv1.Version{},
					harvesterv1.VirtualMachineBackup{},
					harvesterv1.VirtualMachineRestore{},
					harvesterv1.VirtualMachineImage{},
					harvesterv1.VirtualMachineTemplate{},
					harvesterv1.VirtualMachineTemplateVersion{},
					harvesterv1.SupportBundle{},
					harvesterv1.Addon{},
					harvesterv1.ResourceQuota{},
					harvesterv1.ScheduleVMBackup{},
					harvesterv1.VirtualMachineImageDownloader{},
				},
				GenerateTypes:   true,
				GenerateClients: true,
			},
			loggingv1.GroupVersion.Group: {
				Types: []interface{}{
					loggingv1.Logging{},
					loggingv1.ClusterFlow{},
					loggingv1.ClusterOutput{},
					loggingv1.Flow{},
					loggingv1.Output{},
					loggingv1.FluentbitAgent{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			kubevirtv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					kubevirtv1.VirtualMachine{},
					kubevirtv1.VirtualMachineInstance{},
					kubevirtv1.VirtualMachineInstanceMigration{},
					kubevirtv1.KubeVirt{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			cniv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					cniv1.NetworkAttachmentDefinition{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			whereaboutscniv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					whereaboutscniv1.IPPool{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			networkingv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					networkingv1.Ingress{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			storagev1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					storagev1.VolumeAttachment{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			storagesnapshotv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					storagesnapshotv1.VolumeSnapshotClass{},
					storagesnapshotv1.VolumeSnapshot{},
					storagesnapshotv1.VolumeSnapshotContent{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			longhornv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					longhornv1.Node{},
					longhornv1.BackingImage{},
					longhornv1.BackingImageDataSource{},
					longhornv1.Volume{},
					longhornv1.Setting{},
					longhornv1.Backup{},
					longhornv1.BackupVolume{},
					longhornv1.BackupBackingImage{},
					longhornv1.Replica{},
					longhornv1.Engine{},
					longhornv1.Snapshot{},
					longhornv1.BackupTarget{},
				},
				GenerateClients: true,
			},
			upgradev1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					upgradev1.Plan{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			capi.GroupVersion.Group: {
				Types: []interface{}{
					capi.Cluster{},
					capi.Machine{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			corev1.GroupName: {
				Types: []interface{}{
					corev1.PersistentVolume{},
					corev1.ResourceQuota{},
				},
			},
			batchv1.GroupName: {
				Types: []interface{}{
					batchv1.CronJob{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			catalogv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					catalogv1.App{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			mgmtv3.SchemeGroupVersion.Group: {
				Types: []interface{}{
					mgmtv3.ManagedChart{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			monitoring.GroupName: {
				Types: []interface{}{
					monitoringv1.Prometheus{},
					monitoringv1.Alertmanager{},
				},
				GenerateClients: true,
			},
			appsv1.GroupName: {
				Types: []interface{}{
					appsv1.ControllerRevision{},
				},
			},
			networkv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					networkv1.VlanConfig{},
					networkv1.VlanStatus{},
					networkv1.ClusterNetwork{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			cdiv1.CDIGroupVersionKind.Group: {
				Types: []interface{}{
					cdiv1.DataVolume{},
					cdiv1.StorageProfile{},
					cdiv1.VolumeImportSource{},
					cdiv1.CDI{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			cdiuploadv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					cdiuploadv1.UploadTokenRequest{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
		},
	})
	nadControllerInterfaceRefactor()
	capiWorkaround()
	loggingWorkaround()
}

// NB(GC), nadControllerInterfaceRefactor modify the generated resource name of NetworkAttachmentDefinition controller using a dash-separator,
// the original code is generated by https://github.com/rancher/wrangler/blob/e86bc912dfacbc81dc2d70171e4d103248162da6/pkg/controller-gen/generators/group_version_interface_go.go#L82-L97
// since the NAD crd uses a varietal plurals name(i.e network-attachment-definitions), and the default resource name generated by wrangler is
// `networkattachementdefinitions` that will raises crd not found exception of the NAD controller.
func nadControllerInterfaceRefactor() {
	absPath, _ := filepath.Abs("pkg/generated/controllers/k8s.cni.cncf.io/v1/interface.go")
	input, err := ioutil.ReadFile(absPath)
	if err != nil {
		logrus.Fatalf("failed to read the network-attachment-definition file: %v", err)
	}

	output := bytes.ReplaceAll(input, []byte("networkattachmentdefinitions"), []byte("network-attachment-definitions"))

	if err = ioutil.WriteFile(absPath, output, 0644); err != nil {
		logrus.Fatalf("failed to update the network-attachment-definition file: %v", err)
	}
}

// capiWorkaround replaces the variable `SchemeGroupVersion` with `GroupVersion` in clusters.cluster.x-k8s.io client because
// `SchemeGroupVersion` is not declared in the vendor package but wrangler uses it.
// https://github.com/kubernetes-sigs/cluster-api/blob/56f9e9db7a9e9ca625ffe4bdc1e5e93a14d5e96c/api/v1beta1/groupversion_info.go#L29
func capiWorkaround() {
	files := []string{
		"pkg/generated/clientset/versioned/typed/cluster.x-k8s.io/v1beta1/cluster.x-k8s.io_client.go",
		"pkg/generated/clientset/versioned/typed/cluster.x-k8s.io/v1beta1/fake/fake_machine.go",
		"pkg/generated/clientset/versioned/typed/cluster.x-k8s.io/v1beta1/fake/fake_cluster.go",
	}

	// Replace the variable `SchemeGroupVersion` with `GroupVersion` in the above files path
	for _, absPath := range files {
		input, err := ioutil.ReadFile(absPath)
		if err != nil {
			logrus.Fatalf("failed to read the clusters.cluster.x-k8s.io client file: %v", err)
		}
		output := bytes.ReplaceAll(input, []byte("v1beta1.SchemeGroupVersion"), []byte("v1beta1.GroupVersion"))

		if err = ioutil.WriteFile(absPath, output, 0644); err != nil {
			logrus.Fatalf("failed to update the clusters.cluster.x-k8s.io client file: %v", err)
		}
	}
}

// loggingWorkaround replaces the variable `SchemeGroupVersion` with `GroupVersion` in logging.banzaicloud.io client because
// `SchemeGroupVersion` is not declared in the vendor package but wrangler uses it.
// https://github.com/banzaicloud/logging-operator/blob/e935c5d60604036a6f40cd4ab991420c6eaf096b/pkg/sdk/logging/api/v1beta1/groupversion_info.go#L27
func loggingWorkaround() {
	files := []string{
		"pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1/logging.banzaicloud.io_client.go",
		"pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1/fake/fake_clusterflow.go",
		"pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1/fake/fake_clusteroutput.go",
		"pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1/fake/fake_flow.go",
		"pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1/fake/fake_output.go",
		"pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1/fake/fake_logging.go",
		"pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1/fake/fake_fluentbitagent.go",
	}

	// Replace the variable `SchemeGroupVersion` with `GroupVersion` in the above files path
	for _, absPath := range files {
		input, err := ioutil.ReadFile(absPath)
		if err != nil {
			logrus.Fatalf("failed to read the logging.banzaicloud.io client file: %v", err)
		}
		output := bytes.ReplaceAll(input, []byte("v1beta1.SchemeGroupVersion"), []byte("v1beta1.GroupVersion"))

		if err = ioutil.WriteFile(absPath, output, 0644); err != nil {
			logrus.Fatalf("failed to update the logging.banzaicloud.io client file: %v", err)
		}
	}
}
