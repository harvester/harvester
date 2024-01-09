package controllers

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/util/drainhelper"
	"github.com/harvester/harvester/tests/framework/dsl"
)

var _ = ginkgo.Describe("verify node drain apis", func() {
	var node corev1.Node

	ginkgo.BeforeEach(func() {
		gomega.Eventually(func() error {
			nodeList, err := scaled.CoreFactory.Core().V1().Node().List(metav1.ListOptions{})
			if err != nil {
				return err
			}
			for _, v := range nodeList.Items {
				_, ok := v.Labels["node-role.kubernetes.io/control-plane"]
				if !ok {
					node = v
					return nil
				}
			}
			return fmt.Errorf("no worker node found in cluster")
		}).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("drain node using drainhelper", func() {
		ginkgo.By("draining nodes", func() {
			err := drainhelper.DrainNode(testCtx, cfg, &node)
			dsl.MustNotError(err)
		})

		gomega.Eventually(func() error {
			nodeObj, err := scaled.CoreFactory.Core().V1().Node().Get(node.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !nodeObj.Spec.Unschedulable {
				return fmt.Errorf("expected the node to be unschedulable")
			}

			if len(nodeObj.Spec.Taints) == 0 {
				return fmt.Errorf("expected to find taints on the node")
			}

			for _, v := range nodeObj.Spec.Taints {
				if v.Key == "node.kubernetes.io/unschedulable" && v.Effect == corev1.TaintEffectNoSchedule {
					return nil
				}
			}

			return fmt.Errorf("expected to find taint for no schedule")
		}, "30s", "5s").ShouldNot(gomega.HaveOccurred())

	})
})
