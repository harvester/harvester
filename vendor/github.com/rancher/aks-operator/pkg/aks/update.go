package aks

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerservice/mgmt/2020-11-01/containerservice"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/rancher/aks-operator/pkg/aks/services"
	aksv1 "github.com/rancher/aks-operator/pkg/apis/aks.cattle.io/v1"
	"github.com/sirupsen/logrus"
)

func UpdateClusterTags(ctx context.Context, clusterClient services.ManagedClustersClientInterface, resourceGroupName string, resourceName string, parameters containerservice.TagsObject) (containerservice.ManagedClustersUpdateTagsFuture, error) {
	return clusterClient.UpdateTags(ctx, resourceGroupName, resourceName, parameters)
}

// UpdateCluster updates an existing managed Kubernetes cluster. Before updating, it pulls any existing configuration
// and then only updates managed fields.
func UpdateCluster(ctx context.Context, cred *Credentials, clusterClient services.ManagedClustersClientInterface, workplaceClient services.WorkplacesClientInterface,
	spec *aksv1.AKSClusterConfigSpec, phase string) error {
	// Create a new managed cluster from the AKS cluster config
	desiredCluster, err := createManagedCluster(ctx, cred, workplaceClient, spec, phase)
	if err != nil {
		return err
	}

	// Pull the upstream cluster state
	actualCluster, err := clusterClient.Get(ctx, spec.ResourceGroup, spec.ClusterName)
	if err != nil {
		logrus.Errorf("Error getting upstream AKS cluster by name [%s]: %s", spec.ClusterName, err.Error())
		return err
	}

	// Upstream cluster state was successfully pulled. Merge in updates without overwriting upstream fields: we never
	// want to overwrite preconfigured values in Azure with nil values. So only update fields pulled from AKS with
	// values from the managed cluster if they are non nil.

	_, err = clusterClient.CreateOrUpdate(
		ctx,
		spec.ResourceGroup,
		spec.ClusterName,
		updateCluster(*desiredCluster, actualCluster),
	)

	return err
}

func updateCluster(desiredCluster containerservice.ManagedCluster, actualCluster containerservice.ManagedCluster) containerservice.ManagedCluster {
	if !validateUpdate(desiredCluster, actualCluster) {
		logrus.Warn("Not all cluster properties can be updated.")
	}

	if actualCluster.ManagedClusterProperties == nil {
		actualCluster.ManagedClusterProperties = &containerservice.ManagedClusterProperties{}
	}

	// Update kubernetes version
	// If a cluster is imported, we may not have the kubernetes version set on the spec.
	if desiredCluster.KubernetesVersion != nil {
		actualCluster.KubernetesVersion = desiredCluster.KubernetesVersion
	}

	// Add/update agent pool profiles
	if actualCluster.AgentPoolProfiles == nil {
		actualCluster.AgentPoolProfiles = &[]containerservice.ManagedClusterAgentPoolProfile{}
	}

	for _, ap := range *desiredCluster.AgentPoolProfiles {
		if !hasAgentPoolProfile(ap.Name, actualCluster.AgentPoolProfiles) {
			*actualCluster.AgentPoolProfiles = append(*actualCluster.AgentPoolProfiles, ap)
		}
	}

	// Add/update addon profiles (this will keep separate profiles added by AKS). This code will also add/update addon
	// profiles for http application routing and monitoring.
	if actualCluster.AddonProfiles == nil {
		actualCluster.AddonProfiles = map[string]*containerservice.ManagedClusterAddonProfile{}
	}
	for profile := range desiredCluster.AddonProfiles {
		actualCluster.AddonProfiles[profile] = desiredCluster.AddonProfiles[profile]
	}

	// Auth IP ranges
	// note: there could be authorized IP ranges set in AKS that haven't propagated yet when this update is done. Add
	// ranges from Rancher to any ones already set in AKS.
	if actualCluster.APIServerAccessProfile == nil {
		actualCluster.APIServerAccessProfile = &containerservice.ManagedClusterAPIServerAccessProfile{
			AuthorizedIPRanges: &[]string{},
		}
	}

	if desiredCluster.APIServerAccessProfile != nil && desiredCluster.APIServerAccessProfile.AuthorizedIPRanges != nil {
		for i := range *desiredCluster.APIServerAccessProfile.AuthorizedIPRanges {
			ipRange := (*desiredCluster.APIServerAccessProfile.AuthorizedIPRanges)[i]

			if !hasAuthorizedIPRange(ipRange, actualCluster.APIServerAccessProfile) {
				*actualCluster.APIServerAccessProfile.AuthorizedIPRanges = append(*actualCluster.APIServerAccessProfile.AuthorizedIPRanges, ipRange)
			}
		}
	}

	// Linux profile
	if desiredCluster.LinuxProfile != nil {
		actualCluster.LinuxProfile = desiredCluster.LinuxProfile
	}

	// Network profile
	if desiredCluster.NetworkProfile != nil {
		if actualCluster.NetworkProfile == nil {
			actualCluster.NetworkProfile = &containerservice.NetworkProfile{}
		}

		if desiredCluster.NetworkProfile.NetworkPlugin != "" {
			actualCluster.NetworkProfile.NetworkPlugin = desiredCluster.NetworkProfile.NetworkPlugin
		}
		if desiredCluster.NetworkProfile.NetworkPolicy != "" {
			actualCluster.NetworkProfile.NetworkPolicy = desiredCluster.NetworkProfile.NetworkPolicy
		}
		if desiredCluster.NetworkProfile.NetworkMode != "" { // This is never set on the managed cluster so it will always be empty
			actualCluster.NetworkProfile.NetworkMode = desiredCluster.NetworkProfile.NetworkMode
		}
		if desiredCluster.NetworkProfile.DNSServiceIP != nil {
			actualCluster.NetworkProfile.DNSServiceIP = desiredCluster.NetworkProfile.DNSServiceIP
		}
		if desiredCluster.NetworkProfile.DockerBridgeCidr != nil {
			actualCluster.NetworkProfile.DockerBridgeCidr = desiredCluster.NetworkProfile.DockerBridgeCidr
		}
		if desiredCluster.NetworkProfile.PodCidr != nil {
			actualCluster.NetworkProfile.PodCidr = desiredCluster.NetworkProfile.PodCidr
		}
		if desiredCluster.NetworkProfile.ServiceCidr != nil {
			actualCluster.NetworkProfile.ServiceCidr = desiredCluster.NetworkProfile.ServiceCidr
		}
		if desiredCluster.NetworkProfile.OutboundType != "" { // This is never set on the managed cluster so it will always be empty
			actualCluster.NetworkProfile.OutboundType = desiredCluster.NetworkProfile.OutboundType
		}
		if desiredCluster.NetworkProfile.LoadBalancerSku != "" {
			actualCluster.NetworkProfile.LoadBalancerSku = desiredCluster.NetworkProfile.LoadBalancerSku
		}
		// LoadBalancerProfile is not configurable in Rancher so there won't be subfield conflicts. Just pull it from
		// the state in AKS.
	}

	// Service principal client id and secret
	if desiredCluster.ServicePrincipalProfile != nil {
		actualCluster.ServicePrincipalProfile = desiredCluster.ServicePrincipalProfile
	}

	// Tags
	if desiredCluster.Tags != nil {
		actualCluster.Tags = desiredCluster.Tags
	}

	return actualCluster
}

func validateUpdate(desiredCluster containerservice.ManagedCluster, actualCluster containerservice.ManagedCluster) bool {
	/*
		The following fields are managed in Rancher but are NOT configurable on update
		- Name
		- Location
		- DNSPrefix
		- EnablePrivateCluster
		- LoadBalancerProfile
	*/

	if desiredCluster.Name != nil && actualCluster.Name != nil && to.String(desiredCluster.Name) != to.String(actualCluster.Name) {
		logrus.Warnf("Cluster name update from [%s] to [%s] is not supported", *actualCluster.Name, *desiredCluster.Name)
		return false
	}

	if desiredCluster.Location != nil && actualCluster.Location != nil && to.String(desiredCluster.Location) != to.String(actualCluster.Location) {
		logrus.Warnf("Cluster location update from [%s] to [%s] is not supported", *actualCluster.Location, *desiredCluster.Location)
		return false
	}

	if desiredCluster.ManagedClusterProperties != nil && actualCluster.ManagedClusterProperties != nil &&
		desiredCluster.DNSPrefix != nil && actualCluster.DNSPrefix != nil &&
		to.String(desiredCluster.DNSPrefix) != to.String(actualCluster.DNSPrefix) {
		logrus.Warnf("Cluster DNS prefix update from [%s] to [%s] is not supported", *actualCluster.DNSPrefix, *desiredCluster.DNSPrefix)
		return false
	}

	if desiredCluster.ManagedClusterProperties != nil && actualCluster.ManagedClusterProperties != nil &&
		desiredCluster.APIServerAccessProfile != nil && actualCluster.APIServerAccessProfile != nil &&
		to.Bool(desiredCluster.APIServerAccessProfile.EnablePrivateCluster) != to.Bool(actualCluster.APIServerAccessProfile.EnablePrivateCluster) {
		logrus.Warn("Cluster can't be updated from private")
		return false
	}

	return true
}

func hasAgentPoolProfile(name *string, agentPoolProfiles *[]containerservice.ManagedClusterAgentPoolProfile) bool {
	if agentPoolProfiles == nil {
		return false
	}

	for _, ap := range *agentPoolProfiles {
		if *ap.Name == *name {
			return true
		}
	}
	return false
}

func hasAuthorizedIPRange(name string, apiServerAccessProfile *containerservice.ManagedClusterAPIServerAccessProfile) bool {
	if apiServerAccessProfile == nil || apiServerAccessProfile.AuthorizedIPRanges == nil {
		return false
	}

	for _, ipRange := range *apiServerAccessProfile.AuthorizedIPRanges {
		if ipRange == name {
			return true
		}
	}
	return false
}
