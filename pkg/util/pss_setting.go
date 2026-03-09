package util

import (
	"slices"
	"strings"

	"github.com/harvester/harvester/pkg/settings"
)

// GetWhitelistedNamespacesList returns the list of whitelisted namespaces from the pss setting, it will split the comma separated string into a slice of strings and remove unique values
func GetWhitelistedNamespacesList(pssSetting *settings.PodSecuritySetting) []string {
	// split comma separated whitelist
	namespaces := strings.Split(pssSetting.WhitelistedNamespacesList, ",")
	namespaces = append(namespaces, DefaultHarvesterNamespaceWhiteList...)
	slices.Sort(namespaces)
	return slices.Compact(namespaces)
}

// GetRestrictedNamespacesList returns the list of privileged namespaces from the pss setting, it will split the comma separated string into a slice of strings and remove unique values
func GetRestrictedNamespacesList(pssSetting *settings.PodSecuritySetting) []string {
	// split comma separated whitelist
	namespaces := strings.Split(pssSetting.RestrictedNamespacesList, ",")
	slices.Sort(namespaces)
	return slices.Compact(namespaces)
}

// GetPrivilegedNamespacesList returns the list of privileged namespaces from the pss setting, it will split the comma separated string into a slice of strings and remove unique values
func GetPrivilegedNamespacesList(pssSetting *settings.PodSecuritySetting) []string {
	// split comma separated whitelist
	namespaces := strings.Split(pssSetting.PrivilegedNamespacesList, ",")
	slices.Sort(namespaces)
	return slices.Compact(namespaces)
}
