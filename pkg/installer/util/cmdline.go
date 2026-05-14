package util

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/rancher/mapper/values"
)

// ParseCmdLine parses kernel parameters.
func ParseCmdLine(cmdline string, prefix string) (map[string]interface{}, error) {
	//supporting regex https://regexr.com/4mq0s
	parser, err := regexp.Compile(`(\"[^\"]+\")|([^\s]+=(\"[^\"]+\")|([^\s]+))`)
	if err != nil {
		return nil, nil
	}

	data := map[string]interface{}{}
	for _, item := range parser.FindAllString(cmdline, -1) {
		parts := strings.SplitN(item, "=", 2)
		value := "true"
		if len(parts) > 1 {
			value = strings.Trim(parts[1], `"`)
		}
		keys := strings.Split(strings.Trim(parts[0], `"`), ".")
		if prefix != "" {
			if keys[0] != prefix {
				continue
			}
			keys = keys[1:]
		}
		existing, ok := values.GetValue(data, keys...)
		if ok {
			switch v := existing.(type) {
			case string:
				values.PutValue(data, []string{v, value}, keys...)
			case []string:
				values.PutValue(data, append(v, value), keys...)
			}
		} else {
			values.PutValue(data, value, keys...)
		}
	}

	err = toNetworkInterfaces(data)
	if err != nil {
		return data, err
	}
	err = toSchemeVersion(data)
	return data, err
}

// ReadCmdline parses /proc/cmdline and returns a map contains kernel parameters
func ReadCmdline(prefix string) (map[string]interface{}, error) {
	bytes, err := os.ReadFile("/proc/cmdline")
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return ParseCmdLine(string(bytes), prefix)
}

// parse kernel arguments and process network interfaces as a struct
func toNetworkInterfaces(data map[string]interface{}) error {
	networkInterfaces, ok := values.GetValue(data, "install", "management_interface", "interfaces")
	if !ok {
		return nil
	}
	ifDetails := make([]string, 0)

	switch networkInterfaces := networkInterfaces.(type) {
	case string:
		ifDetails = append(ifDetails, networkInterfaces)
	case []string:
		ifDetails = networkInterfaces
	}

	outDetails := make([]interface{}, 0, len(ifDetails))
	for _, v := range ifDetails {
		n, err := parseIfDetails(v)
		if err != nil {
			return err
		}
		outDetails = append(outDetails, n)
	}

	values.PutValue(data, outDetails, "install", "management_interface", "interfaces")
	return nil
}

// parseIfDetails accepts strings in the form of:
// - "hwAddr: be:44:8c:b0:5d:f2"
// - "name: ens3"
// - "be:44:8c:b0:5d:f2"
// - "ens3"
// - "hwAddr:be:44:8c:b0:5d:f2,name:ens3"
// - "hwAddr:be:44:8c:b0:5d:f2,ens3"
// - "be:44:8c:b0:5d:f2,name:ens3"
// and returns a map with the parsed fields `hwAddr` and `name`.
func parseIfDetails(details string) (map[string]interface{}, error) {
	var parts []string
	data := map[string]any{}

	if details == "" {
		return nil, fmt.Errorf("empty interface details")
	}

	if strings.Contains(details, ",") {
		parts = strings.Split(details, ",")
	} else {
		parts = []string{details}
	}

	for _, field := range parts {
		subParts := make([]string, 0, 7)

		for _, s := range strings.Split(field, ":") {
			subParts = append(subParts, strings.TrimSpace(s))
		}

		switch len(subParts) {
		case 7:
			// hwAddr: be:44:8c:b0:5d:f2
			if subParts[0] != "hwAddr" {
				return nil, fmt.Errorf("could not parse interface details %v", details)
			}
			hwAddr := strings.Join(subParts[1:], ":")
			if _, err := IsMACAddress(hwAddr); err != nil {
				return nil, fmt.Errorf("could not parse interface details: %w", err)
			}
			data["hwAddr"] = hwAddr
		case 6:
			// be:44:8c:b0:5d:f2
			hwAddr := strings.Join(subParts, ":")
			if _, err := IsMACAddress(hwAddr); err != nil {
				return nil, fmt.Errorf("could not parse interface details: %w", err)
			}
			data["hwAddr"] = hwAddr
		case 2:
			// name: ens3
			if subParts[0] != "name" {
				return nil, fmt.Errorf("could not parse interface details %v", details)
			}
			data["name"] = subParts[1]
		case 1:
			// ens3
			data["name"] = subParts[0]
		default:
			return nil, fmt.Errorf("could not parse interface details %v", details)
		}
	}

	return data, nil
}

func toSchemeVersion(data map[string]interface{}) error {
	schemeVersion, ok := values.GetValue(data, "scheme_version")
	if !ok {
		return nil
	}

	schemeVersionUint, err := strconv.ParseUint(schemeVersion.(string), 10, 32)
	if err != nil {
		return err
	}
	values.PutValue(data, schemeVersionUint, "scheme_version")
	return nil
}
