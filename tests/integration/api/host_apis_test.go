package api_test

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"

	nodeapi "github.com/harvester/harvester/pkg/api/node"
	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	"github.com/harvester/harvester/tests/framework/cluster"
	"github.com/harvester/harvester/tests/framework/helper"
)

var _ = Describe("verify host APIs", func() {

	Context("operate via steve API", func() {

		var nodesAPI string

		BeforeEach(func() {

			nodesAPI = helper.BuildAPIURL("v1", "nodes", options.HTTPSListenPort)

		})

		Specify("verify host", func() {

			By("get hosts", func() {

				nodes, respCode, respBody, err := helper.GetCollection(nodesAPI)
				MustRespCodeIs(http.StatusOK, "get host", err, respCode, respBody)
				MustEqual(len(nodes.Data), cluster.DefaultWorkers+cluster.DefaultControlPlanes)

			})

		})

		Specify("verify maintenance possible for worker nodes", func() {
			nodes, respCode, respBody, err := helper.GetCollection(nodesAPI)
			MustRespCodeIs(http.StatusOK, "get host", err, respCode, respBody)
			nodeName := nodes.Data[1].ID
			nodeObjectAPI := fmt.Sprintf("%s/%s", nodesAPI, nodeName)

			By("enable maintenance mode of the host", func() {
				MustFinallyBeTrue(func() bool {
					respCode, respBody, err = helper.PostAction(nodeObjectAPI, "maintenancePossible")
					return CheckRespCodeIs(http.StatusNoContent, "maintenancePossible action", err, respCode, respBody)
				}, 30*time.Second, 5*time.Second)
			})
		})

		Specify("verify maintenance possible for controlplane nodes", func() {
			nodes, respCode, respBody, err := helper.GetCollection(nodesAPI)
			MustRespCodeIs(http.StatusOK, "get host", err, respCode, respBody)
			nodeName := nodes.Data[0].ID
			nodeObjectAPI := fmt.Sprintf("%s/%s", nodesAPI, nodeName)

			By("enable maintenance mode of the host", func() {
				MustFinallyBeTrue(func() bool {
					respCode, respBody, err = helper.PostAction(nodeObjectAPI, "maintenancePossible")
					return CheckRespCodeIs(http.StatusInternalServerError, "maintenancePossible action", err, respCode, respBody)
				}, 30*time.Second, 5*time.Second)
			})
		})

		Specify("verify host maintenance mode for worker nodes", func() {

			nodes, respCode, respBody, err := helper.GetCollection(nodesAPI)
			MustRespCodeIs(http.StatusOK, "get host", err, respCode, respBody)
			nodeName := nodes.Data[1].ID
			nodeObjectAPI := fmt.Sprintf("%s/%s", nodesAPI, nodeName)

			By("enable maintenance mode of the host", func() {
				MustFinallyBeTrue(func() bool {
					respCode, respBody, err = helper.PostObjectAction(nodeObjectAPI, nodeapi.MaintenanceModeInput{Force: ""}, "enableMaintenanceMode")
					return CheckRespCodeIs(http.StatusNoContent, "post enableMaintenanceMode action", err, respCode, respBody)
				}, 30*time.Second, 5*time.Second)
			})

			By("then the node is unschedulable and maintain-status is set", func() {
				Eventually(func() error {
					var retNode corev1.Node
					respCode, respBody, err = helper.GetObject(nodeObjectAPI, &retNode)
					if err != nil {
						return fmt.Errorf("got error %v, respBody: %s", err, string(respBody))
					}
					if respCode != http.StatusOK {
						return fmt.Errorf("expected status http.StatusOK, but got %d", respCode)
					}

					if !retNode.Spec.Unschedulable {
						return fmt.Errorf("expected node to be unschedulable")
					}

					_, ok := retNode.Annotations[ctlnode.MaintainStatusAnnotationKey]
					if !ok {
						return fmt.Errorf("unable to find maintenance annotation")
					}
					return nil
				}, "300s", "10s").ShouldNot(HaveOccurred())
			})

			By("disable maintenance mode of the host", func() {
				respCode, respBody, err = helper.PostAction(nodeObjectAPI, "disableMaintenanceMode")
				MustRespCodeIs(http.StatusNoContent, "post disableMaintenanceMode action", err, respCode, respBody)
			})

			By("then the node is schedulable and maintain-status is removed", func() {
				MustFinallyBeTrue(func() bool {
					var retNode corev1.Node
					respCode, respBody, err = helper.GetObject(nodeObjectAPI, &retNode)
					if !CheckRespCodeIs(http.StatusOK, "get host", err, respCode, respBody) {
						return false
					}
					if !Expect(retNode.Spec.Unschedulable).To(BeEquivalentTo(false)) {
						return false
					}
					return Expect(retNode.Annotations[ctlnode.MaintainStatusAnnotationKey]).To(BeEmpty())
				}, 30*time.Second, 1*time.Second)

			})

		})

		Specify("verify host maintenance mode for controlplane nodes", func() {

			nodes, respCode, respBody, err := helper.GetCollection(nodesAPI)
			MustRespCodeIs(http.StatusOK, "get host", err, respCode, respBody)
			nodeName := nodes.Data[0].ID
			nodeObjectAPI := fmt.Sprintf("%s/%s", nodesAPI, nodeName)

			By("patch controlplane node to ensure labels are present", func() {
				Eventually(func() error {
					var retNode corev1.Node
					respCode, respBody, err = helper.GetObject(nodeObjectAPI, &retNode)
					if err != nil {
						return fmt.Errorf("got error %v, respBody: %s", err, string(respBody))
					}
					if respCode != http.StatusOK {
						return fmt.Errorf("expected status http.StatusOK, but got %d", respCode)
					}

					if retNode.Labels == nil {
						retNode.Labels = make(map[string]string)
					}

					retNode.Labels["node-role.kubernetes.io/control-plane"] = "true"
					_, _, err = helper.PostObject(nodeObjectAPI, retNode)
					return err
				}, "30s", "5s").ShouldNot(HaveOccurred())
			})

			By("attempting to enable maintenance mode on controlplane host", func() {
				respCode, respBody, err = helper.PostObjectAction(nodeObjectAPI, nodeapi.MaintenanceModeInput{Force: ""}, "enableMaintenanceMode")
				MustRespCodeIs(http.StatusInternalServerError, "enable maintenance", err, respCode, respBody)
			})

			By("then the node maintain-status is not set", func() {
				Consistently(func() error {
					var retNode corev1.Node
					respCode, respBody, err = helper.GetObject(nodeObjectAPI, &retNode)
					if err != nil {
						return fmt.Errorf("got error %v, respBody: %s", err, string(respBody))
					}
					if respCode != http.StatusOK {
						return fmt.Errorf("expected status http.StatusOK, but got %d", respCode)
					}

					if retNode.Spec.Unschedulable {
						return fmt.Errorf("expected node to be schedulable")
					}

					_, ok := retNode.Annotations[ctlnode.MaintainStatusAnnotationKey]
					if ok {
						return fmt.Errorf("should not find maintenance annotation")
					}
					return nil
				}, "300s", "10s").ShouldNot(HaveOccurred())
			})

		})
	})

})
