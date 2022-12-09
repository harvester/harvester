package drainhelper

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/drain"
)

// drain helper leverages k8s.io/kubectl/pkg/drain to evict pods while following longhorn
// kubevirt specific guidelines

const (
	// combination of recommendations from longhorn and kubevirt docs
	// https://kubevirt.io/user-guide/operations/node_maintenance/
	//https://longhorn.io/docs/1.3.1/volumes-and-nodes/maintenance/#updating-the-node-os-or-container-runtime
	defaultSkipPodLabels      = "app!=csi-attacher,app!=csi-provisioner"
	defaultGracePeriodSeconds = 180
	defaultTimeOut            = 240 * time.Second
	DrainAnnotation           = "harvesterhci.io/drain-requested"
	ForcedDrain               = "harvesterhci.io/drain-forced"
)

func defaultDrainHelper(ctx context.Context, cfg *rest.Config) (*drain.Helper, error) {
	logger := logrus.New()
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &drain.Helper{
		Ctx:                 ctx,
		GracePeriodSeconds:  defaultGracePeriodSeconds,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  true,
		PodSelector:         defaultSkipPodLabels,
		Client:              client,
		Force:               true,
		Out:                 logger.Writer(),
		ErrOut:              logger.Writer(),
		Timeout:             defaultTimeOut,
	}, nil
}

func DrainNode(ctx context.Context, cfg *rest.Config, node *corev1.Node) error {
	d, err := defaultDrainHelper(ctx, cfg)
	if err != nil {
		return fmt.Errorf("unable to create node drain helper: %v", err)
	}

	err = drain.RunCordonOrUncordon(d, node, true)
	if err != nil {
		return fmt.Errorf("error during node cordon: %v", err)
	}

	return drain.RunNodeDrain(d, node.Name)
}
