package monitor

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-manager/datastore"
)

type Monitor interface {
	Start()
	Close()

	SyncCollectedData() error
	GetCollectedData() (interface{}, error)
}

type baseMonitor struct {
	logger logrus.FieldLogger

	ds *datastore.DataStore

	syncPeriod time.Duration

	ctx  context.Context
	quit context.CancelFunc
}

func newBaseMonitor(ctx context.Context, quit context.CancelFunc, logger logrus.FieldLogger, ds *datastore.DataStore, syncPeriod time.Duration) *baseMonitor {
	m := &baseMonitor{
		logger: logger,

		ds: ds,

		syncPeriod: syncPeriod,

		ctx:  ctx,
		quit: quit,
	}

	return m
}
