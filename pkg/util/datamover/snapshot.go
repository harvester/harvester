package datamover

import (
	"context"
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

// Snapshots imports an existing snapshot without transferring backend ownership.
type Snapshots struct{ Client dynamic.Interface }

func (s Snapshots) Import(parent metav1.Object, sourceName, name string) error {
	content, err := s.sourceContent(parent, sourceName)
	if err != nil {
		return err
	}
	imported := importedContent(parent, content, name)
	if err := s.ensureContent(parent, imported); err != nil {
		return err
	}
	return s.ensureSnapshot(parent, name)
}

func importedContent(
	parent metav1.Object, source *snapshotv1.VolumeSnapshotContent, name string,
) *snapshotv1.VolumeSnapshotContent {
	imported := &snapshotv1.VolumeSnapshotContent{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snapshot.storage.k8s.io/v1", Kind: "VolumeSnapshotContent"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver:         source.Spec.Driver,
			DeletionPolicy: snapshotv1.VolumeSnapshotContentRetain,
			Source: snapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: ptr.To(*source.Status.SnapshotHandle),
			},
			VolumeSnapshotRef: corev1.ObjectReference{
				APIVersion: snapshotv1.SchemeGroupVersion.String(), Kind: "VolumeSnapshot",
				Namespace: joblifecycle.Namespace, Name: name,
			},
			SourceVolumeMode: source.Spec.SourceVolumeMode,
		},
	}
	joblifecycle.Track(&imported.ObjectMeta, parent, BackupKind)
	return imported
}

func (s Snapshots) sourceContent(parent metav1.Object, name string) (*snapshotv1.VolumeSnapshotContent, error) {
	source, err := s.sourceSnapshot(parent, name)
	if err != nil {
		return nil, err
	}
	content, err := s.content(*source.Status.BoundVolumeSnapshotContentName)
	if err != nil {
		return nil, err
	}
	if err := validateSourceContent(source, content); err != nil {
		return nil, err
	}
	return content, nil
}

func (s Snapshots) sourceSnapshot(parent metav1.Object, name string) (*snapshotv1.VolumeSnapshot, error) {
	source, err := s.snapshot(parent.GetNamespace(), name)
	if err != nil {
		return nil, err
	}
	if source.DeletionTimestamp != nil || source.Status == nil || !ptr.Deref(source.Status.ReadyToUse, false) {
		return nil, ErrPending
	}
	if err := joblifecycle.Validate(parent, BackupKind, source.ObjectMeta); err != nil {
		return nil, err
	}
	if ptr.Deref(source.Status.BoundVolumeSnapshotContentName, "") == "" {
		return nil, ErrPending
	}
	return source, nil
}

func validateSourceContent(
	source *snapshotv1.VolumeSnapshot, content *snapshotv1.VolumeSnapshotContent,
) error {
	ref := content.Spec.VolumeSnapshotRef
	if ref.Name != source.Name || ref.Namespace != source.Namespace || ref.UID != source.UID {
		return fmt.Errorf("source snapshot content has a different binding")
	}
	if content.DeletionTimestamp != nil || content.Status == nil ||
		ptr.Deref(content.Status.SnapshotHandle, "") == "" {
		return ErrPending
	}
	return nil
}

func (s Snapshots) ensureContent(parent metav1.Object, desired *snapshotv1.VolumeSnapshotContent) error {
	current, err := s.content(desired.Name)
	if apierrors.IsNotFound(err) {
		_, err = s.create(contentResource, "", desired)
		return err
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(parent, BackupKind, current.ObjectMeta); err != nil {
		return err
	}
	if current.DeletionTimestamp != nil {
		return ErrPending
	}
	a, b := current.Spec, desired.Spec
	currentMode := ptr.Deref(a.SourceVolumeMode, corev1.PersistentVolumeFilesystem)
	desiredMode := ptr.Deref(b.SourceVolumeMode, corev1.PersistentVolumeFilesystem)
	if a.Driver != b.Driver || a.DeletionPolicy != b.DeletionPolicy ||
		ptr.Deref(a.Source.SnapshotHandle, "") != ptr.Deref(b.Source.SnapshotHandle, "") ||
		a.VolumeSnapshotRef.Name != b.VolumeSnapshotRef.Name || a.VolumeSnapshotRef.Namespace != b.VolumeSnapshotRef.Namespace ||
		currentMode != desiredMode {
		return fmt.Errorf("imported snapshot content does not match source")
	}
	return nil
}

func (s Snapshots) ensureSnapshot(parent metav1.Object, name string) error {
	current, err := s.snapshot(joblifecycle.Namespace, name)
	if apierrors.IsNotFound(err) {
		return s.createSnapshot(parent, name)
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(parent, BackupKind, current.ObjectMeta); err != nil {
		return err
	}
	if ptr.Deref(current.Spec.Source.VolumeSnapshotContentName, "") != name {
		return fmt.Errorf("imported snapshot has a different source")
	}
	content, err := s.content(name)
	if err != nil {
		return err
	}
	if uid := content.Spec.VolumeSnapshotRef.UID; uid != "" && uid != current.UID {
		return fmt.Errorf("imported content is bound to a different snapshot UID")
	}
	if current.Status != nil && ptr.Deref(current.Status.BoundVolumeSnapshotContentName, name) != name {
		return fmt.Errorf("imported snapshot is bound to a different content")
	}
	if current.Status != nil && current.Status.Error != nil {
		return fmt.Errorf("imported snapshot failed: %s", ptr.Deref(current.Status.Error.Message, "unknown error"))
	}
	if current.DeletionTimestamp != nil || current.Status == nil || !ptr.Deref(current.Status.ReadyToUse, false) {
		return ErrPending
	}
	return nil
}

func (s Snapshots) createSnapshot(parent metav1.Object, name string) error {
	desired := &snapshotv1.VolumeSnapshot{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snapshot.storage.k8s.io/v1", Kind: "VolumeSnapshot"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: joblifecycle.Namespace},
		Spec:       snapshotv1.VolumeSnapshotSpec{Source: snapshotv1.VolumeSnapshotSource{VolumeSnapshotContentName: ptr.To(name)}},
	}
	joblifecycle.Track(&desired.ObjectMeta, parent, BackupKind)
	_, err := s.create(snapshotResource, joblifecycle.Namespace, desired)
	if err != nil {
		return err
	}
	return ErrPending
}

func (s Snapshots) Delete(parent metav1.Object, name string) error {
	if err := s.deleteSnapshot(parent, name); err != nil {
		return err
	}
	return s.deleteContent(parent, name)
}

func (s Snapshots) deleteSnapshot(parent metav1.Object, name string) error {
	snapshot, err := s.snapshot(joblifecycle.Namespace, name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(parent, BackupKind, snapshot.ObjectMeta); err != nil {
		return err
	}
	err = s.Client.Resource(snapshotResource).Namespace(snapshot.Namespace).Delete(context.Background(), name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &snapshot.UID},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return ErrPending
}

func (s Snapshots) deleteContent(parent metav1.Object, name string) error {
	content, err := s.content(name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(parent, BackupKind, content.ObjectMeta); err != nil {
		return err
	}
	if content.Spec.DeletionPolicy != snapshotv1.VolumeSnapshotContentRetain {
		return fmt.Errorf("imported snapshot must retain its backend snapshot")
	}
	err = s.Client.Resource(contentResource).Delete(context.Background(), name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &content.UID},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return ErrPending
}

var (
	snapshotResource = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshots"}
	contentResource  = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotcontents"}
)

// The generated v4 snapshot clients omit sourceVolumeMode. Use the already
// vendored v6 API through a dynamic client to preserve that immutable field.
func (s Snapshots) snapshot(namespace, name string) (*snapshotv1.VolumeSnapshot, error) {
	value, err := s.Client.Resource(snapshotResource).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	result := &snapshotv1.VolumeSnapshot{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(value.Object, result)
	return result, err
}

func (s Snapshots) content(name string) (*snapshotv1.VolumeSnapshotContent, error) {
	value, err := s.Client.Resource(contentResource).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	result := &snapshotv1.VolumeSnapshotContent{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(value.Object, result)
	return result, err
}

func (s Snapshots) create(resource schema.GroupVersionResource, namespace string, value interface{}) (*unstructured.Unstructured, error) {
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(value)
	if err != nil {
		return nil, err
	}
	client := s.Client.Resource(resource).Namespace(namespace)
	return client.Create(context.Background(), &unstructured.Unstructured{Object: data}, metav1.CreateOptions{})
}

// Cleanup also finds content left behind between import creation steps.
func (s Snapshots) Cleanup(parent metav1.Object) error {
	if err := s.cleanupImports(parent); err != nil {
		return err
	}
	if err := s.cleanupContents(parent); err != nil {
		return err
	}
	return s.cleanupSources(parent)
}

func (s Snapshots) cleanupImports(parent metav1.Object) error {
	client := s.Client.Resource(snapshotResource).Namespace(joblifecycle.Namespace)
	snapshots, err := client.List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.Selector(parent, BackupKind),
	})
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots.Items {
		// A VM may itself live in harvester-system. Its original snapshot is
		// removed only after all imported resources have disappeared.
		if _, ok, _ := unstructured.NestedString(snapshot.Object, "spec", "source", "persistentVolumeClaimName"); ok {
			continue
		}
		if err := s.Delete(parent, snapshot.GetName()); err != nil {
			return err
		}
	}
	return nil
}

func (s Snapshots) cleanupContents(parent metav1.Object) error {
	contents, err := s.Client.Resource(contentResource).List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.Selector(parent, BackupKind),
	})
	if err != nil {
		return err
	}
	for _, content := range contents.Items {
		if err := s.Delete(parent, content.GetName()); err != nil {
			return err
		}
	}
	return nil
}

func (s Snapshots) cleanupSources(parent metav1.Object) error {
	client := s.Client.Resource(snapshotResource).Namespace(parent.GetNamespace())
	list, err := client.List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.Selector(parent, BackupKind),
	})
	if err != nil {
		return err
	}
	for _, obj := range list.Items {
		if err := s.deleteSource(parent, obj.GetName()); err != nil {
			return err
		}
	}
	return nil
}

func (s Snapshots) deleteSource(parent metav1.Object, name string) error {
	source, err := s.snapshot(parent.GetNamespace(), name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(parent, BackupKind, source.ObjectMeta); err != nil {
		return err
	}
	client := s.Client.Resource(snapshotResource).Namespace(parent.GetNamespace())
	err = client.Delete(context.Background(), source.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &source.UID},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return ErrPending
}
