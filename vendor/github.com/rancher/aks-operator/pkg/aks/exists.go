package aks

import (
	"context"

	"github.com/rancher/aks-operator/pkg/aks/services"
	aksv1 "github.com/rancher/aks-operator/pkg/apis/aks.cattle.io/v1"
)

func ExistsResourceGroup(ctx context.Context, groupsClient services.ResourceGroupsClientInterface, resourceGroup string) (bool, error) {
	resp, err := groupsClient.CheckExistence(ctx, resourceGroup)

	// client should return 204 (no content) and if not, return false and the associated error.
	return resp.StatusCode == 204, err
}

// ExistsCluster Check if AKS managed Kubernetes cluster exist
func ExistsCluster(ctx context.Context, clusterClient services.ManagedClustersClientInterface, spec *aksv1.AKSClusterConfigSpec) (bool, error) {
	resp, err := clusterClient.Get(ctx, spec.ResourceGroup, spec.ClusterName)

	// client should return 200 OK and if not, return false and the associated error. If the error is non nil and
	// permissions related, we will want that bubbled up to the ui so the user knows to adjust their resource permissions
	// in AKS.
	return resp.StatusCode == 200, err
}
