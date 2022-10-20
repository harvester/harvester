package controllers

import (
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("verify dependency charts", func() {
	var a, dep *harvesterv1.Addon
	var addonController ctlharvesterv1.AddonController
	BeforeEach(func() {
		a = &harvesterv1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-addon-create-parent",
				Namespace: "default",
				Annotations: map[string]string{
					"harvesterhci.io/addon-defaults": "ZGVmYXVsdFZhbHVlcwo=",
				},
			},
			Spec: harvesterv1.AddonSpec{
				Chart:   "vm-import-controller",
				Repo:    "http://harvester-cluster-repo.cattle-system.svc",
				Version: "v0.1.0",
				Enabled: true,
				DependsOn: []harvesterv1.AddonRef{
					{
						Name:      "demo-addon-create-dep",
						Namespace: "default",
					},
				},
			},
		}

		dep = &harvesterv1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-addon-create-dep",
				Namespace: "default",
				Annotations: map[string]string{
					"harvesterhci.io/addon-defaults": "ZGVmYXVsdFZhbHVlcwo=",
				},
			},
			Spec: harvesterv1.AddonSpec{
				Chart:   "vm-import-controller-dep",
				Repo:    "http://harvester-cluster-repo.cattle-system.svc",
				Version: "v0.1.0",
				Enabled: false,
			},
		}
		Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			_, err := addonController.Create(a)
			if err != nil {
				return err
			}
			_, err = addonController.Create(dep)
			return err
		}).ShouldNot(HaveOccurred())
	})

	It("checking dependency and main addon are deployed", func() {
		By("checking dependency addon is deployed", func() {
			Eventually(func() error {
				depObj, err := addonController.Get(dep.Namespace, dep.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if depObj.Status.Status != harvesterv1.AddonDeployed {
					return fmt.Errorf("expected dependency addon to be deploy, but current state is %s", depObj.Status.Status)
				}

				return nil
			}, "120s", "10s").ShouldNot(HaveOccurred())

		})

		By("checking parent addon is deployed", func() {
			Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if aObj.Status.Status != harvesterv1.AddonDeployed {
					return fmt.Errorf("expected dependency addon to be deploy, but current state is %s", aObj.Status.Status)
				}

				return nil
			}, "120s", "10s").ShouldNot(HaveOccurred())

		})
	})

	AfterEach(func() {
		Eventually(func() error {
			err := addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			return addonController.Delete(dep.Namespace, dep.Name, &metav1.DeleteOptions{})

		}).ShouldNot(HaveOccurred())
	})

})
