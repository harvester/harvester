package ready

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/rancher/wrangler/pkg/generated/controllers/apps"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/wait"
	restclient "k8s.io/client-go/rest"
)

const (
	defaultInterval = 3 * time.Second
	defaultTimeout  = 10 * time.Minute
)

var (
	logf = ginkgo.GinkgoT().Logf
)

type Condition struct {
	name              string
	wc                wait.ConditionFunc
	interval, timeout time.Duration
}

func (c *Condition) Wait(ctx context.Context) error {
	return wait.PollImmediate(c.interval, c.timeout, func() (bool, error) {
		select {
		case <-ctx.Done():
			return true, nil
		default:
			return c.wc()
		}
	})
}

func NewCondition(name string, wc wait.ConditionFunc, interval, timeout time.Duration) *Condition {
	return &Condition{
		name:     name,
		wc:       wc,
		interval: interval,
		timeout:  timeout,
	}
}

func NewDefaultCondition(name string, wc wait.ConditionFunc) *Condition {
	return NewCondition(name, wc, defaultInterval, defaultTimeout)
}

type ConditionList []*Condition

func (cl ConditionList) Wait(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, condition := range cl {
		condition := condition
		g.Go(func() error {
			defer func() {
				logf("condition %s is ok", condition.name)
			}()
			logf("wait for condition %s", condition.name)
			return condition.Wait(ctx)
		})
	}
	return g.Wait()
}

type AppsConditionFunc func(appsFactory *apps.Factory, namespace, name string) (string, wait.ConditionFunc)

type NamespaceCondition struct {
	namespace     string
	kubeConfig    *restclient.Config
	appsFactory   *apps.Factory
	conditionList ConditionList
}

func NewNamespaceCondition(kubeConfig *restclient.Config, namespace string) (*NamespaceCondition, error) {
	appsFactory, err := apps.NewFactoryFromConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	return &NamespaceCondition{
		namespace:   namespace,
		kubeConfig:  kubeConfig,
		appsFactory: appsFactory,
	}, nil
}

func (n *NamespaceCondition) AddAppsCondition(appsConditionFunc AppsConditionFunc, names ...string) {
	for _, name := range names {
		kind, wc := appsConditionFunc(n.appsFactory, n.namespace, name)
		condition := NewDefaultCondition(fmt.Sprintf("%s %s", name, kind), wc)
		n.conditionList = append(n.conditionList, condition)
	}
}

func (n *NamespaceCondition) AddDeploymentsReady(names ...string) {
	n.AddAppsCondition(isDeploymentReady, names...)
}

func (n *NamespaceCondition) AddDaemonSetsReady(names ...string) {
	n.AddAppsCondition(isDaemenSetReady, names...)
}

func (n *NamespaceCondition) AddDeploymentsClean(names ...string) {
	n.AddAppsCondition(isDeploymentClean, names...)
}

func (n *NamespaceCondition) AddDaemonSetsClean(names ...string) {
	n.AddAppsCondition(isDaemenSetClean, names...)
}

func (n *NamespaceCondition) Wait(ctx context.Context) error {
	return n.conditionList.Wait(ctx)
}
