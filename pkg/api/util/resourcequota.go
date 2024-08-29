package util

import (
	"context"

	"github.com/sirupsen/logrus"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func CanUpdateResourceQuota(clientSet kubernetes.Clientset, namespace string, user string) (bool, error) {
	review, err := clientSet.AuthorizationV1().SubjectAccessReviews().Create(
		context.TODO(),
		&authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Namespace: namespace,
					Verb:      "update",
					Group:     v1beta1.SchemeGroupVersion.Group,
					Version:   v1beta1.SchemeGroupVersion.Version,
					Resource:  v1beta1.ResourceQuotaResourceName,
				},
				User: user,
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": namespace,
			"user":      user,
		}).Error("Failed to check update resource quota")
		return false, err
	}
	return review.Status.Allowed, nil
}
