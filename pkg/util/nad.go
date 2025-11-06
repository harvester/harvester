package util

import (
	"encoding/json"
	"fmt"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)


type NetConf struct {
	cnitypes.PluginConf
}

func DecodeNadConfigToNetConf(nad *cniv1.NetworkAttachmentDefinition) (*NetConf, error) {
	conf := &NetConf{}
	if nad == nil || nad.Spec.Config == "" {
		return conf, nil
	}

	if err := json.Unmarshal([]byte(nad.Spec.Config), conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal nad %v/%v config %s %w", nad.Namespace, nad.Name, nad.Spec.Config, err)
	}

	return conf, nil
}

func (nc *NetConf) IsBridgeCNI() bool {
	return nc.Type == "bridge"
}

func IsNadCreatedBySystem(nad *cniv1.NetworkAttachmentDefinition) bool {
	return nad.Namespace == HarvesterSystemNamespaceName
}
