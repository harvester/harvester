package version

import "errors"

var (
	ErrInvalidVersion      = errors.New("invalid version string")
	ErrIncomparableVersion = errors.New("incomparable: dev version")

	ErrInvalidHarvesterVersion       = errors.New("invalid harvester version")
	ErrMinUpgradeRequirement         = errors.New("current version does not meet minimum upgrade requirement")
	ErrDowngrade                     = errors.New("downgrading is prohibited")
	ErrDevUpgrade                    = errors.New("upgrading from dev versions to non-dev versions is prohibited")
	ErrPrereleaseCrossVersionUpgrade = errors.New("cross-version upgrades from/to any prerelease version are prohibited")
)
