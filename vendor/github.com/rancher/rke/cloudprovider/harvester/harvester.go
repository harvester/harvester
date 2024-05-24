package harvester

import v3 "github.com/rancher/rke/types"

const HarvesterCloudProviderName = "harvester"

type CloudProvider struct {
	Name   string
	Config string
}

func GetInstance() *CloudProvider {
	return &CloudProvider{}
}

func (p *CloudProvider) Init(cloudProviderConfig v3.CloudProvider) error {
	// Harvester cloud provider is an external cloud provider
	p.Name = "external"
	p.Config = cloudProviderConfig.HarvesterCloudProvider.CloudConfig
	return nil
}

func (p *CloudProvider) GetName() string {
	return p.Name
}

func (p *CloudProvider) GenerateCloudConfigFile() (string, error) {
	return p.Config, nil
}
