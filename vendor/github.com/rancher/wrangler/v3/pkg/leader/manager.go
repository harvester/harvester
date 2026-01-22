package leader

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

type Manager struct {
	sync.Mutex
	leaderChan    chan struct{}
	leaderStarted bool
	leaderCTX     context.Context
	namespace     string
	name          string
	k8s           kubernetes.Interface
}

func NewManager(namespace, name string, k8s kubernetes.Interface) *Manager {
	return &Manager{
		leaderChan: make(chan struct{}),
		namespace:  namespace,
		name:       name,
		k8s:        k8s,
	}
}

func (m *Manager) Start(ctx context.Context) {
	m.Lock()
	defer m.Unlock()

	if m.leaderStarted {
		return
	}

	m.leaderStarted = true
	go RunOrDie(ctx, m.namespace, m.name, m.k8s, func(ctx context.Context) {
		m.leaderCTX = ctx
		close(m.leaderChan)
	})
}

// OnLeaderOrDie this function will be called when leadership is acquired or die if failed
func (m *Manager) OnLeaderOrDie(name string, f func(ctx context.Context) error) {
	go func() {
		<-m.leaderChan
		if err := f(m.leaderCTX); err != nil {
			logrus.Fatalf("%s leader func failed be executed: %v", name, err)
		} else {
			logrus.Infof("%s leader func executed successfully", name)
		}
	}()
}

// OnLeader this function will be called when leadership is acquired.
func (m *Manager) OnLeader(f func(ctx context.Context) error) {
	go func() {
		<-m.leaderChan
		for {
			if err := f(m.leaderCTX); err != nil {
				logrus.Errorf("failed to call leader func: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			break
		}
	}()
}
