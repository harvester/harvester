package util

import (
	"strings"
)

type NTPSettings struct {
	NTPServers []string `json:"ntpServers,omitempty"`
}

func ReGenerateNTPServers(ntpSettings *NTPSettings) string {
	if ntpSettings == nil {
		return ""
	}
	return strings.Join(ntpSettings.NTPServers, " ")
}
