package versionguard

import (
	"errors"

	"github.com/harvester/go-common/version"
	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func Check(upgrade *v1beta1.Upgrade, strictMode bool, minUpgradableVersionStr string) error {

	currentVersion, err := version.NewHarvesterVersion(upgrade.Status.PreviousVersion)
	if err != nil {
		return err
	}
	upgradeVersion, err := version.NewHarvesterVersion(upgrade.Spec.Version)
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
		repoInfo, err := getRepoInfo(upgrade)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"namespace": upgrade.Namespace,
				"name":      upgrade.Name,
			}).Warn("failed to retrieve repo info")
			return err
		}

		minUpgradableVersion, err = version.NewHarvesterVersion(repoInfo.Release.MinUpgradableVersion)
		if err != nil && !errors.Is(version.ErrInvalidVersion, err) {
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
