package supportbundle

import (
	"time"
)

// determineDurationWithDefaults determines a duration based on priority:
// 1. If spec value has a value (> 0), use it directly
// 2. If it's 0, use settings value
// 3. If settings is 0, use default value
func DetermineDurationWithDefaults(specValue int, settingValue int, defaultValue int) time.Duration {
	// Priority 1: If spec value has a value (> 0), use it directly
	if specValue > 0 {
		return time.Duration(specValue) * time.Minute
	}

	// Priority 2: If spec value is 0, use settings value
	if settingValue > 0 {
		return time.Duration(settingValue) * time.Minute
	}

	// Priority 3: If settings is also 0, use default value
	return time.Duration(defaultValue) * time.Minute
}
