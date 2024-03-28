package util

import (
	"slices"
	"strings"
)

type NTPSettings struct {
	NTPServers []string `json:"ntpServers,omitempty"`
}

func ReGenerateNTPServers(ntpSettings *NTPSettings, curNTPServers []string) string {
	parsedNTPServers := make([]string, 0)
	if len(curNTPServers) == 0 {
		curNTPServers = parsedNTPServers
	}

	for _, ntpServer := range ntpSettings.NTPServers {
		if !slices.Contains(curNTPServers, ntpServer) {
			curNTPServers = append(curNTPServers, ntpServer)
		}
	}
	return strings.Join(curNTPServers, " ")
}
