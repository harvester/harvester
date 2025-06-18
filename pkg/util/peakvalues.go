package util

import "time"

const (

	// the allowed min and max HotplugRatio value
	MaxHotplugRatioValue = 20
	MinHotplugRatioValue = 1

	// default and min-max allowed values for UpgradeConfig.LogReadyTimeout
	DefaultUpgradeLogReadyTimeout = 5 * time.Minute
	MinUpgradeLogReadyTimeout     = 1 * time.Minute
	MaxUpgradeLogReadyTimeout     = 20 * time.Minute
)
