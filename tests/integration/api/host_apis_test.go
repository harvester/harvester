package api_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	"github.com/harvester/harvester/tests/framework/cluster"
	. "github.com/harvester/harvester/tests/framework/dsl"
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

		Specify("verify host maintenance mode", func() {

			nodes, respCode, respBody, err := helper.GetCollection(nodesAPI)
			MustRespCodeIs(http.StatusOK, "get host", err, respCode, respBody)
			nodeName := nodes.Data[0].ID
			nodeObjectAPI := fmt.Sprintf("%s/%s", nodesAPI, nodeName)

			By("enable maintenance mode of the host", func() {
				respCode, respBody, err = helper.PostAction(nodeObjectAPI, "enableMaintenanceMode")
				MustRespCodeIs(http.StatusNoContent, "post enableMaintenanceMode action", err, respCode, respBody)
			})

			By("then the node is unschedulable and maintain-status is set")
			MustFinallyBeTrue(func() bool {
				var retNode corev1.Node
				respCode, respBody, err = helper.GetObject(nodeObjectAPI, &retNode)
				MustRespCodeIs(http.StatusOK, "get host", err, respCode, respBody)
				Expect(retNode.Spec.Unschedulable).To(BeEquivalentTo(true))
				maintainStatus := retNode.Annotations[ctlnode.MaintainStatusAnnotationKey]
				return maintainStatus == ctlnode.MaintainStatusComplete
			}, 10*time.Second, 1*time.Second)

			By("disable maintenance mode of the host", func() {
				respCode, respBody, err = helper.PostAction(nodeObjectAPI, "disableMaintenanceMode")
				MustRespCodeIs(http.StatusNoContent, "post disableMaintenanceMode action", err, respCode, respBody)
			})

			By("then the node is schedulable and maintain-status is removed", func() {
				var retNode corev1.Node
				respCode, respBody, err = helper.GetObject(nodeObjectAPI, &retNode)
				MustRespCodeIs(http.StatusOK, "get host", err, respCode, respBody)
				Expect(retNode.Spec.Unschedulable).To(BeEquivalentTo(false))
				Expect(retNode.Annotations[ctlnode.MaintainStatusAnnotationKey]).To(BeEmpty())
			})

		})

	})

})
