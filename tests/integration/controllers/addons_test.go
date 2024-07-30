package controllers

import (
	"fmt"
	"time"

	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	ctlappsv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/tests/integration/controllers/fake"
)

var _ = ginkgo.Describe("verify helm chart is create and addon gets to desired state", func() {

	var a *harvesterv1.Addon
	var app *catalogv1.App
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController
	var jobController ctlbatchv1.JobController
	var appController ctlappsv1.AppController

	var jobName string

	const managedChartKey = "catalog.cattle.io/managed"
	ginkgo.BeforeEach(func() {
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

		app = &catalogv1.App{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-addon-create",
				Namespace: "default",
			},
			Spec: catalogv1.ReleaseSpec{
				Chart: &catalogv1.Chart{
					Metadata: &catalogv1.Metadata{},
				},
			},
		}
		gomega.Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			jobController = scaled.Management.BatchFactory.Batch().V1().Job()
			appController = scaled.Management.CatalogFactory.Catalog().V1().App()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("checking helm and addon reconcile", func() {
		ginkgo.By("helm chart exists and has same spec as addon", func() {
			gomega.Eventually(func() error {
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
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check jobname is populated in helmchart", func() {
			gomega.Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Status.JobName != "" {
					jobName = h.Status.JobName
					ginkgo.GinkgoWriter.Printf("found job name: %s\n", jobName)
					return nil
				}

				return fmt.Errorf("waiting for jobname to be populated in helmchart status")
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})
		ginkgo.By("check job has been updated", func() {
			gomega.Eventually(func() error {
				j, err := jobController.Get(a.Namespace, jobName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if j.Status.CompletionTime != nil {
					ginkgo.GinkgoWriter.Printf("job status: %v \n", j.Status)
					return nil
				}

				return fmt.Errorf("waiting for job to complete")
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check status of addon", func() {
			gomega.Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if aObj.Status.Status != harvesterv1.AddonDeployed {
					return fmt.Errorf("waiting for addon to be deploy successfully. current status is %s", aObj.Status.Status)
				}
				return nil
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("watch status of addon to ensure it doesnt change", func() {
			gomega.Eventually(func() error {
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
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("creating an app is created", func() {
			gomega.Eventually(func() error {
				_, err := appController.Create(app)
				return err
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("ensuring app is patched", func() {
			gomega.Eventually(func() error {
				appObj, err := appController.Get(app.Namespace, app.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if val, ok := appObj.Spec.Chart.Metadata.Annotations[managedChartKey]; ok && val == "true" {
					return nil
				}

				return fmt.Errorf("waiting for key to be added to annotations on app")
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

	})

	ginkgo.AfterEach(func() {
		gomega.Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(gomega.HaveOccurred())
	})
})

var _ = ginkgo.Describe("addon and helm chart deletion", func() {
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

	ginkgo.It("verify helm deletion tasks", func() {
		ginkgo.By("create addon", func() {
			gomega.Eventually(func() error {
				addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
				helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
				_, err := addonController.Create(a)
				return err
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check helm chart object is created", func() {
			gomega.Eventually(func() error {
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
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check addon status is successful", func() {
			gomega.Eventually(func() error {
				a, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if a.Status.Status != harvesterv1.AddonDeployed {
					return fmt.Errorf("addon %s is not deployed successfully, status %v", a.Name, a.Status.Status)
				}
				return nil
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("delete addon object", func() {
			gomega.Eventually(func() error {
				return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("ensuring helm chart object is not found", func() {
			gomega.Eventually(func() error {
				_, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return nil
					}
					return err
				}

				// default scenario when hc is found
				return fmt.Errorf("found a helm chart, waiting for gc")
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})
	})
})

var _ = ginkgo.Describe("verify helm chart redeploy", func() {
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

	ginkgo.BeforeEach(func() {
		gomega.Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("reconcile helm chart recreation", func() {
		ginkgo.By("fetch helm chart and verify its spec", func() {
			gomega.Eventually(func() error {
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
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check addon status is successful", func() {
			gomega.Eventually(func() error {
				a, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if a.Status.Status != harvesterv1.AddonDeployed {
					return fmt.Errorf("addon %s is not deployed successfully, status %v", a.Name, a.Status.Status)
				}
				return nil
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("delete helm chart object", func() {
			gomega.Eventually(func() error {
				return helmController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("verify helm chart is recreated", func() {
			gomega.Eventually(func() error {
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
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		gomega.Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(gomega.HaveOccurred())
	})
})

var _ = ginkgo.Describe("perform addon upgrade", func() {
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

	ginkgo.BeforeEach(func() {
		gomega.Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("reconcile helm chart upgrade", func() {
		ginkgo.By("check helm chart object is created", func() {
			gomega.Eventually(func() error {
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
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check addon status is successful", func() {
			gomega.Eventually(func() error {
				a, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if a.Status.Status != harvesterv1.AddonDeployed {
					return fmt.Errorf("addon %s is not deployed successfully, status %v", a.Name, a.Status.Status)
				}
				return nil
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("update addon", func() {
			gomega.Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				aObj.Spec.Version = "0.2.0"
				aObj.Spec.Chart = "vm-import-controller-2"
				aObj.Spec.Repo = "http://harvester-cluster-repo.cattle-system.svc.cluster.local"
				_, err = addonController.Update(aObj)
				return err
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check helm chart got updated", func() {
			gomega.Eventually(func() error {
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
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		gomega.Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(gomega.HaveOccurred())
	})

})

// Failed Addon reconcile
var _ = ginkgo.Describe("verify helm chart is create and addon gets to failed state", func() {

	var a *harvesterv1.Addon
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController
	var jobName string
	ginkgo.BeforeEach(func() {
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

		gomega.Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("checking helm and addon reconcile", func() {
		ginkgo.By("helm chart exists and has same spec as addon", func() {
			gomega.Eventually(func() error {
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
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check jobname is populated in helmchart", func() {
			gomega.Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if h.Status.JobName != "" {
					jobName = h.Status.JobName
					ginkgo.GinkgoWriter.Printf("found job name: %s\n", jobName)
					//fmt.Printf("found job name: %s\n", h.Status.JobName)
					return nil
				}

				return fmt.Errorf("waiting for jobname to be populated in helmchart status")
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check status of addon", func() {
			gomega.Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if aObj.Status.Status == harvesterv1.AddonEnabling && harvesterv1.AddonOperationFailed.IsTrue(aObj) {
					return nil
				}
				return fmt.Errorf("waiting for addon to be deploy failed. current status is %s", aObj.Status.Status)
			}, "120s", "5s").ShouldNot(gomega.HaveOccurred())
		})

	})

	ginkgo.AfterEach(func() {
		gomega.Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(gomega.HaveOccurred())
	})
})

var _ = ginkgo.Describe("enable and disable successful addon", func() {

	var a *harvesterv1.Addon
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController
	ginkgo.BeforeEach(func() {
		a = &harvesterv1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-disable-success",
				Namespace: "default",
				Annotations: map[string]string{
					"harvesterhci.io/addon-defaults":          "ZGVmYXVsdFZhbHVlcwo=",
					"harvesterhci.io/addon-operation-timeout": "1",
				},
			},
			Spec: harvesterv1.AddonSpec{
				Chart:   "vm-import-controller",
				Repo:    "http://harvester-cluster-repo.cattle-system.svc",
				Version: "v0.1.0",
				Enabled: true,
			},
		}

		gomega.Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("check and disable addon", func() {

		ginkgo.By("helm chart exists and has same spec as addon", func() {
			gomega.Eventually(func() error {
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
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		// only when successful, next operation is allowed
		ginkgo.By("check addon status is successful", func() {
			gomega.Eventually(func() error {
				a, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if a.Status.Status != harvesterv1.AddonDeployed {
					return fmt.Errorf("addon %s is not deployed successfully, status %v", a.Name, a.Status.Status)
				}
				return nil
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("updating addon to disable", func() {
			gomega.Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("error fetching addon: %v", err)
				}
				a := aObj.DeepCopy()
				a.Spec.Enabled = false
				_, err = addonController.Update(a)
				return err
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("helm chart is removed", func() {
			gomega.Eventually(func() error {
				_, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return nil
					}
					return fmt.Errorf("error getting hemChart: %v", err)
				}

				return fmt.Errorf("waiting for helm chart to be removed")
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})

	})

	ginkgo.AfterEach(func() {
		gomega.Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(gomega.HaveOccurred())
	})
})

var _ = ginkgo.Describe("enable and disable failed addon", func() {

	var a *harvesterv1.Addon
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController
	ginkgo.BeforeEach(func() {
		a = &harvesterv1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-disable-fail",
				Namespace: "default",
				Annotations: map[string]string{
					"harvesterhci.io/addon-defaults":          "ZGVmYXVsdFZhbHVlcwo=",
					"harvesterhci.io/addon-operation-timeout": "1",
				},
			},
			Spec: harvesterv1.AddonSpec{
				Chart:   "vm-import-controller",
				Repo:    "http://harvester-cluster-repo.cattle-system.svc",
				Version: "v0.0.1", // non-existing version, make sure addon will fail
				Enabled: true,
			},
		}

		gomega.Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("check disable addon", func() {

		ginkgo.By("helm chart exists and has same spec as addon", func() {
			gomega.Eventually(func() error {
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
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check addon status is failed", func() {
			gomega.Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if aObj.Status.Status == harvesterv1.AddonEnabling && harvesterv1.AddonOperationFailed.IsTrue(aObj) {
					return nil
				}
				return fmt.Errorf("waiting for addon to be deploy failed. current status is %s", aObj.Status.Status)
			}, "120s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("updating addon to disable", func() {
			gomega.Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("error fetching addon: %v", err)
				}
				a := aObj.DeepCopy()
				// simulate user operation
				a.Annotations["harvesterhci.io/addon-last-operation"] = "disable"
				a.Annotations["harvesterhci.io/addon-last-operation-timestamp"] = time.Now().UTC().Format(time.RFC3339)
				a.Spec.Enabled = false
				_, err = addonController.Update(a)
				return err
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check addon disable status is failed", func() {
			gomega.Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if aObj.Status.Status == harvesterv1.AddonDisabled {
					return nil
				}
				return fmt.Errorf("waiting for addon to be disabled. current status is %s", aObj.Status.Status)
			}, "120s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("helm chart is removed", func() {
			gomega.Eventually(func() error {
				_, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return nil
					}
					return fmt.Errorf("error getting hemChart: %v", err)
				}

				return fmt.Errorf("waiting for helm chart to be removed")
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})

	})

	ginkgo.AfterEach(func() {
		gomega.Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(gomega.HaveOccurred())
	})
})

var _ = ginkgo.Describe("test addon upgrade fail", func() {
	var addonController ctlharvesterv1.AddonController
	var helmController ctlhelmv1.HelmChartController

	a := &harvesterv1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-addon-upgrade-2",
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

	ginkgo.BeforeEach(func() {
		gomega.Eventually(func() error {
			addonController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
			helmController = scaled.Management.HelmFactory.Helm().V1().HelmChart()
			_, err := addonController.Create(a)
			return err
		}).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("reconcile helm chart upgrade", func() {
		ginkgo.By("check helm chart object is created", func() {
			gomega.Eventually(func() error {
				h, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				aObj, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
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
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check addon status is successful", func() {
			gomega.Eventually(func() error {
				a, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if a.Status.Status != harvesterv1.AddonDeployed {
					return fmt.Errorf("addon %s is not deployed successfully, status %v", a.Name, a.Status.Status)
				}
				return nil
			}, "90s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("patching helm chart to fail before upgrade", func() {
			gomega.Eventually(func() error {
				hObj, err := helmController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if hObj.Annotations == nil {
					hObj.Annotations = make(map[string]string)
				}

				hObj.Annotations[fake.OverrideToFail] = "true"
				_, err = helmController.Update(hObj)
				return err
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("update addon", func() {
			gomega.Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				aObj.Spec.Version = "0.2.0"
				aObj.Spec.Chart = "vm-import-controller-2"
				aObj.Spec.Repo = "http://harvester-cluster-repo.cattle-system.svc.cluster.local"
				_, err = addonController.Update(aObj)
				return err
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check helm chart got updated", func() {
			gomega.Eventually(func() error {
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
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("check addon upgrade failed", func() {
			gomega.Eventually(func() error {
				aObj, err := addonController.Get(a.Namespace, a.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if aObj.Status.Status == harvesterv1.AddonUpdating && harvesterv1.AddonOperationFailed.IsTrue(aObj) {
					return nil
				}
				return fmt.Errorf("addon %s is not in correct state %v", aObj.Name, aObj.Status.Status)
			}, "60s", "5s").ShouldNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		gomega.Eventually(func() error {
			return addonController.Delete(a.Namespace, a.Name, &metav1.DeleteOptions{})
		}).ShouldNot(gomega.HaveOccurred())
	})

})
