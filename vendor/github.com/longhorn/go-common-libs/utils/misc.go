package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/longhorn/go-common-libs/types"
)

// Contains checks if the given slice contains the given value.
func Contains[T comparable](slice []T, value T) bool {
	for _, s := range slice {
		if s == value {
			return true
		}
	}
	return false
}

// GetFunctionName returns the <package>.<function name> of the given function.
func GetFunctionName(f interface{}) string {
	value := reflect.ValueOf(f)
	if value.Kind() != reflect.Func {
		return ""
	}
	return filepath.Base(runtime.FuncForPC(value.Pointer()).Name())
}

// GetFunctionPath returns the full path of the given function.
func GetFunctionPath(f interface{}) string {
	getFn := func() string {
		return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	}
	return GetFunctionInfo(f, getFn)
}

// GetFunctionInfo take a function as interface{} and a function for getting the info.
func GetFunctionInfo(f interface{}, getInfoFn func() string) string {
	value := reflect.ValueOf(f)
	if value.Kind() != reflect.Func {
		return ""
	}
	return getInfoFn()
}

// IsStringInSlice checks if the given string 'item' is present in the 'list' of strings.
func IsStringInSlice(list []string, item string) bool {
	for _, str := range list {
		if str == item {
			return true
		}
	}
	return false
}

// RandomID returns a random string with the specified length.
// If the specified length is less than or equal to 0, the default length will
// be used.
func RandomID(randomIDLenth int) string {
	if randomIDLenth <= 0 {
		randomIDLenth = types.RandomIDDefaultLength
	}

	uuid := strings.Replace(UUID(), "-", "", -1)

	if len(uuid) > randomIDLenth {
		uuid = uuid[:randomIDLenth]
	}
	return uuid
}

// UUID returns a random UUID string.
func UUID() string {
	return uuid.New().String()
}

// GenerateRandomNumber generates a random positive number between lower and upper.
// The return value should be between [lower, upper), and error is nil when success.
// If the error is not nil, the return value is 0.
func GenerateRandomNumber(lower, upper int64) (int64, error) {
	if lower > upper {
		return 0, errors.Errorf("invalid boundary: [%v, %v)", lower, upper)
	}

	if lower == upper {
		return lower, nil
	}

	randNum, err := rand.Int(rand.Reader, big.NewInt(upper-lower))
	if err != nil {
		return 0, err
	}
	return (lower + randNum.Int64()), nil
}

// ConvertTypeToString converts the given value to string.
func ConvertTypeToString[T any](value T) string {
	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	default:
		return fmt.Sprintf("Unsupported type: %v", v.Kind())
	}
}
