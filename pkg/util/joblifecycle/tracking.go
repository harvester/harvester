package joblifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	wranglername "github.com/rancher/wrangler/v3/pkg/name"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	Namespace                 = "harvester-system"
	AnnotationParentNamespace = "harvesterhci.io/job-parent-namespace"
	LabelParentUID            = "harvesterhci.io/job-parent-uid"
	LabelParentKind           = "harvesterhci.io/job-parent-kind"
	AnnotationParentName      = "harvesterhci.io/job-parent-name"
)

// Track associates an independent Job with the resource responsible for cleanup.
func Track(meta *metav1.ObjectMeta, parent metav1.Object, kind string) {
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	if meta.Annotations == nil {
		meta.Annotations = map[string]string{}
	}
	meta.Labels[LabelParentUID] = string(parent.GetUID())
	meta.Labels[LabelParentKind] = kind
	meta.Annotations[AnnotationParentName] = parent.GetName()
	meta.Annotations[AnnotationParentNamespace] = parent.GetNamespace()
	meta.OwnerReferences = nil
}

// ValidateJob accepts terminating Jobs, which can still hold protected results.
func ValidateJob(parent metav1.Object, kind string, job *batchv1.Job) error {
	if job.Namespace != Namespace {
		return fmt.Errorf("job is outside %s", Namespace)
	}
	return Validate(parent, kind, job.ObjectMeta)
}

// ParentName routes independent Job events to the matching parent controller.
func ParentName(meta metav1.ObjectMeta, kind string) string {
	if len(meta.OwnerReferences) != 0 || meta.Labels[LabelParentKind] != kind ||
		meta.Labels[LabelParentUID] == "" {
		return ""
	}
	return meta.Annotations[AnnotationParentName]
}

func GetJob(
	client ctlbatchv1.JobClient, parent metav1.Object, kind, name string,
) (*batchv1.Job, error) {
	job, err := client.Get(Namespace, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return job, ValidateJob(parent, kind, job)
}

func ReleaseJob(client ctlbatchv1.JobClient, parent metav1.Object, kind, name string) error {
	job, err := GetJob(client, parent, kind, name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return Release(client, job)
}

func DeleteJob(client ctlbatchv1.JobClient, parent metav1.Object, kind, name string) error {
	job, err := GetJob(client, parent, kind, name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return Delete(client, job)
}

// DeleteJobs validates parent identity before releasing and deleting its Jobs.
func DeleteJobs(client ctlbatchv1.JobClient, parent metav1.Object, kind string) error {
	if parent.GetUID() == "" || parent.GetName() == "" || parent.GetNamespace() == "" {
		return fmt.Errorf("cannot clean Jobs without a complete parent identity")
	}
	selector := labels.Set{LabelParentUID: string(parent.GetUID()), LabelParentKind: kind}
	jobs, err := client.List(Namespace, metav1.ListOptions{LabelSelector: selector.AsSelector().String()})
	if err != nil {
		return err
	}
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if ValidateJob(parent, kind, job) != nil {
			continue
		}
		if err := Delete(client, job); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks identity before adopting or deleting a tracked resource.
// Cluster-scoped PVs and snapshot contents also use this metadata.
func Validate(parent metav1.Object, kind string, meta metav1.ObjectMeta) error {
	if parent.GetUID() == "" || parent.GetName() == "" || parent.GetNamespace() == "" ||
		len(meta.OwnerReferences) != 0 || meta.Labels[LabelParentUID] != string(parent.GetUID()) ||
		meta.Labels[LabelParentKind] != kind || meta.Annotations[AnnotationParentName] != parent.GetName() ||
		meta.Annotations[AnnotationParentNamespace] != parent.GetNamespace() {
		return fmt.Errorf("resource %s/%s does not belong to %s %s/%s",
			meta.Namespace, meta.Name, kind, parent.GetNamespace(), parent.GetName())
	}
	return nil
}

// Name includes immutable parent identity even when readable prefixes truncate.
func Name(prefix string, parent metav1.Object, volume string) string {
	identity := []string{string(parent.GetUID()), parent.GetNamespace(), parent.GetName(), volume, prefix}
	sum := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
	return wranglername.SafeConcatName(prefix, hex.EncodeToString(sum[:16]))
}

func Selector(parent metav1.Object, kind string) string {
	return labels.Set{LabelParentUID: string(parent.GetUID()), LabelParentKind: kind}.AsSelector().String()
}
