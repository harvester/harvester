package nodedrain

import (
	"context"
	"fmt"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/config"
	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	longhornv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/drainhelper"
)

const (
	nodeDrainController  = "node-drain-controller"
	defaultWorkloadType  = "VirtualMachineInstance"
	defaultSingleCPCount = 1
	defaultHACPCount     = 3
)

// ControllerHandler to drain nodes.
// the controller checks if node has any replicas which may be the only working replica for a VM and attempts to shutdown the VM
// before the drain is initiated. This ensures that no data is lost when the instance-managers are terminated as
// part of the drain process
type ControllerHandler struct {
	nodes                        ctlcorev1.NodeClient
	nodeCache                    ctlcorev1.NodeCache
	virtualMachineInstanceCache  ctlkubevirtv1.VirtualMachineInstanceCache
	virtualMachineInstanceClient ctlkubevirtv1.VirtualMachineInstanceClient
	virtualMachineClient         ctlkubevirtv1.VirtualMachineClient
	virtualMachineCache          ctlkubevirtv1.VirtualMachineCache
	longhornVolumeCache          longhornv1beta1.VolumeCache
	longhornReplicaCache         longhornv1beta1.ReplicaCache
	restConfig                   *rest.Config
	context                      context.Context
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	lhv := management.LonghornFactory.Longhorn().V1beta1().Volume()
	lhr := management.LonghornFactory.Longhorn().V1beta1().Replica()
	ndc := &ControllerHandler{
		nodes:                        nodes,
		nodeCache:                    nodes.Cache(),
		virtualMachineInstanceCache:  vmis.Cache(),
		virtualMachineInstanceClient: vmis,
		virtualMachineClient:         vms,
		virtualMachineCache:          vms.Cache(),
		longhornReplicaCache:         lhr.Cache(),
		longhornVolumeCache:          lhv.Cache(),
		restConfig:                   management.RestConfig,
		context:                      ctx,
	}

	nodes.OnChange(ctx, nodeDrainController, ndc.OnNodeChange)
	return nil
}

// OnNodeChange handles reconcile logic for node drains
func (ndc *ControllerHandler) OnNodeChange(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}
	_, ok := node.Annotations[drainhelper.DrainAnnotation]

	if !ok {
		return node, nil
	}

	_, forced := node.Annotations[drainhelper.ForcedDrain]

	if ok {
		// still running a check in the background to avoid maintenance issues when using object annotations
		// directly
		err := drainhelper.DrainPossible(ndc.nodeCache, node)
		if err != nil {
			return node, err
		}

		logrus.Infof("attempting to place node %s in maintenance mode", node.Name)
		if forced {
			shutdownVMs, err := ndc.listVMI(node)
			if err != nil {
				return node, fmt.Errorf("error listing VMIs in scope for shutdown: %v", err)
			}

			for _, v := range shutdownVMs {
				// fetch VMI again in case its changed
				err := ndc.findAndStopVM(v)
				if err != nil {
					return node, err
				}
			}
		}
		// run node drain
		nodeCopy := node.DeepCopy()
		err = drainhelper.DrainNode(ndc.context, ndc.restConfig, nodeCopy)
		if err != nil {
			return node, err
		}

		nodeCopy.Annotations[ctlnode.MaintainStatusAnnotationKey] = ctlnode.MaintainStatusRunning
		delete(nodeCopy.Annotations, drainhelper.DrainAnnotation)
		delete(nodeCopy.Annotations, drainhelper.ForcedDrain)
		return ndc.nodes.Update(nodeCopy)
	}
	return node, nil
}

// findAndStopVM is a wrapper function to identify the owner VM for a VMI, and patch the run strategy
func (ndc *ControllerHandler) findAndStopVM(vmi *kubevirtv1.VirtualMachineInstance) error {

	vm, err := findVM(vmi)
	if err != nil {
		return err
	}
	vmObj, err := ndc.virtualMachineCache.Get(vmi.Namespace, vm)
	if err != nil {
		return fmt.Errorf("error looking up vm %s in namespace %s in vm cache: %v", vmi.Name, vmi.Namespace, err)
	}

	vmObjCopy := vmObj.DeepCopy()
	vmObjCopy.Spec.RunStrategy = &[]kubevirtv1.VirtualMachineRunStrategy{kubevirtv1.RunStrategyHalted}[0]
	_, err = ndc.virtualMachineClient.Update(vmObjCopy)
	if err != nil {
		return fmt.Errorf("error updating run strategy for vm %s in namespace %s: %v", vmObj.Name, vmObj.Namespace, err)
	}

	return nil
}

// findVM is a wrapper function to identify VM from VMI owner references
func findVM(vmi *kubevirtv1.VirtualMachineInstance) (string, error) {
	refs := vmi.GetOwnerReferences()
	for _, owner := range refs {
		if owner.Kind == "VirtualMachine" && owner.APIVersion == "kubevirt.io/v1" {
			return owner.Name, nil
		}
	}
	return "", fmt.Errorf("no valid VM owner found for VMI %s", vmi.Name)
}

// list VMI will list VM's which may have their last healthy replica on current node. The VM itself may be
// scheduled on a different VM
func (ndc *ControllerHandler) listVMI(node *corev1.Node) ([]*kubevirtv1.VirtualMachineInstance, error) {
	var vmiList []*kubevirtv1.VirtualMachineInstance
	volList, err := ndc.listVolumeNames(node)
	if err != nil {
		return nil, fmt.Errorf("error in listVolumeNames: %v", err)
	}

	for _, v := range volList {
		for _, workloads := range v.Status.KubernetesStatus.WorkloadsStatus {
			if workloads.WorkloadType == defaultWorkloadType {
				vmiObj, err := ndc.virtualMachineInstanceCache.Get(v.Status.KubernetesStatus.Namespace, workloads.WorkloadName)
				if err != nil {
					return nil, err
				}
				vmiList = append(vmiList, vmiObj)
			}
		}
	}
	return vmiList, nil
}

// listVolumeNames will filter on all loghorn volumes, and identify volumes with only 1 working replica
// which is currently on the node in scope for drain.
func (ndc *ControllerHandler) listVolumeNames(node *corev1.Node) ([]*longhornv1.Volume, error) {
	type internalVolumeDetails struct {
		healthy   int
		unhealthy int
		replicas  []*longhornv1.Replica
	}
	volumeMap := map[string]internalVolumeDetails{}

	replicaList, err := ndc.longhornReplicaCache.List(util.LonghornSystemNamespaceName, labels.NewSelector())
	if err != nil {
		return nil, err
	}

	// identify PVC's with just one working replica
	for _, r := range replicaList {
		v, ok := volumeMap[r.Spec.VolumeName]
		if !ok {
			v = internalVolumeDetails{}
		}
		if r.Status.Started {
			v.healthy++
		} else {
			v.unhealthy++
		}
		v.replicas = append(v.replicas, r)
		volumeMap[r.Spec.VolumeName] = v
	}

	var possibleVolumeNames []string

	for pvcName, replicaState := range volumeMap {
		if replicaState.healthy <= 1 {
			for _, v := range replicaState.replicas {
				// last started replica is on the current node
				if v.Status.Started && v.Spec.NodeID == node.Name {
					possibleVolumeNames = append(possibleVolumeNames, pvcName)
				}
			}
		}
	}

	volList := make([]*longhornv1.Volume, 0, len(possibleVolumeNames))
	for _, v := range possibleVolumeNames {
		vObj, err := ndc.longhornVolumeCache.Get(util.LonghornSystemNamespaceName, v)
		if err != nil {
			return nil, err
		}
		volList = append(volList, vObj)
	}

	return volList, nil
}

// findAndListVM is called by action handler to leverage caches to find unhealthy VM's impacted by the migration
func (ndc *ControllerHandler) FindAndListVM(node *corev1.Node) ([]string, error) {
	shutdownVMs, err := ndc.listVMI(node)
	if err != nil {
		return nil, fmt.Errorf("error listing VMI: %v", err)
	}
	impactedVMDetails := make([]string, 0, len(shutdownVMs))
	for _, v := range shutdownVMs {
		vmName, err := findVM(v)
		if err != nil {
			return nil, err
		}
		impactedVMDetails = append(impactedVMDetails, fmt.Sprintf("%s/%s", v.Namespace, vmName))
	}
	return impactedVMDetails, nil
}

func ActionHelper(nodeCache ctlcorev1.NodeCache, virtualMachineInstanceCache ctlkubevirtv1.VirtualMachineInstanceCache,
	longhornVolumeCache longhornv1beta1.VolumeCache, longhornReplicaCache longhornv1beta1.ReplicaCache) *ControllerHandler {
	return &ControllerHandler{
		nodeCache:                   nodeCache,
		virtualMachineInstanceCache: virtualMachineInstanceCache,
		longhornVolumeCache:         longhornVolumeCache,
		longhornReplicaCache:        longhornReplicaCache,
	}
}
