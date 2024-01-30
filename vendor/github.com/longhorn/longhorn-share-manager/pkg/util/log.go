package util

import (
	"os"

	"github.com/sirupsen/logrus"
)

func NewLogger() logrus.FieldLogger {

	// the debug level is enabled based on a global cli var
	// and it will be set on the package logger
	logger := logrus.New()
	logger.SetLevel(logrus.GetLevel())
	logger.SetOutput(os.Stdout)
	return logger
}
