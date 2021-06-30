package dsl

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	ctldatavolumev1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
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

func MustDataVolumeSucceeded(controller ctldatavolumev1.DataVolumeController, namespace, name string) {
	AfterDataVolumeExist(controller, namespace, name, func(dv *cdiv1beta1.DataVolume) bool {
		return dv.Status.Phase == cdiv1beta1.Succeeded
	})
}

func MustDataVolumeUnowned(controller ctldatavolumev1.DataVolumeController, namespace, name string) {
	AfterDataVolumeExist(controller, namespace, name, func(dv *cdiv1beta1.DataVolume) bool {
		var annotations = dv.GetAnnotations()
		var _, ok = annotations[ref.AnnotationSchemaOwnerKeyName]
		return !ok
	})
}

func AfterDataVolumeExist(controller ctldatavolumev1.DataVolumeController, namespace, name string,
	callback func(dv *cdiv1beta1.DataVolume) bool) {
	gomega.Eventually(func() bool {
		var dv, err = controller.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			ginkgo.GinkgoT().Logf("failed to get data volume: %v", err)
			return false
		}
		return callback(dv)
	}, dvTimeoutInterval, dvPollingInterval).Should(gomega.BeTrue())
}
