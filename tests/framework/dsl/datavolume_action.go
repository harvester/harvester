package dsl

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctldatavolumev1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
)

func MustDataVolumeDeleted(controller ctldatavolumev1.DataVolumeController, namespace, name string) {
	gomega.Eventually(func() bool {
		_, err := controller.Get(namespace, name, metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return true
		}
		ginkgo.GinkgoT().Logf("datavolume %s is still exist: %v", name, err)
		return false
	}, dvTimeoutInterval, dvPollingInterval).Should(gomega.BeTrue())
}
