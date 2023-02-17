package drainhelper

import (
	"context"
	"errors"
	"fmt"
	"time"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/drain"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
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
	defaultSingleCPCount      = 1
	defaultHACPCount          = 3
)

var (
	errSingleControlPlaneNode = errors.New("single controlplane cluster, cannot plance controlplane in maintenance mode")
	errHAControlPlaneNode     = errors.New("another controlplane is already in maintenance mode, cannot place current node in maintenance mode")
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

// DrainPossible is a helper method to check node object and query remaining nodes in cluster
// to identify if it is possible to place the current mode in maintenance mode
func DrainPossible(nodeCache ctlcorev1.NodeCache, node *corev1.Node) error {
	_, cpLabelOK := node.Labels["node-role.kubernetes.io/control-plane"]
	if !cpLabelOK { // not a controlplane node. no further action needed
		return nil
	}

	req, err := labels.NewRequirement("node-role.kubernetes.io/control-plane", selection.Exists, nil)
	if err != nil {
		return fmt.Errorf("error creating requirement: %v", err)
	}

	cpSelector := labels.NewSelector()
	cpSelector = cpSelector.Add(*req)

	nodeList, err := nodeCache.List(cpSelector)
	if err != nil {
		return fmt.Errorf("error listing nodes matching selector %s: %v", cpSelector.String(), err)
	}

	// only controlplane node which we are trying to place into maintenance
	if len(nodeList) == defaultSingleCPCount {
		return errSingleControlPlaneNode
	}

	var availableNodes int
	for _, v := range nodeList {
		_, ok := v.Annotations[ctlnode.MaintainStatusAnnotationKey]
		logrus.Debugf("nodeName: %s,  annotation present: %v", v.Name, ok)
		if !ok {
			availableNodes++
		}
	}

	if availableNodes != defaultHACPCount {
		return errHAControlPlaneNode
	}

	return nil
}
