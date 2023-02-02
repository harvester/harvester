package controllers

import (
	"fmt"
	"time"

	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

var _ = Describe("verify helm chart is create and addon gets to desired state", func() {

	var a *harvesterv1.Addon
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController
	var jobController ctlbatchv1.JobController
	var jobName string
	BeforeEach(func() {
		a = &harvesterv1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-addon-create",
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
			},
		}

		Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			jobController = scaled.Management.BatchFactory.Batch().V1().Job()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(HaveOccurred())
	})

	It("checking helm and addon reconcile", func() {
		By("helm chart exists and has same spec as addon", func() {
			Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Spec.Chart != a.Spec.Chart {
					return fmt.Errorf("expected chart name to be same")
				}

				if h.Spec.Version != a.Spec.Version {
					return fmt.Errorf("expected chart version to be same")
				}

				if h.Spec.Repo != a.Spec.Repo {
					return fmt.Errorf("expected chart repo to be same")
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("check jobname is populated in helmchart", func() {
			Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Status.JobName != "" {
					jobName = h.Status.JobName
					GinkgoWriter.Printf("found job name: %s\n", jobName)
					return nil
				}

				return fmt.Errorf("waiting for jobname to be populated in helmchart status")
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
		By("check job has been updated", func() {
			Eventually(func() error {
				j, err := jobController.Get(a.Namespace, jobName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if j.Status.CompletionTime != nil {
					GinkgoWriter.Printf("job status: %v \n", j.Status)
					return nil
				}

				return fmt.Errorf("waiting for job to complete")
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("check status of addon", func() {
			Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if aObj.Status.Status != harvesterv1.AddonDeployed {
					return fmt.Errorf("waiting for addon to be deploy successfully. current status is %s", aObj.Status.Status)
				}
				return nil
			}, "60s", "5s").ShouldNot(HaveOccurred())
		})

		By("watch status of addon to ensure it doesnt change", func() {
			Eventually(func() error {
				i := 0
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				initialStatus := aObj.Status.Status
				interval := time.Duration(500 * time.Millisecond)
				t := time.NewTicker(interval)
				for range t.C {
					i++
					aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
					if err != nil {
						return err
					}
					if aObj.Status.Status != initialStatus {
						return fmt.Errorf("addon status changing during reconcile")
					}
					if i > 10 {
						break
					}
				}
				return nil
			}, "60s", "5s").ShouldNot(HaveOccurred())
		})
	})

	AfterEach(func() {
		Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(HaveOccurred())
	})
})

var _ = Describe("addon and helm chart deletion", func() {
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController

	a := &harvesterv1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-addon-deleted",
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
		},
	}

	It("verify helm deletion tasks", func() {
		By("create addon", func() {
			Eventually(func() error {
				addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
				helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
				_, err := addonController.Create(a)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("check helm chart object is created", func() {
			Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Spec.Chart != a.Spec.Chart {
					return fmt.Errorf("expected chart name to be same")
				}

				if h.Spec.Version != a.Spec.Version {
					return fmt.Errorf("expected chart version to be same")
				}

				if h.Spec.Repo != a.Spec.Repo {
					return fmt.Errorf("expected chart repo to be same")
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("delete addon object", func() {
			Eventually(func() error {
				return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("ensuring helm chart object is not found", func() {
			Eventually(func() error {
				_, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return nil
					}
					return err
				}

				// default scenario when hc is found
				return fmt.Errorf("found a helm chart, waiting for gc")
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
	})
})

var _ = Describe("verify helm chart redeploy", func() {
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController

	a := &harvesterv1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-addon-redeploy",
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
		},
	}

	BeforeEach(func() {
		Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(HaveOccurred())
	})

	It("reconcile helm chart recreation", func() {
		By("fetch helm chart and verify its spec", func() {
			Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Spec.Chart != a.Spec.Chart {
					return fmt.Errorf("expected chart name to be same")
				}

				if h.Spec.Version != a.Spec.Version {
					return fmt.Errorf("expected chart version to be same")
				}

				if h.Spec.Repo != a.Spec.Repo {
					return fmt.Errorf("expected chart repo to be same")
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("delete helm chart object", func() {
			Eventually(func() error {
				return helmController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("verify helm chart is recreated", func() {
			Eventually(func() error {
				hc, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if err != nil {
					return err
				}

				if hc.Spec.Chart != a.Spec.Chart {
					return fmt.Errorf("expected chart name to be same")
				}

				if hc.Spec.Version != a.Spec.Version {
					return fmt.Errorf("expected chart version to be same")
				}

				if hc.Spec.Repo != a.Spec.Repo {
					return fmt.Errorf("expected chart repo to be same")
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
	})

	AfterEach(func() {
		Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(HaveOccurred())
	})
})

var _ = Describe("perform addon upgrade", func() {
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController

	a := &harvesterv1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-addon-upgrade",
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
		},
	}

	BeforeEach(func() {
		Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(HaveOccurred())
	})

	It("reconcile helm chart upgrade", func() {
		By("check helm chart object is created", func() {
			Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Spec.Chart != a.Spec.Chart {
					return fmt.Errorf("expected chart name to be same")
				}

				if h.Spec.Version != a.Spec.Version {
					return fmt.Errorf("expected chart version to be same")
				}

				if h.Spec.Repo != a.Spec.Repo {
					return fmt.Errorf("expected chart repo to be same")
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("update addon", func() {
			Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				aObj.Spec.Version = "0.2.0"
				aObj.Spec.Chart = "vm-import-controller-2"
				aObj.Spec.Repo = "http://harvester-cluster-repo.cattle-system.svc.cluster.local"
				_, err = addonController.Update(aObj)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("check helm chart got updated", func() {
			Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Spec.Chart != aObj.Spec.Chart {
					return fmt.Errorf("expected chart name to be same")
				}

				if h.Spec.Version != aObj.Spec.Version {
					return fmt.Errorf("expected chart version to be same")
				}

				if h.Spec.Repo != aObj.Spec.Repo {
					return fmt.Errorf("expected chart repo to be same")
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
	})

	AfterEach(func() {
		Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(HaveOccurred())
	})

})

// Failed Addon reconcile
var _ = Describe("verify helm chart is create and addon gets to failed state", func() {

	var a *harvesterv1.Addon
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController
	var jobName string
	BeforeEach(func() {
		a = &harvesterv1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-addon-fail",
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
			},
		}

		Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(HaveOccurred())
	})

	It("checking helm and addon reconcile", func() {
		By("helm chart exists and has same spec as addon", func() {
			Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Spec.Chart != a.Spec.Chart {
					return fmt.Errorf("expected chart name to be same")
				}

				if h.Spec.Version != a.Spec.Version {
					return fmt.Errorf("expected chart version to be same")
				}

				if h.Spec.Repo != a.Spec.Repo {
					return fmt.Errorf("expected chart repo to be same")
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("check jobname is populated in helmchart", func() {
			Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Status.JobName != "" {
					jobName = h.Status.JobName
					GinkgoWriter.Printf("found job name: %s\n", jobName)
					//fmt.Printf("found job name: %s\n", h.Status.JobName)
					return nil
				}

				return fmt.Errorf("waiting for jobname to be populated in helmchart status")
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("check status of addon", func() {
			Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if aObj.Status.Status != harvesterv1.AddonFailed {
					return fmt.Errorf("waiting for addon to be deploy successfully. current status is %s", aObj.Status.Status)
				}
				return nil
			}, "120s", "5s").ShouldNot(HaveOccurred())
		})

	})

	AfterEach(func() {
		Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(HaveOccurred())
	})
})

var _ = Describe("enable and disable successful addon", func() {

	var a *harvesterv1.Addon
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController
	BeforeEach(func() {
		a = &harvesterv1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-disable-success",
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
			},
		}

		Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(HaveOccurred())
	})

	It("check disable addon", func() {

		By("helm chart exists and has same spec as addon", func() {
			Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Spec.Chart != a.Spec.Chart {
					return fmt.Errorf("expected chart name to be same")
				}

				if h.Spec.Version != a.Spec.Version {
					return fmt.Errorf("expected chart version to be same")
				}

				if h.Spec.Repo != a.Spec.Repo {
					return fmt.Errorf("expected chart repo to be same")
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("updating addon to disable", func() {
			Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("error fetching addon: %v", err)
				}
				aObj.Spec.Enabled = false
				_, err = addonController.Update(aObj)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("helm chart is missing", func() {
			Eventually(func() error {
				_, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return nil
					}
					return fmt.Errorf("error getting hemChart: %v", err)
				}

				return fmt.Errorf("waiting for helm chart to be removed")
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

	})

	AfterEach(func() {
		Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(HaveOccurred())
	})
})

var _ = Describe("enable and disable failed addon", func() {

	var a *harvesterv1.Addon
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController
	BeforeEach(func() {
		a = &harvesterv1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-disable-fail",
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
			},
		}

		Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(HaveOccurred())
	})

	It("check disable addon", func() {

		By("helm chart exists and has same spec as addon", func() {
			Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Spec.Chart != a.Spec.Chart {
					return fmt.Errorf("expected chart name to be same")
				}

				if h.Spec.Version != a.Spec.Version {
					return fmt.Errorf("expected chart version to be same")
				}

				if h.Spec.Repo != a.Spec.Repo {
					return fmt.Errorf("expected chart repo to be same")
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("updating addon to disable", func() {
			Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("error fetching addon: %v", err)
				}
				aObj.Spec.Enabled = false
				_, err = addonController.Update(aObj)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("helm chart is missing", func() {
			Eventually(func() error {
				_, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return nil
					}
					return fmt.Errorf("error getting hemChart: %v", err)
				}

				return fmt.Errorf("waiting for helm chart to be removed")
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

	})

	AfterEach(func() {
		Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(HaveOccurred())
	})
})
