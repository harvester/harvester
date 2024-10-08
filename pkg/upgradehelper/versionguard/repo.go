package versionguard

import (
	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade/repoinfo"
)

func getRepoInfo(upgrade *v1beta1.Upgrade) (*repoinfo.RepoInfo, error) {
	repoInfo := &repoinfo.RepoInfo{}
	if err := repoInfo.Load(upgrade.Status.RepoInfo); err != nil {
		return nil, err
	}
	return repoInfo, nil
}
