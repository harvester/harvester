package versionguard

import (
	"errors"

	"github.com/harvester/go-common/version"
	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func Check(upgrade *v1beta1.Upgrade, strictMode bool, minUpgradableVersionStr string) error {

	repoInfo, err := getRepoInfo(upgrade)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace": upgrade.Namespace,
			"name":      upgrade.Name,
		}).Error("failed to retrieve repo info")
		return err
	}

	upgradeVersion, err := version.NewHarvesterVersion(repoInfo.Release.Harvester)
	if err != nil {
		return err
	}

	currentVersion, err := version.NewHarvesterVersion(upgrade.Status.PreviousVersion)
	if err != nil {
		return err
	}

	var minUpgradableVersion *version.HarvesterVersion
	if minUpgradableVersionStr != "" {
		minUpgradableVersion, err = version.NewHarvesterVersion(minUpgradableVersionStr)
		if err != nil {
			return err
		}
	} else {
		minUpgradableVersion, err = version.NewHarvesterVersion(repoInfo.Release.MinUpgradableVersion)
		// When the error is ErrInvalidVersion, let the nil minUpgradableVersion slip through the check since it's a
		// valid scenario. It implies "upgrade with no restrictions."
		if err != nil && !errors.Is(err, version.ErrInvalidVersion) {
			return err
		}
	}

	logrus.WithFields(logrus.Fields{
		"namespace":            upgrade.Namespace,
		"name":                 upgrade.Name,
		"currentVersion":       currentVersion,
		"upgradeVersion":       upgradeVersion,
		"minUpgradableVersion": minUpgradableVersion,
	}).Info("upgrade eligibility check")

	harvesterUpgradeVersion := version.NewHarvesterUpgradeVersion(currentVersion, upgradeVersion, minUpgradableVersion)

	return harvesterUpgradeVersion.CheckUpgradeEligibility(strictMode)
}
