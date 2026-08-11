package fakeclients

import (
	"context"

	webhookpkg "github.com/rancher/wrangler/v3/pkg/webhook"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	corefake "k8s.io/client-go/kubernetes/fake"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	k8stesting "k8s.io/client-go/testing"

	whtypes "github.com/harvester/harvester/pkg/webhook/types"
)

// AllowedSARClient returns a fake SAR client whose reactor always returns Allowed=true.
func AllowedSARClient() authorizationv1client.SubjectAccessReviewInterface {
	return SARClientSet(true).AuthorizationV1().SubjectAccessReviews()
}

// DeniedSARClient returns a fake SAR client whose reactor always returns Allowed=false.
func DeniedSARClient() authorizationv1client.SubjectAccessReviewInterface {
	return SARClientSet(false).AuthorizationV1().SubjectAccessReviews()
}

// SARClientSet returns a fake Kubernetes clientset whose SAR reactor returns the given Allowed status.
func SARClientSet(allowed bool) *corefake.Clientset {
	cs := corefake.NewClientset()
	cs.Fake.PrependReactor("create", "subjectaccessreviews",
		func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, &authorizationv1.SubjectAccessReview{
				Status: authorizationv1.SubjectAccessReviewStatus{Allowed: allowed},
			}, nil
		})
	return cs
}

// NewFakeRequest returns a fake webhook request with the given username.
func NewFakeRequest(username string) *whtypes.Request {
	return whtypes.NewRequest(&webhookpkg.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authenticationv1.UserInfo{Username: username},
		},
		Context: context.Background(),
	}, nil)
}
