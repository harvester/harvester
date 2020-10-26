package network

import (
	"encoding/json"
	"errors"
	"strconv"

	cniv1 "github.com/containernetworking/cni/pkg/types"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	cni "github.com/rancher/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
)

type networkStore struct {
	types.Store
	nadCache cni.NetworkAttachmentDefinitionCache
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

	if err := s.checkUniqueVlanID(data.Namespace(), bridgeConf); err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, err.Error())
	}
	return s.Store.Create(request, schema, data)
}

func (s *networkStore) checkUniqueVlanID(ns string, config *NetConf) error {
	nads, err := s.nadCache.List(ns, labels.Everything())
	if err != nil {
		return errors.New("failed to list network attachment definitions" + err.Error())
	}
	for _, nad := range nads {
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
