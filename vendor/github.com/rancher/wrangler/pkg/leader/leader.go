package leader

import (
	"context"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type Callback func(cb context.Context)

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

	rl, err := resourcelock.New(resourcelock.ConfigMapsLeasesResourceLock,
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

	t := time.Second
	if dl := os.Getenv("CATTLE_DEV_MODE"); dl != "" {
		t = time.Hour
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: 45 * t,
		RenewDeadline: 30 * t,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
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
		},
		ReleaseOnCancel: true,
	})
	panic("unreachable")
}
