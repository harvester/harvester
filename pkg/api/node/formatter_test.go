package node

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

var (
	testNode = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-58rk8",
		},
	}
	workingVM = &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-vm",
			Namespace: "default",
			Labels: map[string]string{
				kubevirtv1.NodeNameLabel: testNode.Name,
			},
		},
	}
	workingVolume = &lhv1beta2.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-volume",
			Namespace: "longhorn-system",
		},
		Status: lhv1beta2.VolumeStatus{
			KubernetesStatus: lhv1beta2.KubernetesStatus{
				Namespace: "default",
				WorkloadsStatus: []lhv1beta2.WorkloadStatus{
					{
						WorkloadName: "healthy-vm",
						WorkloadType: "VirtualMachineInstance",
					},
				},
			},
		},
	}

	workingReplica1 = &lhv1beta2.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-volume-r-1",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.ReplicaSpec{
			InstanceSpec: lhv1beta2.InstanceSpec{
				VolumeName: workingVolume.Name,
				NodeID:     testNode.Name,
			},
		},
		Status: lhv1beta2.ReplicaStatus{
			InstanceStatus: lhv1beta2.InstanceStatus{
				Started: true,
			},
		},
	}

	workingReplica2 = &lhv1beta2.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-volume-r-2",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.ReplicaSpec{
			InstanceSpec: lhv1beta2.InstanceSpec{
				VolumeName: workingVolume.Name,
				NodeID:     "harvester-1111",
			},
		},
		Status: lhv1beta2.ReplicaStatus{
			InstanceStatus: lhv1beta2.InstanceStatus{
				Started: true,
			},
		},
	}

	workingReplica3 = &lhv1beta2.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-volume-r-3",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.ReplicaSpec{
			InstanceSpec: lhv1beta2.InstanceSpec{
				VolumeName: workingVolume.Name,
				NodeID:     "harvester-2222",
			},
		},
		Status: lhv1beta2.ReplicaStatus{
			InstanceStatus: lhv1beta2.InstanceStatus{
				Started: true,
			},
		},
	}

	// VM with last working replica on node being drained
	failingVM = &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-vm",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "kubevirt.io/v1",
					Kind:       "VirtualMachine",
					Name:       "failing-vm",
				},
			},
		},
	}
	failingVolume = &lhv1beta2.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-volume",
			Namespace: "longhorn-system",
		},
		Status: lhv1beta2.VolumeStatus{
			KubernetesStatus: lhv1beta2.KubernetesStatus{
				Namespace: "default",
				WorkloadsStatus: []lhv1beta2.WorkloadStatus{
					{
						WorkloadName: failingVM.Name,
						WorkloadType: "VirtualMachineInstance",
					},
				},
			},
		},
	}

	failingReplica1 = &lhv1beta2.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-volume-r-1",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.ReplicaSpec{
			InstanceSpec: lhv1beta2.InstanceSpec{
				VolumeName: failingVolume.Name,
				NodeID:     testNode.Name,
			},
		},
		Status: lhv1beta2.ReplicaStatus{
			InstanceStatus: lhv1beta2.InstanceStatus{
				Started: true,
			},
		},
	}

	failingReplica2 = &lhv1beta2.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-volume-r-2",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.ReplicaSpec{
			InstanceSpec: lhv1beta2.InstanceSpec{
				VolumeName: failingVolume.Name,
				NodeID:     "harvester-1111",
			},
		},
		Status: lhv1beta2.ReplicaStatus{
			InstanceStatus: lhv1beta2.InstanceStatus{
				Started: false,
			},
		},
	}

	failingReplica3 = &lhv1beta2.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-volume-r-3",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.ReplicaSpec{
			InstanceSpec: lhv1beta2.InstanceSpec{
				VolumeName: failingVolume.Name,
				NodeID:     "harvester-2222",
			},
		},
		Status: lhv1beta2.ReplicaStatus{
			InstanceStatus: lhv1beta2.InstanceStatus{
				Started: false,
			},
		},
	}

	// VM with failing replicas but not on node in scope for drain
	failingVM2 = &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-vm2",
			Namespace: "default",
		},
	}
	failingVolume2 = &lhv1beta2.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-volume2",
			Namespace: "longhorn-system",
		},
		Status: lhv1beta2.VolumeStatus{
			KubernetesStatus: lhv1beta2.KubernetesStatus{
				Namespace: "default",
				WorkloadsStatus: []lhv1beta2.WorkloadStatus{
					{
						WorkloadName: failingVM2.Name,
						WorkloadType: "VirtualMachineInstance",
					},
				},
			},
		},
	}

	failingReplica12 = &lhv1beta2.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-volume2-r-1",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.ReplicaSpec{
			InstanceSpec: lhv1beta2.InstanceSpec{
				VolumeName: failingVolume2.Name,
				NodeID:     "harvester-1111",
			},
		},
		Status: lhv1beta2.ReplicaStatus{
			InstanceStatus: lhv1beta2.InstanceStatus{
				Started: true,
			},
		},
	}

	failingReplica22 = &lhv1beta2.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-volume2-r-2",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.ReplicaSpec{
			InstanceSpec: lhv1beta2.InstanceSpec{
				VolumeName: failingVolume2.Name,
				NodeID:     "harvester-1111",
			},
		},
		Status: lhv1beta2.ReplicaStatus{
			InstanceStatus: lhv1beta2.InstanceStatus{
				Started: false,
			},
		},
	}

	failingReplica32 = &lhv1beta2.Replica{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-volume2-r-3",
			Namespace: "longhorn-system",
		},
		Spec: lhv1beta2.ReplicaSpec{
			InstanceSpec: lhv1beta2.InstanceSpec{
				VolumeName: failingVolume2.Name,
				NodeID:     "harvester-2222",
			},
		},
		Status: lhv1beta2.ReplicaStatus{
			InstanceStatus: lhv1beta2.InstanceStatus{
				Started: false,
			},
		},
	}

	vmWithCDROM = &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cdrom-vm",
			Namespace: "default",
			Labels: map[string]string{
				kubevirtv1.NodeNameLabel: testNode.Name,
			},
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				Devices: kubevirtv1.Devices{
					Disks: []kubevirtv1.Disk{
						{
							DiskDevice: kubevirtv1.DiskDevice{
								CDRom: &kubevirtv1.CDRomTarget{},
							},
						},
					},
				},
			},
		},
	}

	vmWithContainerDisk = &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "containerdisk-vm",
			Namespace: "default",
			Labels: map[string]string{
				kubevirtv1.NodeNameLabel: testNode.Name,
			},
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Volumes: []kubevirtv1.Volume{
				{
					VolumeSource: kubevirtv1.VolumeSource{
						ContainerDisk: &kubevirtv1.ContainerDiskSource{},
					},
				},
			},
		},
	}
)

func Test_listUnhealthyVM(t *testing.T) {
	assert := require.New(t)
	typedObjects := []runtime.Object{workingVolume, workingVM, workingReplica1, workingReplica2, workingReplica3, failingVolume, failingVM,
		failingReplica1, failingReplica2, failingReplica3, failingVM2, failingVolume2, failingReplica12, failingReplica22, failingReplica32}
	client := fake.NewSimpleClientset(typedObjects...)
	k8sclientset := k8sfake.NewSimpleClientset(testNode)

	h := ActionHandler{
		nodeCache:                   fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
		nodeClient:                  fakeclients.NodeClient(k8sclientset.CoreV1().Nodes),
		longhornVolumeCache:         fakeclients.LonghornVolumeCache(client.LonghornV1beta2().Volumes),
		longhornReplicaCache:        fakeclients.LonghornReplicaCache(client.LonghornV1beta2().Replicas),
		virtualMachineInstanceCache: fakeclients.VirtualMachineInstanceCache(client.KubevirtV1().VirtualMachineInstances),
	}

	fakeHTTP := httptest.NewRecorder()
	err := h.listUnhealthyVM(fakeHTTP, testNode)
	assert.NoError(err, "expected no error while listing unhealthy VM's")
	resp := &ListUnhealthyVM{}
	err = json.NewDecoder(fakeHTTP.Body).Decode(resp)
	assert.NoError(err, "expected no error parsing json response")
	assert.Len(resp.VMs, 1, "expected to find one vm")
}

func Test_listUnmigratableVM(t *testing.T) {
	assert := require.New(t)
	typedObjects := []runtime.Object{workingVM, vmWithContainerDisk, vmWithCDROM}
	client := fake.NewSimpleClientset(typedObjects...)
	k8sclientset := k8sfake.NewSimpleClientset(testNode)

	h := ActionHandler{
		nodeCache:                   fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
		nodeClient:                  fakeclients.NodeClient(k8sclientset.CoreV1().Nodes),
		longhornVolumeCache:         fakeclients.LonghornVolumeCache(client.LonghornV1beta1().Volumes),
		longhornReplicaCache:        fakeclients.LonghornReplicaCache(client.LonghornV1beta1().Replicas),
		virtualMachineInstanceCache: fakeclients.VirtualMachineInstanceCache(client.KubevirtV1().VirtualMachineInstances),
	}

	fakeHTTP := httptest.NewRecorder()
	err := h.listUnhealthyVM(fakeHTTP, testNode)
	assert.NoError(err, "expected no error while listing unhealthy VM's")
	resp := &ListUnhealthyVM{}
	err = json.NewDecoder(fakeHTTP.Body).Decode(resp)
	assert.NoError(err, "expected no error parsing json response")
	assert.Len(resp.VMs, 2, "expected to find two vms")
}
