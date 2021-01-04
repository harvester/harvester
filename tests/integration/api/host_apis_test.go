package api_test

import (
	"net/http"

	. "github.com/onsi/ginkgo"

	"github.com/rancher/harvester/tests/framework/cluster"
	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/helper"
)

var _ = Describe("verify host APIs", func() {

	Context("operate via steve API", func() {

		var nodesAPI string

		BeforeEach(func() {

			nodesAPI = helper.BuildAPIURL("v1", "nodes")

		})

		Specify("verify host", func() {

			By("get hosts", func() {

				nodes, respCode, respBody, err := helper.GetCollection(nodesAPI)
				MustRespCodeIs(http.StatusOK, "get host", err, respCode, respBody)
				MustEqual(len(nodes.Data), cluster.DefaultWorkers+cluster.DefaultControlPlanes)

			})

		})

	})

})
