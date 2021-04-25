package cache

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/lasso/pkg/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	defaultTimeout = 15 * time.Second
)

type healthcheck struct {
	lock     sync.Mutex
	nsClient *client.Client
	callback func(bool)
}

func (h *healthcheck) ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return h.nsClient.Get(ctx, "", "kube-system", &v1.Namespace{}, metav1.GetOptions{})
}

func (h *healthcheck) start(ctx context.Context, cf client.SharedClientFactory) error {
	first, err := h.initialize(cf)
	if err != nil {
		return err
	}
	if first {
		h.ensureHealthy(ctx)
	}
	return nil
}

func (h *healthcheck) initialize(cf client.SharedClientFactory) (bool, error) {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.nsClient != nil {
		return false, nil
	}

	nsClient, err := cf.ForResource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}, false)
	if err != nil {
		return false, err
	}
	h.nsClient = nsClient

	return true, nil
}

func (h *healthcheck) ensureHealthy(ctx context.Context) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.pingUntilGood(ctx)
}

func (h *healthcheck) report(good bool) {
	if h.callback != nil {
		h.callback(good)
	}
}

func (h *healthcheck) pingUntilGood(ctx context.Context) {
	for {
		if err := h.ping(ctx); err == nil {
			h.report(true)
			return
		}

		h.report(false)
		time.Sleep(defaultTimeout)
	}
}
