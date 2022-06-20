package dsl

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MustPVCDeleted(controller v1.PersistentVolumeClaimController, namespace, name string) {
	gomega.Eventually(func() bool {
		_, err := controller.Get(namespace, name, metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return true
		}
		ginkgo.GinkgoT().Logf("PVC %s still exists: %v", name, err)
		return false
	}, pvcTimeoutInterval, pvcPollingInterval).Should(gomega.BeTrue())
}
