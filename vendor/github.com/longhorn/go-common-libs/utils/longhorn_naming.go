package utils

import (
	"regexp"
)

// IsEngineProcess distinguish if the process is a engine process by its name.
func IsEngineProcess(processName string) bool {
	// engine process name example: pvc-5a8ee916-5989-46c6-bafc-ddbf7c802499-e-0
	return regexp.MustCompile(`.+?-e-[^-]*\d$`).MatchString(processName)
}
