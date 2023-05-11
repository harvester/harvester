package util

import "strings"

type NTPSettings struct {
	NTPServers []string `json:"ntpServers,omitempty"`
}

func GetNTPServers(ntpSettings *NTPSettings) string {
	ntpServers := ntpSettings.NTPServers

	return strings.Join(ntpServers, " ")
}
