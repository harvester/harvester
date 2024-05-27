package utils

import (
	"fmt"

	aksv1 "github.com/rancher/aks-operator/pkg/apis/aks.cattle.io/v1"
)

func BuildNodePoolMap(nodePools []aksv1.AKSNodePool, clusterName string) (map[string]*aksv1.AKSNodePool, error) {
	ret := make(map[string]*aksv1.AKSNodePool, len(nodePools))
	for i := range nodePools {
		if nodePools[i].Name != nil {
			if _, ok := ret[*nodePools[i].Name]; ok {
				return nil, fmt.Errorf("cluster [%s] cannot have multiple nodepools with name %s", clusterName, *nodePools[i].Name)
			}
			ret[*nodePools[i].Name] = &nodePools[i]
		}
	}
	return ret, nil
}
