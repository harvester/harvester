package data

import (
	"context"

	nodev1 "github.com/harvester/node-manager/pkg/apis/node.harvesterhci.io/v1beta1"
	loggingv1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	rancherv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/crd"
)

func createCRDs(ctx context.Context, restConfig *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(ctx, restConfig)
	if err != nil {
		return err
	}
	return factory.
		BatchCreateCRDsIfNotExisted(
			crd.NonNamespacedFromGV(harvesterv1.SchemeGroupVersion, "Setting", harvesterv1.Setting{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "APIService", rancherv3.APIService{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "Setting", rancherv3.Setting{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "User", rancherv3.User{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "Group", rancherv3.Group{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "GroupMember", rancherv3.GroupMember{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "Token", rancherv3.Token{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "NodeDriver", rancherv3.NodeDriver{}),
			crd.NonNamespacedFromGV(rancherv3.SchemeGroupVersion, "Feature", rancherv3.Feature{}),
			crd.NonNamespacedFromGV(upgradev1.SchemeGroupVersion, "Plan", upgradev1.Plan{}),
			crd.NonNamespacedFromGV(loggingv1.GroupVersion, "Logging", loggingv1.Logging{}),
		).
		BatchCreateCRDsIfNotExisted(
			crd.FromGV(harvesterv1.SchemeGroupVersion, "KeyPair", harvesterv1.KeyPair{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Upgrade", harvesterv1.Upgrade{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "UpgradeLog", harvesterv1.UpgradeLog{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Version", harvesterv1.Version{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineImage", harvesterv1.VirtualMachineImage{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineTemplate", harvesterv1.VirtualMachineTemplate{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineTemplateVersion", harvesterv1.VirtualMachineTemplateVersion{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineBackup", harvesterv1.VirtualMachineBackup{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineRestore", harvesterv1.VirtualMachineRestore{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Preference", harvesterv1.Preference{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "SupportBundle", harvesterv1.SupportBundle{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "ResourceQuota", harvesterv1.ResourceQuota{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "ScheduleVMBackup", harvesterv1.ScheduleVMBackup{}),
			// The BackingImage struct is not compatible with wrangler schemas generation, pass nil as the workaround.
			// The expected CRD will be applied by Longhorn chart.
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "BackingImage", nil),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "BackingImageDataSource", lhv1beta2.BackingImageDataSource{}),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "Backup", lhv1beta2.Backup{}),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "BackupVolume", lhv1beta2.BackupVolume{}),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "BackupBackingImage", lhv1beta2.BackupBackingImage{}),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "Engine", lhv1beta2.Engine{}),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "Snapshot", lhv1beta2.Snapshot{}),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "Volume", lhv1beta2.Volume{}),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "LHNode", lhv1beta2.Node{}),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "Setting", lhv1beta2.Setting{}),
			crd.FromGV(lhv1beta2.SchemeGroupVersion, "BackupTarget", lhv1beta2.BackupTarget{}),
			crd.FromGV(provisioningv1.SchemeGroupVersion, "Cluster", provisioningv1.Cluster{}),
			crd.FromGV(fleetv1alpha1.SchemeGroupVersion, "Cluster", fleetv1alpha1.Cluster{}),
			crd.FromGV(clusterv1.GroupVersion, "Cluster", clusterv1.Cluster{}),
			crd.FromGV(clusterv1.GroupVersion, "Machine", clusterv1.Machine{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Addon", harvesterv1.Addon{}).WithStatus(),
			crd.FromGV(monitoringv1.SchemeGroupVersion, "Prometheus", monitoringv1.Prometheus{}),
			crd.FromGV(monitoringv1.SchemeGroupVersion, "Alertmanager", monitoringv1.Alertmanager{}),
			crd.FromGV(loggingv1.GroupVersion, "ClusterFlow", loggingv1.ClusterFlow{}),
			crd.FromGV(loggingv1.GroupVersion, "ClusterOutput", loggingv1.ClusterOutput{}),
			crd.FromGV(loggingv1.GroupVersion, "Flow", loggingv1.ClusterFlow{}),
			crd.FromGV(loggingv1.GroupVersion, "Output", loggingv1.ClusterOutput{}),
			crd.FromGV(nodev1.SchemeGroupVersion, "NodeConfig", nodev1.NodeConfig{}),
			crd.FromGV(rkev1.SchemeGroupVersion, "RKEControlPlane", rkev1.RKEControlPlane{}),
		).
		BatchWait()
}
