package utils

import (
	"github.com/sirupsen/logrus"
)

func SetLogLevel(level string) {
	ll, err := logrus.ParseLevel(level)
	if err != nil {
		logrus.Infof("Failed to parse the log level %s, error %s, fallback to info level", level, err.Error())
		ll = logrus.InfoLevel
	} else {
		logrus.Infof("Set log level to %s", level)
	}
	// set global log level
	logrus.SetLevel(ll)
}
