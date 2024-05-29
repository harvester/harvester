package gocommon

import (
	"context"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/sirupsen/logrus"
)

func RestartService(unit string) error {
	ctx := context.Background()
	conn, err := dbus.NewWithContext(ctx)
	if err != nil {
		logrus.Errorf("Failed to create new connection for systemd. err: %v", err)
		return err
	}
	responseChan := make(chan string, 1)
	if _, err := conn.RestartUnitContext(ctx, unit, "fail", responseChan); err != nil {
		logrus.Errorf("Failed to restart service %s. err: %v", unit, err)
		return err
	}
	return nil
}
