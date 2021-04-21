package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	cniv1 "github.com/containernetworking/cni/pkg/types"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	cni "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/indexeres"
)

type networkStore struct {
	types.Store
	nadCache cni.NetworkAttachmentDefinitionCache
	vmCache  ctlkubevirtv1.VirtualMachineCache
}

type NetConf struct {
	cniv1.NetConf
	BrName       string `json:"bridge"`
	IsGW         bool   `json:"isGateway"`
	IsDefaultGW  bool   `json:"isDefaultGateway"`
	ForceAddress bool   `json:"forceAddress"`
	IPMasq       bool   `json:"ipMasq"`
	MTU          int    `json:"mtu"`
	HairpinMode  bool   `json:"hairpinMode"`
	PromiscMode  bool   `json:"promiscMode"`
	Vlan         int    `json:"vlan"`
}

const (
	FieldSpec   = "spec"
	FieldConfig = "config"
)

func (s *networkStore) Create(request *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	objData := data.Data()
	config := objData.Map(FieldSpec).String(FieldConfig)
	if config == "" {
		return types.APIObject{}, apierror.NewAPIError(validation.InvalidBodyContent, "NAD config %s is empty")
	}

	var bridgeConf = &NetConf{}
	err := json.Unmarshal([]byte(config), &bridgeConf)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, "Failed to decode NAD config value, error: "+err.Error())
	}

	if bridgeConf.Vlan < 1 || bridgeConf.Vlan > 4094 {
		return types.APIObject{}, apierror.NewAPIError(validation.InvalidBodyContent, "Bridge vlan vid must >=1 and <=4094")
	}

	if err := s.checkUniqueVlanID(data.Namespace(), bridgeConf); err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, err.Error())
	}
	return s.Store.Create(request, schema, data)
}

func (s *networkStore) checkUniqueVlanID(ns string, config *NetConf) error {
	nadList, err := s.nadCache.List(ns, labels.Everything())
	if err != nil {
		return errors.New("failed to list network attachment definitions" + err.Error())
	}
	for _, nad := range nadList {
		var bridgeConf = &NetConf{}
		err := json.Unmarshal([]byte(nad.Spec.Config), &bridgeConf)
		if err != nil {
			logrus.Errorf("failed to decode network attachment definition, %s", err.Error())
			continue
		}
		if bridgeConf.Vlan == config.Vlan {
			return errors.New("invalid vlan id " + strconv.Itoa(bridgeConf.Vlan) + ", it is already exist")
		}
	}
	return nil
}

func (s *networkStore) Delete(request *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	networkName := request.Name
	vms, err := s.vmCache.GetByIndex(indexeres.VMByNetworkIndex, networkName)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, err.Error())
	}

	if len(vms) > 0 {
		vmNameList := make([]string, 0, len(vms))
		for _, vm := range vms {
			vmNameList = append(vmNameList, vm.Name)
		}
		errorMessage := fmt.Sprintf("network %s is still used by vm：%s", networkName, strings.Join(vmNameList, ","))
		errorCode := validation.ErrorCode{
			Code:   "ResourceIsUsed",
			Status: http.StatusBadRequest,
		}
		return types.APIObject{}, apierror.NewAPIError(errorCode, errorMessage)
	}

	return s.Store.Delete(request, schema, id)
}
