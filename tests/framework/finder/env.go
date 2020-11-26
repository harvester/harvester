package finder

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/iancoleman/strcase"
)

type EnvFinder struct {
	prefix string
}

func (f *EnvFinder) Get(camelEnvName string, defaultVal string) string {
	envName := strcase.ToScreamingSnake(camelEnvName)
	if f.prefix != "" {
		envName = f.prefix + "_" + envName
	}
	envName = strings.ToUpper(envName)
	if val, ok := os.LookupEnv(envName); ok {
		return val
	}
	_ = os.Setenv(envName, defaultVal)
	return defaultVal
}

func (f *EnvFinder) GetInt(camelEnvName string, defaultVal int) int {
	val := f.Get(camelEnvName, fmt.Sprint(defaultVal))
	ret, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return ret
}

func (f *EnvFinder) GetDuration(camelEnvName string, defaultVal time.Duration) time.Duration {
	val := f.Get(camelEnvName, defaultVal.String())
	ret, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return ret
}

func NewEnvFinder(prefix string) *EnvFinder {
	return &EnvFinder{
		prefix: strings.ToUpper(prefix),
	}
}
