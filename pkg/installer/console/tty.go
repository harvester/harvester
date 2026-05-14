package console

import (
	"os"
	goruntime "runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

func getFirstConsoleTTY() string {
	b, err := os.ReadFile("/sys/class/tty/console/active")
	if err != nil {
		logrus.Error(err)
		return ""
	}

	ttys := strings.Split(strings.TrimRight(string(b), "\n"), " ")
	if len(ttys) > 0 {
		// arm devices generally have first console as /dev/ttyAMA0
		// this console is skipped in iso based installs due to display resolution issues
		// as a result of this automatic install via ipxe fails since installer runs in
		// say /dev/tty1 but first console returned by this method is ttyAMA0
		// we are currently adding a check to skip AMA0 if it is the first console and return
		// the second item in the list
		if goruntime.GOARCH == "arm64" && strings.Contains(ttys[0], "AMA0") && len(ttys) > 1 {
			return ttys[1]
		}
		return ttys[0]
	}
	return ""
}

func isFirstConsoleTTY() bool {
	tty := os.Getenv("TTY")
	logrus.Infof("my tty is %s", tty)
	if tty == "" {
		return false
	}
	return strings.TrimPrefix(tty, "/dev/") == getFirstConsoleTTY()
}
