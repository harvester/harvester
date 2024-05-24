package aks

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerservice/mgmt/2020-11-01/containerservice"
	"github.com/rancher/aks-operator/pkg/aks/services"
)

func GetClusterAccessProfile(ctx context.Context, clusterClient services.ManagedClustersClientInterface, resourceGroupName string, resourceName string, roleName string) (containerservice.ManagedClusterAccessProfile, error) {
	return clusterClient.GetAccessProfile(ctx, resourceGroupName, resourceName, roleName)
}
