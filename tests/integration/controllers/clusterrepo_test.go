package controllers

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	ctlcatalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/controller/master/mcmsettings"
)

var _ = ginkgo.Describe("verify cluster repos are patched", func() {
	var harvestercharts, partnercharts *catalogv1.ClusterRepo
	var clusterRepoController ctlcatalogv1.ClusterRepoController

	ginkgo.BeforeEach(func() {
		harvestercharts = &catalogv1.ClusterRepo{
			ObjectMeta: metav1.ObjectMeta{
				Name: "harvester-charts",
			},
			Spec: catalogv1.RepoSpec{},
		}

		partnercharts = &catalogv1.ClusterRepo{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rancher-partner-charts",
			},
			Spec: catalogv1.RepoSpec{},
		}

		clusterRepoController = scaled.Management.CatalogFactory.Catalog().V1().ClusterRepo()
		gomega.Eventually(func() error {
			_, err := clusterRepoController.Create(harvestercharts)
			return err
		}, "30s", "5s").ShouldNot(gomega.HaveOccurred())

		gomega.Eventually(func() error {
			_, err := clusterRepoController.Create(partnercharts)
			return err
		}, "30s", "5s").ShouldNot(gomega.HaveOccurred())

	})

	ginkgo.It("verify cluster repo annotations", func() {
		ginkgo.By("checking annotation on harvester-charts", func() {
			gomega.Eventually(func() error {
				obj, err := clusterRepoController.Get(harvestercharts.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if obj.Annotations == nil {
					return fmt.Errorf("expected to find annotations on cluster repo")
				}

				if val, ok := obj.Annotations[mcmsettings.HideClusterRepoKey]; ok && val == mcmsettings.HideClusterRepoValue {
					return nil
				}

				return fmt.Errorf("waiting for hide key/value: %s/%s to be annotated on cluster repo", mcmsettings.HideClusterRepoKey, mcmsettings.HideClusterRepoValue)
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By("checking annotation on rancher-partner-charts", func() {
			gomega.Consistently(func() error {
				obj, err := clusterRepoController.Get(partnercharts.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if obj.Annotations == nil {
					return nil
				}

				if _, ok := obj.Annotations[mcmsettings.HideClusterRepoKey]; !ok {
					return nil
				}

				return fmt.Errorf("key/value %s/%s should not have been annotated on cluster repo", mcmsettings.HideClusterRepoKey, mcmsettings.HideClusterRepoValue)
			}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
		})
	})
	ginkgo.AfterEach(func() {
		gomega.Eventually(func() error {
			return clusterRepoController.Delete(harvestercharts.Name, &metav1.DeleteOptions{})
		}, "30s", "5s").ShouldNot(gomega.HaveOccurred())

		gomega.Eventually(func() error {
			return clusterRepoController.Delete(partnercharts.Name, &metav1.DeleteOptions{})
		}, "30s", "5s").ShouldNot(gomega.HaveOccurred())
	})

})
