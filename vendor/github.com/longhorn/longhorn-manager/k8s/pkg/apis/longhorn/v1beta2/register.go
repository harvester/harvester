package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn"
)

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

var SchemeGroupVersion = schema.GroupVersion{Group: longhorn.GroupName, Version: "v1beta2"}

func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Volume{},
		&VolumeList{},
		&Engine{},
		&EngineList{},
		&Replica{},
		&ReplicaList{},
		&Setting{},
		&SettingList{},
		&EngineImage{},
		&EngineImageList{},
		&Node{},
		&NodeList{},
		&InstanceManager{},
		&InstanceManagerList{},
		&ShareManager{},
		&ShareManagerList{},
		&BackingImage{},
		&BackingImageList{},
		&BackingImageManager{},
		&BackingImageManagerList{},
		&BackingImageDataSource{},
		&BackingImageDataSourceList{},
		&BackupTarget{},
		&BackupTargetList{},
		&BackupVolume{},
		&BackupVolumeList{},
		&Backup{},
		&BackupList{},
		&RecurringJob{},
		&RecurringJobList{},
		&Orphan{},
		&OrphanList{},
		&Snapshot{},
		&SnapshotList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
