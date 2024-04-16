package repoinfo

import "gopkg.in/yaml.v2"

type HarvesterRelease struct {
	Harvester            string `yaml:"harvester,omitempty"`
	HarvesterChart       string `yaml:"harvesterChart,omitempty"`
	OS                   string `yaml:"os,omitempty"`
	Kubernetes           string `yaml:"kubernetes,omitempty"`
	Rancher              string `yaml:"rancher,omitempty"`
	MonitoringChart      string `yaml:"monitoringChart,omitempty"`
	MinUpgradableVersion string `yaml:"minUpgradableVersion,omitempty"`
}

type RepoInfo struct {
	Release HarvesterRelease
}

func (info *RepoInfo) Marshall() (string, error) {
	out, err := yaml.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (info *RepoInfo) Load(data string) error {
	return yaml.Unmarshal([]byte(data), info)
}
