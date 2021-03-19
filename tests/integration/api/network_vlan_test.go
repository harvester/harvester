package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"
	"github.com/rancher/apiserver/pkg/types"
	netv1alpha "github.com/rancher/harvester-network-controller/pkg/apis/network.harvester.cattle.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/helper"
)

const (
	defaultNIC = "eth0"
	validNIC   = "eth1"
	invalidNIC = "invalid"
)

var _ = PDescribe("verify VLAN Network", func() {
	var cnAPI, nnAPI string
	var nnCollection nodeNetworkCollection

	BeforeEach(func() {
		cnAPI = helper.BuildAPIURL("v1", "network.harvester.cattle.io.clusternetworks", options.HTTPSListenPort)
		nnAPI = helper.BuildAPIURL("v1", "network.harvester.cattle.io.nodenetworks", options.HTTPSListenPort)
	})

	Context("enable VLAN network", func() {
		var vlanCn netv1alpha.ClusterNetwork
		var vlanCnURL string

		BeforeEach(func() {
			vlanCnURL = fmt.Sprintf("%s/%s/%s", cnAPI, namespace, "vlan")
			respCode, respBody, err := helper.GetObject(vlanCnURL, &vlanCn)
			MustRespCodeIs(http.StatusOK, "get vlan clusternetwork", err, respCode, respBody)
		})

		It("enable VLAN network", func() {
			By("enable VLAN network")
			vlanCn.Enable = true
			config := map[string]string{
				netv1alpha.KeyDefaultNIC: defaultNIC,
			}
			vlanCn.Config = config
			respCode, respBody, err := helper.PutObject(vlanCnURL, &vlanCn)
			MustRespCodeIs(http.StatusOK, "enable VLAN network", err, respCode, respBody)

			By("validate the number of nodenetworks and the status of nodenetworks")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err := listNodeNetworks(nnAPI, &nnCollection)
				MustRespCodeIs(http.StatusOK, "list nodenetworks", err, respCode, respBody)

				nodeAPI := helper.BuildAPIURL("v1", "nodes", options.HTTPSListenPort)
				nodesCollection, respCode, respBody, err := helper.GetCollection(nodeAPI)
				MustRespCodeIs(http.StatusOK, "list nodes", err, respCode, respBody)

				if !(len(nnCollection.NodeNetworks) == len(nodesCollection.Data)) {
					return false
				}

				for _, nn := range nnCollection.NodeNetworks {
					return validateStatus(nn, corev1.ConditionTrue)
				}

				return true
			})
		})
	})

	Context("modify one of nodenetworks' physical NIC", func() {
		var nn *netv1alpha.NodeNetwork
		var nnURL string

		BeforeEach(func() {
			respCode, respBody, err := listNodeNetworks(nnAPI, &nnCollection)
			MustRespCodeIs(http.StatusOK, "list nodenetworks", err, respCode, respBody)
			MustEqual(len(nnCollection.NodeNetworks) > 0, true)

			nn = nnCollection.NodeNetworks[0]
			nnURL = fmt.Sprintf("%s/%s/%s", nnAPI, namespace, nn.Name)

			respCode, respBody, err = helper.GetObject(nnURL, nn)
			MustRespCodeIs(http.StatusOK, "get nodenetwork", err, respCode, respBody)
		})

		It("reset NIC spec", func() {
			nn.Spec.NIC = ""
			respCode, respBody, err := helper.PutObject(nnURL, nn)
			MustRespCodeIs(http.StatusOK, "delete physical NIC", err, respCode, respBody)

			MustFinallyBeTrue(func() bool {
				respCode, respBody, err := helper.GetObject(nnURL, nn)
				MustRespCodeIs(http.StatusOK, "get nodenetwork", err, respCode, respBody)
				return validateStatus(nn, corev1.ConditionFalse)
			})
		})

		It("modify to another valid physical NIC", func() {
			nn.Spec.NIC = validNIC
			respCode, respBody, err := helper.PutObject(nnURL, nn)
			MustRespCodeIs(http.StatusOK, "modify physical NIC as another valid NIC", err, respCode, respBody)

			MustFinallyBeTrue(func() bool {
				respCode, respBody, err := helper.GetObject(nnURL, nn)
				MustRespCodeIs(http.StatusOK, "get nodenetwork", err, respCode, respBody)
				return validateStatus(nn, corev1.ConditionTrue) && len(nn.Status.NetworkLinkStatus) != 0
			})
		})

		It("modify to a invalid physical NIC", func() {
			nn.Spec.NIC = invalidNIC
			respCode, respBody, err := helper.PutObject(nnURL, nn)
			MustRespCodeIs(http.StatusOK, "modify physical NIC as an invalid NIC", err, respCode, respBody)

			MustFinallyBeTrue(func() bool {
				respCode, respBody, err := helper.GetObject(nnURL, nn)
				MustRespCodeIs(http.StatusOK, "get nodenetwork", err, respCode, respBody)
				return validateStatus(nn, corev1.ConditionFalse)
			})
		})
	})

	Context("disable VLAN network", func() {
		var vlanCn netv1alpha.ClusterNetwork
		var vlanCnURL string

		BeforeEach(func() {
			vlanCnURL = fmt.Sprintf("%s/%s/%s", cnAPI, namespace, "vlan")
			respCode, respBody, err := helper.GetObject(vlanCnURL, &vlanCn)
			MustRespCodeIs(http.StatusOK, "get vlan clusternetwork", err, respCode, respBody)
		})

		It("disable VLAN network", func() {
			By("disable VLAN network")
			vlanCn.Enable = false
			respCode, respBody, err := helper.PutObject(vlanCnURL, &vlanCn)
			MustRespCodeIs(http.StatusOK, "disable VLAN network", err, respCode, respBody)

			By("all nodenetwork CR should be removed")
			MustFinallyBeTrue(func() bool {
				nnsCollection, respCode, respBody, err := helper.GetCollection(nnAPI)
				if !CheckRespCodeIs(http.StatusOK, "list nodenetworks", err, respCode, respBody) {
					return false
				}
				return len(nnsCollection.Data) == 0
			})
		})
	})
})

type nodeNetworkCollection struct {
	types.Collection
	NodeNetworks []*netv1alpha.NodeNetwork `json:"data"`
}

func listNodeNetworks(url string, collection *nodeNetworkCollection) (respCode int, respBody []byte, err error) {
	err = helper.NewHTTPClient().
		GET(url).
		BindBody(&respBody).
		Code(&respCode).
		Do()
	if err != nil {
		return
	}
	if respCode != http.StatusOK {
		return
	}

	if err = json.Unmarshal(respBody, collection); err != nil {
		return
	}

	return
}

func validateStatus(nn *netv1alpha.NodeNetwork, expected corev1.ConditionStatus) bool {
	for _, condition := range nn.Status.Conditions {
		if condition.Type == netv1alpha.NodeNetworkReady {
			return condition.Status == expected
		}
	}

	return true
}
