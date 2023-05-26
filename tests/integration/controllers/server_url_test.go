package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlmgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/controller/master/mcmsettings"
)

const (
	address       = "172.19.109.0"
	addressUpdate = "172.19.109.1"
)

var _ = Describe("verify svc kube-system/ingress-expose is synced to server-url setting", func() {

	var svc *corev1.Service
	var setting *managementv3.Setting
	var svcController ctlcorev1.ServiceController
	var settingController ctlmgmtv3.SettingController

	BeforeEach(func() {
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress-expose",
				Namespace: "kube-system",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Port: 8443,
					},
				},
				Type: corev1.ServiceTypeLoadBalancer,
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP: address,
						},
					},
				}},
		}

		setting = &managementv3.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: "server-url",
			},
		}

		svcController = scaled.Management.CoreFactory.Core().V1().Service()
		settingController = scaled.Management.RancherManagementFactory.Management().V3().Setting()

		Eventually(func() error {
			svcObj, err := svcController.Create(svc)
			if err != nil {
				return err
			}
			svcObj.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: address}}
			_, err = svcController.UpdateStatus(svcObj)
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			_, err := settingController.Create(setting)
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("verify sync of setting", func() {
		By("checking value of server-url", func() {
			Eventually(func() error {
				settingObj, err := settingController.Get(setting.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				val, ok := settingObj.Annotations[mcmsettings.PatchedByHarvesterKey]

				if ok && val == mcmsettings.PatchedByHarvesterValue && settingObj.Value == fmt.Sprintf("https://%s", address) {
					return nil
				}
				return fmt.Errorf("waiting for setting value to be correct. current value: %s", settingObj.Value)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("changing ingress-expose address", func() {
			Eventually(func() error {
				svcObj, err := svcController.Get(svc.Namespace, svc.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				svcObj.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: addressUpdate}}
				_, err = svcController.UpdateStatus(svcObj)
				return err
			}).ShouldNot(HaveOccurred())
		})

		By("checking value of server-url has not changed", func() {
			Consistently(func() error {
				settingObj, err := settingController.Get(setting.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				val, ok := settingObj.Annotations[mcmsettings.PatchedByHarvesterKey]

				if ok && val == mcmsettings.PatchedByHarvesterValue && settingObj.Value == fmt.Sprintf("https://%s", address) {
					return nil
				}
				return fmt.Errorf("expected server-url to not have changed: %s", settingObj.Value)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
	})
	AfterEach(func() {
		Eventually(func() error {
			return svcController.Delete(svc.Namespace, svc.Name, &metav1.DeleteOptions{})
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return settingController.Delete(setting.Name, &metav1.DeleteOptions{})
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
})
