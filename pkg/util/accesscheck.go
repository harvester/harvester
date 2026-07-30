package util

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// Kubernetes resource API verbs for use with CheckObjectAccess.
const (
	VerbGet = "get"
)

// GVRs used with CheckObjectAccess.
var (
	ServiceGVR             = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	PVCGVR                 = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}
	VirtualMachineImageGVR = schema.GroupVersionResource{Group: "harvesterhci.io", Version: "v1beta1", Resource: "virtualmachineimages"}
)

type ResourceAccessCheck struct {
	SAR       authorizationv1client.SubjectAccessReviewInterface
	Username  string
	Groups    []string
	Verb      string
	GVR       schema.GroupVersionResource
	Namespace string
	Name      string
}

// CheckObjectAccess checks whether the given user/groups can perform the requested verb on a resource.
func CheckObjectAccess(ctx context.Context, check ResourceAccessCheck) (bool, error) {
	review, err := check.SAR.Create(ctx, &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: check.Namespace,
				Verb:      check.Verb,
				Group:     check.GVR.Group,
				Version:   check.GVR.Version,
				Resource:  check.GVR.Resource,
				Name:      check.Name,
			},
			User:   check.Username,
			Groups: check.Groups,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to check access: %w", err)
	}
	return review.Status.Allowed, nil
}
