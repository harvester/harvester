package versionguard

import (
	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvupgrade "github.com/harvester/harvester/pkg/controller/master/upgrade"
)

func getRepoInfo(upgrade *v1beta1.Upgrade) (*harvupgrade.RepoInfo, error) {
	repoInfo := &harvupgrade.RepoInfo{}
	if err := repoInfo.Load(upgrade.Status.RepoInfo); err != nil {
		return nil, err
	}
	return repoInfo, nil
}
