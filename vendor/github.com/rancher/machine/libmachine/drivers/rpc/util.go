package rpcdriver

import (
	"os"
	"strconv"
	"strings"

	"github.com/rancher/machine/libmachine/mcnflag"
)

// GetDriverOpts converts driver flags into RPCFlags.
func GetDriverOpts(flags []mcnflag.Flag, args []string) *RPCFlags {
	allFlags := getAllFlags(args)
	foundFlags := make(map[string]any)

	for _, f := range flags {
		switch f.(type) {
		case *mcnflag.BoolFlag:
			flag := f.(*mcnflag.BoolFlag)
			setFlag(flag.Name, flag.EnvVar, nil, allFlags, foundFlags, toBool)

		case mcnflag.BoolFlag:
			flag := f.(mcnflag.BoolFlag)
			setFlag(flag.Name, flag.EnvVar, nil, allFlags, foundFlags, toBool)

		case *mcnflag.StringFlag:
			flag := f.(*mcnflag.StringFlag)
			var defaultValue any
			if flag.Value != "" {
				defaultValue = flag.Value
			}
			setFlag(flag.Name, flag.EnvVar, defaultValue, allFlags, foundFlags, toString)

		case mcnflag.StringFlag:
			flag := f.(mcnflag.StringFlag)
			var defaultValue any
			if flag.Value != "" {
				defaultValue = flag.Value
			}
			setFlag(flag.Name, flag.EnvVar, defaultValue, allFlags, foundFlags, toString)

		case *mcnflag.IntFlag:
			flag := f.(*mcnflag.IntFlag)
			var defaultValue any
			if flag.Value != 0 {
				defaultValue = flag.Value
			}
			setFlag(flag.Name, flag.EnvVar, defaultValue, allFlags, foundFlags, toInt)

		case mcnflag.IntFlag:
			flag := f.(mcnflag.IntFlag)
			var defaultValue any
			if flag.Value != 0 {
				defaultValue = flag.Value
			}
			setFlag(flag.Name, flag.EnvVar, defaultValue, allFlags, foundFlags, toInt)

		case *mcnflag.StringSliceFlag:
			flag := f.(*mcnflag.StringSliceFlag)
			setFlag(flag.Name, flag.EnvVar, flag.Value, allFlags, foundFlags, toStringSlice)

		case mcnflag.StringSliceFlag:
			flag := f.(mcnflag.StringSliceFlag)
			setFlag(flag.Name, flag.EnvVar, flag.Value, allFlags, foundFlags, toStringSlice)
		}
	}

	return &RPCFlags{Values: foundFlags}
}

// getAllFlags retrieves all flags present in args. These flags are identified by their prefix, which can be "-" or
// "--".
func getAllFlags(args []string) map[string]any {
	flagValues := make(map[string]any)
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}

		trimmedFlag := strings.TrimLeft(arg, "-")
		flagParts := strings.Split(trimmedFlag, "=")
		if len(flagParts) > 1 {
			flagValues[flagParts[0]] = flagParts[1]
		} else if len(args) > i+1 && !strings.HasPrefix(args[i+1], "-") {
			flagValues[trimmedFlag] = args[i+1]
		} else {
			flagValues[trimmedFlag] = true
		}
	}

	return flagValues
}

func toBool(v any) any {
	if boolV, ok := v.(bool); ok {
		return boolV
	}

	return nil
}

func toString(v any) any {
	if stringV, ok := v.(string); ok {
		return stringV
	}

	return nil
}

func toInt(v any) any {
	// If v is already an int, return it.
	if intV, ok := v.(int); ok {
		return intV
	}

	// If v is a string, try converting it into an int.
	if stringV := toString(v); stringV != nil {
		if intV, err := strconv.Atoi(stringV.(string)); err == nil {
			return intV
		}
	}

	return nil
}

func toStringSlice(v any) any {
	// If v is a string, slice it by comma.
	if stringV, ok := v.(string); ok {
		return strings.Split(stringV, ",")
	}

	return nil
}

func setFlag(
	name, envvar string,
	defaultValue any,
	allFlags map[string]any,
	foundFlags map[string]any,
	convertFunc func(any) any,
) {
	if v, ok := allFlags[name]; ok {
		if result := convertFunc(v); result != nil {
			foundFlags[name] = result
		}
	} else if envvar != "" {
		if v, ok := os.LookupEnv(envvar); ok {
			if result := convertFunc(v); result != nil {
				foundFlags[name] = result
			}
		}
	} else if defaultValue != nil {
		foundFlags[name] = defaultValue
	}
}
