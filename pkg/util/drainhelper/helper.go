package drainhelper

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/selection"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/drain"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	"github.com/harvester/harvester/pkg/util"
)

// drain helper leverages k8s.io/kubectl/pkg/drain to evict pods while following longhorn
// kubevirt specific guidelines

const (
	// combination of recommendations from longhorn and kubevirt docs
	// https://kubevirt.io/user-guide/operations/node_maintenance/
	//https://longhorn.io/docs/1.3.1/volumes-and-nodes/maintenance/#updating-the-node-os-or-container-runtime
	defaultSkipPodLabels      = "app!=csi-attacher,app!=csi-provisioner,kubevirt.io!=hotplug-disk"
	defaultGracePeriodSeconds = 180
	defaultTimeOut            = 240 * time.Second
	DrainAnnotation           = "harvesterhci.io/drain-requested"
	ForcedDrain               = "harvesterhci.io/drain-forced"
	defaultSingleCPCount      = 1
	defaultHACPCount          = 3
)

var (
	errSingleControlPlaneNode = errors.New("single controlplane cluster, cannot place controlplane in maintenance mode")
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
		AdditionalFilters:   []drain.PodFilter{maintainModeStrategyFilter},
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
	_, etcdLabelOK := node.Labels["node-role.kubernetes.io/etcd"]

	if !cpLabelOK && !etcdLabelOK { // not a controlplane node. no further action needed
		return nil
	}

	cpReq, err := labels.NewRequirement("node-role.kubernetes.io/control-plane", selection.Exists, nil)
	if err != nil {
		return fmt.Errorf("error creating requirement: %v", err)
	}

	cpSelector := labels.NewSelector()
	cpSelector = cpSelector.Add(*cpReq)

	cpNodeList, err := nodeCache.List(cpSelector)
	if err != nil {
		return fmt.Errorf("error listing nodes matching selector %s: %v", cpSelector.String(), err)
	}

	etcdReq, err := labels.NewRequirement("node-role.kubernetes.io/etcd", selection.Exists, nil)
	if err != nil {
		return fmt.Errorf("error creating requirement: %v", err)
	}

	etcdSelector := labels.NewSelector()
	etcdSelector = etcdSelector.Add(*etcdReq)

	etcdNodeList, err := nodeCache.List(etcdSelector)
	if err != nil {
		return fmt.Errorf("error listing nodes matching selector %s: %v", etcdSelector.String(), err)
	}

	// add items to map since controlplane nodes will have etcd labels
	// this is needed to ensure CI passes on kind since no etcd labels are added
	// to controlplane nodes in kind
	nodeMap := make(map[string]*corev1.Node)
	for _, v := range cpNodeList {
		nodeMap[v.Name] = v
	}
	for _, v := range etcdNodeList {
		nodeMap[v.Name] = v
	}
	// only controlplane node which we are trying to place into maintenance
	if len(nodeMap) == defaultSingleCPCount {
		return errSingleControlPlaneNode
	}

	var availableNodes int
	for _, v := range nodeMap {
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

func maintainModeStrategyFilter(pod corev1.Pod) drain.PodDeleteStatus {
	// Ignore VMs that should not be migrated in maintenance mode. These
	// VMs are forcibly shut down when maintenance mode is activated.
	value, ok := pod.Labels[util.LabelMaintainModeStrategy]
	if ok && value != util.MaintainModeStrategyMigrate {
		logrus.WithFields(logrus.Fields{
			"namespace": pod.Namespace,
			"pod_name":  pod.Name,
		}).Infof("migration of pod owned by VM %s is skipped because of the label %s",
			pod.Labels[util.LabelVMName], util.LabelMaintainModeStrategy)
		return drain.MakePodDeleteStatusSkip()
	}
	return drain.MakePodDeleteStatusOkay()
}
