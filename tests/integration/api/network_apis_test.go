package api_test

import (
	"fmt"
	"net/http"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo"
	"github.com/tidwall/gjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rancher/harvester/pkg/config"
	ctlkubevirtv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/fuzz"
	"github.com/rancher/harvester/tests/framework/helper"
)

const (
	testNetworkNamespace      = "default"
	testNetworkTypeVlan       = "L2VlanNetwork"
	testBridgeVID             = 100
	testNetworkBridgeTemplate = `
{"cniVersion":"0.3.1","name":"%s","type":"bridge","bridge":"harvester-br0","promiscMode":true,"vlan":%d,"ipam":{}}
`
)

type BridgeNetwork struct {
	NAD *cniv1.NetworkAttachmentDefinition
}

func NewBridgeNetwork(name string, vid int) *BridgeNetwork {
	bridgeNetwork := BridgeNetwork{
		NAD: NewNAD(name, testNetworkTypeVlan, NewBridgeNetworkConfig(name, vid)),
	}
	return &bridgeNetwork
}

func NewBridgeNetworkConfig(name string, vid int) string {
	return fmt.Sprintf(testNetworkBridgeTemplate, name, vid)
}

func NewNAD(name, networkType, config string) *cniv1.NetworkAttachmentDefinition {
	networkLabels := map[string]string{
		"test.harvester.cattle.io":          "harvester-test",
		"networks.harvester.cattle.io/type": networkType,
	}
	return &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNetworkNamespace,
			Labels:    networkLabels,
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{
			Config: config,
		},
	}
}

var _ = Describe("verify network APIs", func() {

	var (
		scaled       *config.Scaled
		vmBuilder    *VMBuilder
		vmNamespace  string
		vmController ctlkubevirtv1.VirtualMachineController
		networkName  string
	)

	BeforeEach(func() {
		scaled = harvester.Scaled()
		vmBuilder = NewDefaultTestVMBuilder(testResourceLabels)
		vmNamespace = testVMNamespace
		vmController = scaled.VirtFactory.Kubevirt().V1().VirtualMachine()
	})

	Context("operate via steve API", func() {

		var networkAPI string

		BeforeEach(func() {

			networkAPI = helper.BuildAPIURL("v1", "k8s.cni.cncf.io.network-attachment-definitions", options.HTTPSListenPort)

		})

		Context("verify the create action", func() {

			It("should fail if name is missing", func() {
				networkName = ""
				network := NewBridgeNetwork(networkName, testBridgeVID).NAD
				respCode, respBody, err := helper.PostObject(networkAPI, network)
				MustRespCodeIs(http.StatusUnprocessableEntity, "create network", err, respCode, respBody)
			})

			It("should fail if vid < 1", func() {
				networkName = fuzz.String(5)
				network := NewBridgeNetwork(networkName, 0).NAD
				respCode, respBody, err := helper.PostObject(networkAPI, network)
				MustRespCodeIs(http.StatusUnprocessableEntity, "create network", err, respCode, respBody)
			})

			It("should fail if vid > 4094", func() {
				network := NewBridgeNetwork(networkName, 4095).NAD
				respCode, respBody, err := helper.PostObject(networkAPI, network)
				MustRespCodeIs(http.StatusUnprocessableEntity, "create network", err, respCode, respBody)
			})

			It("create a network with valid vid", func() {
				By("should success if vid isn't existed")
				network := NewBridgeNetwork(networkName, testBridgeVID).NAD
				respCode, respBody, err := helper.PostObject(networkAPI, network)
				MustRespCodeIs(http.StatusCreated, "create network", err, respCode, respBody)

				MustFinallyBeTrue(func() bool {
					networkURL := fmt.Sprintf("%s/%s/%s", networkAPI, testNetworkNamespace, networkName)
					respCode, respBody, err = helper.GetObject(networkURL, &network)
					return CheckRespCodeIs(http.StatusOK, "get network", err, respCode, respBody)
				})

				By("should fail if vid is existed")
				network = NewBridgeNetwork("another-"+networkName, testBridgeVID).NAD
				respCode, respBody, err = helper.PostObject(networkAPI, network)
				MustRespCodeIs(http.StatusInternalServerError, "create network", err, respCode, respBody)
			})

		})

		Specify("verify the edit action", func() {

			By("get the created network")
			var network cniv1.NetworkAttachmentDefinition
			networkURL := fmt.Sprintf("%s/%s/%s", networkAPI, testNetworkNamespace, networkName)
			respCode, respBody, err := helper.GetObject(networkURL, &network)
			MustRespCodeIs(http.StatusOK, "get network", err, respCode, respBody)

			By("edit the created network's vlan vid")
			changedVID := 10
			network.Spec.Config = NewBridgeNetworkConfig(networkName, changedVID)
			respCode, respBody, err = helper.PutObject(networkURL, network)
			MustRespCodeIs(http.StatusOK, "edit network", err, respCode, respBody)

			By("then the created network's vlan vid changed")
			respCode, respBody, err = helper.GetObject(networkURL, &network)
			MustRespCodeIs(http.StatusOK, "get the changed network", err, respCode, respBody)
			config := gjson.GetBytes(respBody, "spec.config").String()
			MustEqual(int64(changedVID), gjson.Get(config, "vlan").Int())

		})

		Specify("verify the delete action", func() {

			By("create a vm use this network")
			vm, err := vmController.Create(vmBuilder.Container().Network(networkName).VM())
			MustNotError(err)
			vmName := vm.Name
			MustVMExist(vmController, vmNamespace, vmName)

			By("should fail if delete the used network")
			networkURL := helper.BuildResourceURL(networkAPI, testNetworkNamespace, networkName)
			respCode, respBody, err := helper.DeleteObject(networkURL)
			MustRespCodeIs(http.StatusBadRequest, "delete network", err, respCode, respBody)

			By("after delete the virtual machine")
			err = vmController.Delete(vmNamespace, vmName, &metav1.DeleteOptions{})
			MustNotError(err)
			MustVMDeleted(vmController, vmNamespace, vmName)

			By("should success if delete the unused network")
			respCode, respBody, err = helper.DeleteObject(networkURL)
			MustRespCodeIn("delete network", err, respCode, respBody, http.StatusOK, http.StatusNoContent)
		})

	})

})
