package types

import "github.com/pkg/errors"

var (
	// It is recommended to wrap the following common errors with meaningful message, so that the error handler can unwrap
	// the underlying error and compare directly using errors.Is. For example:
	//
	//	func GetSysConfig(filePath string) (string, error) {
	//		configVal, err := readConfig(filePath)
	//		if os.IsNotExist(err) {
	//			return fmt.Errorf("config file %q not present: %w", filePath, ErrNotConfigured)
	//		}
	//		...
	//	}
	//
	//	configVal, err := GetSysConfig(filePath)
	//	if errors.Is(err, types.ErrNotConfigured) {
	//		configVal = defaultVal
	//	}

	ErrNotConfigured = errors.New("is not configured")
)
