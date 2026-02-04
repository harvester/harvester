package leader

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type Callback func(cb context.Context)

const devModeEnvKey = "CATTLE_DEV_MODE"
const leaseDurationEnvKey = "CATTLE_ELECTION_LEASE_DURATION"
const renewDeadlineEnvKey = "CATTLE_ELECTION_RENEW_DEADLINE"
const retryPeriodEnvKey = "CATTLE_ELECTION_RETRY_PERIOD"

const defaultLeaseDuration = 45 * time.Second
const defaultRenewDeadline = 30 * time.Second
const defaultRetryPeriod = 2 * time.Second

const developmentLeaseDuration = 45 * time.Hour
const developmentRenewDeadline = 30 * time.Hour

func RunOrDie(ctx context.Context, namespace, name string, client kubernetes.Interface, cb Callback) {
	if namespace == "" {
		namespace = "kube-system"
	}

	err := run(ctx, namespace, name, client, cb)
	if err != nil {
		logrus.Fatalf("Failed to start leader election for %s", name)
	}
	panic("Failed to start leader election for " + name)
}

func run(ctx context.Context, namespace, name string, client kubernetes.Interface, cb Callback) error {
	id, err := os.Hostname()
	if err != nil {
		return err
	}

	rl, err := resourcelock.New(resourcelock.LeasesResourceLock,
		namespace,
		name,
		client.CoreV1(),
		client.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: id,
		})
	if err != nil {
		logrus.Fatalf("error creating leader lock for %s: %v", name, err)
	}

	cbs := leaderelection.LeaderCallbacks{
		OnStartedLeading: func(ctx context.Context) {
			go cb(ctx)
		},
		OnStoppedLeading: func() {
			select {
			case <-ctx.Done():
				// The context has been canceled or is otherwise complete.
				// This is a request to terminate. Exit 0.
				// Exiting cleanly is useful when the context is canceled
				// so that Kubernetes doesn't record it exiting in error
				// when the exit was requested. For example, the wrangler-cli
				// package sets up a context that cancels when SIGTERM is
				// sent in. If a node is shut down this is the type of signal
				// sent. In that case you want the 0 exit code to mark it as
				// complete so that everything comes back up correctly after
				// a restart.
				// The pattern found here can be found inside the kube-scheduler.
				logrus.Info("requested to terminate, exiting")
				os.Exit(0)
			default:
				logrus.Fatalf("leaderelection lost for %s", name)
			}
		},
	}

	config, err := computeConfig(rl, cbs)
	if err != nil {
		return err
	}

	leaderelection.RunOrDie(ctx, *config)
	panic("unreachable")
}

func computeConfig(rl resourcelock.Interface, cbs leaderelection.LeaderCallbacks) (*leaderelection.LeaderElectionConfig, error) {
	leaseDuration := defaultLeaseDuration
	renewDeadline := defaultRenewDeadline
	retryPeriod := defaultRetryPeriod
	var err error
	if d := os.Getenv(devModeEnvKey); d != "" {
		leaseDuration = developmentLeaseDuration
		renewDeadline = developmentRenewDeadline
	}
	if d := os.Getenv(leaseDurationEnvKey); d != "" {
		leaseDuration, err = time.ParseDuration(d)
		if err != nil {
			return nil, fmt.Errorf("%s value [%s] is not a valid duration: %w", leaseDurationEnvKey, d, err)
		}
	}
	if d := os.Getenv(renewDeadlineEnvKey); d != "" {
		renewDeadline, err = time.ParseDuration(d)
		if err != nil {
			return nil, fmt.Errorf("%s value [%s] is not a valid duration: %w", renewDeadlineEnvKey, d, err)
		}
	}
	if d := os.Getenv(retryPeriodEnvKey); d != "" {
		retryPeriod, err = time.ParseDuration(d)
		if err != nil {
			return nil, fmt.Errorf("%s value [%s] is not a valid duration: %w", retryPeriodEnvKey, d, err)
		}
	}

	return &leaderelection.LeaderElectionConfig{
		Lock:            rl,
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryPeriod,
		Callbacks:       cbs,
		ReleaseOnCancel: true,
	}, nil
}
