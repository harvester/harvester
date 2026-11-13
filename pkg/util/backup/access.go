package backup

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"reflect"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// JobServiceAccount is local to each Job namespace. The chart installs its
// Secret-reading Role in longhorn-system; controllers bind individual accounts.
const JobServiceAccount = "harvester-repository-job"

//go:embed manifests/*.yaml
var accessManifests embed.FS

var accessTemplates = template.Must(template.ParseFS(accessManifests, "manifests/*.yaml"))

// EnsureJobAccess provisions access once per namespace, not once per Job.
// Namespace ownership removes the cross-namespace binding when its subject's
// namespace is deleted, including when a namespace is later recreated.
func EnsureJobAccess(ctx context.Context, client kubernetes.Interface, namespace string) error {
	ns, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if ns.DeletionTimestamp != nil {
		return fmt.Errorf("repository Job namespace is terminating")
	}
	if err := ensureJobAccount(ctx, client, ns); err != nil {
		return err
	}
	return ensureJobBinding(ctx, client, ns)
}

func ensureJobAccount(
	ctx context.Context, client kubernetes.Interface, namespace *corev1.Namespace,
) error {
	wanted := &corev1.ServiceAccount{}
	if err := renderAccess("serviceaccount.yaml", namespace, wanted); err != nil {
		return err
	}
	accounts := client.CoreV1().ServiceAccounts(wanted.Namespace)
	_, err := accounts.Create(ctx, wanted, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	stored, err := accounts.Get(ctx, wanted.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(metav1.GetControllerOf(stored), metav1.GetControllerOf(wanted)) {
		return fmt.Errorf("repository Job ServiceAccount belongs to a different owner")
	}
	return nil
}

func ensureJobBinding(
	ctx context.Context, client kubernetes.Interface, namespace *corev1.Namespace,
) error {
	wanted := &rbacv1.RoleBinding{}
	if err := renderAccess("rolebinding.yaml", namespace, wanted); err != nil {
		return err
	}
	bindings := client.RbacV1().RoleBindings(wanted.Namespace)
	_, err := bindings.Create(ctx, wanted, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	stored, err := bindings.Get(ctx, wanted.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	owner := metav1.GetControllerOf(wanted)
	if !reflect.DeepEqual(metav1.GetControllerOf(stored), owner) || stored.RoleRef != wanted.RoleRef {
		return fmt.Errorf("repository Job RoleBinding has an unexpected owner or role")
	}
	if reflect.DeepEqual(stored.Subjects, wanted.Subjects) {
		return nil
	}
	stored.Subjects = wanted.Subjects
	_, err = bindings.Update(ctx, stored, metav1.UpdateOptions{})
	return err
}

func renderAccess(name string, namespace *corev1.Namespace, object interface{}) error {
	var manifest bytes.Buffer
	if err := accessTemplates.ExecuteTemplate(&manifest, name, namespace); err != nil {
		return fmt.Errorf("rendering repository Job %s: %w", name, err)
	}
	if err := yaml.UnmarshalStrict(manifest.Bytes(), object); err != nil {
		return fmt.Errorf("decoding repository Job %s: %w", name, err)
	}
	return nil
}
