package utils

import "github.com/achanda/go-sysctl"

func EnsureSysctlValue(name, value string) error {
	v, err := sysctl.Get(name)
	if err != nil {
		return err
	}
	if v == value {
		return nil
	}

	return sysctl.Set(name, value)
}
