package nodedrain

import (
	"context"
	"encoding/json"
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
)

func Test_listVMI(t *testing.T) {
	assert := require.New(t)
	typedObjects := []runtime.Object{workingVolume, workingVM, workingReplica1, workingReplica2, workingReplica3, failingVolume, failingVM,
		failingReplica1, failingReplica2, failingReplica3, failingVM2, failingVolume2, failingReplica12, failingReplica22, failingReplica32}
	client := fake.NewSimpleClientset(typedObjects...)
	k8sclientset := k8sfake.NewSimpleClientset(testNode)

	ndc := &ControllerHandler{
		nodes:                        fakeclients.NodeClient(k8sclientset.CoreV1().Nodes),
		nodeCache:                    fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
		virtualMachineInstanceClient: fakeclients.VirtualMachineInstanceClient(client.KubevirtV1().VirtualMachineInstances),
		virtualMachineInstanceCache:  fakeclients.VirtualMachineInstanceCache(client.KubevirtV1().VirtualMachineInstances),
		virtualMachineClient:         fakeclients.VirtualMachineClient(client.KubevirtV1().VirtualMachines),
		virtualMachineCache:          fakeclients.VirtualMachineCache(client.KubevirtV1().VirtualMachines),
		longhornVolumeCache:          fakeclients.LonghornVolumeCache(client.LonghornV1beta2().Volumes),
		longhornReplicaCache:         fakeclients.LonghornReplicaCache(client.LonghornV1beta2().Replicas),
		restConfig:                   nil,
		context:                      context.TODO(),
	}

	vmiList, err := ndc.listVMI(testNode)
	assert.NoError(err, "expected no error")
	assert.Len(vmiList, 1, "expected to find only 1 vmi")
	assert.Contains(vmiList, failingVM, "expected to find failingVM only")
}

const vmiListString = `
{
    "apiVersion": "kubevirt.io/v1",
    "kind": "VirtualMachineInstance",
    "metadata": {
        "annotations": {
            "harvesterhci.io/sshNames": "[\"default/gm\"]",
            "kubevirt.io/latest-observed-api-version": "v1",
            "kubevirt.io/storage-observed-api-version": "v1",
            "kubevirt.io/vm-generation": "2"
        },
        "creationTimestamp": "2024-08-22T02:38:46Z",
        "finalizers": [
            "kubevirt.io/virtualMachineControllerFinalize",
            "foregroundDeleteVirtualMachine",
            "wrangler.cattle.io/harvester-lb-vmi-controller"
        ],
        "generation": 12,
        "labels": {
            "harvesterhci.io/vmName": "pinned-to-host",
            "kubevirt.io/nodeName": "harvester-kfs2c"
        },
        "name": "pinned-to-host",
        "namespace": "default",
        "ownerReferences": [
            {
                "apiVersion": "kubevirt.io/v1",
                "blockOwnerDeletion": true,
                "controller": true,
                "kind": "VirtualMachine",
                "name": "pinned-to-host",
                "uid": "c0630daf-d1d2-417b-91d6-a707f0e7e9d0"
            }
        ],
        "resourceVersion": "10393539",
        "uid": "6dd94105-90c6-4646-8fe7-3341b775d7ad"
    },
    "spec": {
        "affinity": {
            "nodeAffinity": {
                "requiredDuringSchedulingIgnoredDuringExecution": {
                    "nodeSelectorTerms": [
                        {
                            "matchExpressions": [
                                {
                                    "key": "network.harvesterhci.io/mgmt",
                                    "operator": "In",
                                    "values": [
                                        "true"
                                    ]
                                }
                            ]
                        }
                    ]
                }
            }
        },
        "architecture": "amd64",
        "domain": {
            "cpu": {
                "cores": 2,
                "model": "host-model",
                "sockets": 1,
                "threads": 1
            },
            "devices": {
                "disks": [
                    {
                        "bootOrder": 1,
                        "disk": {
                            "bus": "virtio"
                        },
                        "name": "disk-0"
                    },
                    {
                        "disk": {
                            "bus": "virtio"
                        },
                        "name": "cloudinitdisk"
                    }
                ],
                "inputs": [
                    {
                        "bus": "usb",
                        "name": "tablet",
                        "type": "tablet"
                    }
                ],
                "interfaces": [
                    {
                        "bridge": {},
                        "model": "virtio",
                        "name": "default"
                    }
                ]
            },
            "features": {
                "acpi": {
                    "enabled": true
                }
            },
            "firmware": {
                "uuid": "eb76ee29-5ae6-57ad-b9c8-b238feb1e709"
            },
            "machine": {
                "type": "q35"
            },
            "memory": {
                "guest": "3996Mi"
            },
            "resources": {
                "limits": {
                    "cpu": "2",
                    "memory": "4Gi"
                },
                "requests": {
                    "cpu": "125m",
                    "memory": "2730Mi"
                }
            }
        },
        "evictionStrategy": "LiveMigrateIfPossible",
        "hostname": "pinned-to-host",
        "networks": [
            {
                "multus": {
                    "networkName": "default/workload"
                },
                "name": "default"
            }
        ],
        "nodeSelector": {
            "kubernetes.io/hostname": "harvester-kfs2c"
        },
        "terminationGracePeriodSeconds": 120,
        "volumes": [
            {
                "name": "disk-0",
                "persistentVolumeClaim": {
                    "claimName": "pinned-to-host-disk-0-dopwc"
                }
            },
            {
                "cloudInitNoCloud": {
                    "networkDataSecretRef": {
                        "name": "pinned-to-host-5bphm"
                    },
                    "secretRef": {
                        "name": "pinned-to-host-5bphm"
                    }
                },
                "name": "cloudinitdisk"
            }
        ]
    },
    "status": {
        "activePods": {
            "59b3891b-7043-403a-8688-414942ae40fc": "harvester-kfs2c"
        },
        "conditions": [
            {
                "lastProbeTime": null,
                "lastTransitionTime": "2024-08-22T02:39:08Z",
                "status": "True",
                "type": "Ready"
            },
            {
                "lastProbeTime": null,
                "lastTransitionTime": null,
                "status": "True",
                "type": "LiveMigratable"
            },
            {
                "lastProbeTime": "2024-08-22T02:39:55Z",
                "lastTransitionTime": null,
                "status": "True",
                "type": "AgentConnected"
            }
        ],
        "currentCPUTopology": {
            "cores": 2,
            "sockets": 1,
            "threads": 1
        },
        "guestOSInfo": {
            "id": "opensuse-leap",
            "kernelRelease": "6.4.0-150600.23.17-default",
            "kernelVersion": "#1 SMP PREEMPT_DYNAMIC Tue Jul 30 06:37:32 UTC 2024 (9c450d7)",
            "name": "openSUSE Leap",
            "prettyName": "openSUSE Leap 15.6",
            "version": "15.6",
            "versionId": "15.6"
        },
        "interfaces": [
            {
                "infoSource": "domain, guest-agent, multus-status",
                "interfaceName": "eth0",
                "ipAddress": "172.19.106.159",
                "ipAddresses": [
                    "172.19.106.159",
                    "fe80::ca9:6fff:fe37:139"
                ],
                "mac": "0e:a9:6f:37:01:39",
                "name": "default",
                "queueCount": 1
            }
        ],
        "launcherContainerImageVersion": "registry.suse.com/suse/sles/15.5/virt-launcher:1.1.1-150500.8.15.1",
        "machine": {
            "type": "pc-q35-7.1"
        },
        "memory": {
            "guestAtBoot": "3996Mi",
            "guestCurrent": "3996Mi",
            "guestRequested": "3996Mi"
        },
        "migrationMethod": "BlockMigration",
        "migrationTransport": "Unix",
        "nodeName": "harvester-kfs2c",
        "phase": "Running",
        "phaseTransitionTimestamps": [
            {
                "phase": "Pending",
                "phaseTransitionTimestamp": "2024-08-22T02:38:46Z"
            },
            {
                "phase": "Scheduling",
                "phaseTransitionTimestamp": "2024-08-22T02:38:47Z"
            },
            {
                "phase": "Scheduled",
                "phaseTransitionTimestamp": "2024-08-22T02:39:08Z"
            },
            {
                "phase": "Running",
                "phaseTransitionTimestamp": "2024-08-22T02:39:11Z"
            }
        ],
        "qosClass": "Burstable",
        "runtimeUser": 107,
        "selinuxContext": "none",
        "virtualMachineRevisionName": "revision-start-vm-c0630daf-d1d2-417b-91d6-a707f0e7e9d0-1",
        "volumeStatus": [
            {
                "name": "cloudinitdisk",
                "size": 1048576,
                "target": "vdb"
            },
            {
                "name": "disk-0",
                "persistentVolumeClaimInfo": {
                    "accessModes": [
                        "ReadWriteMany"
                    ],
                    "capacity": {
                        "storage": "10Gi"
                    },
                    "filesystemOverhead": "0.055",
                    "requests": {
                        "storage": "10Gi"
                    },
                    "volumeMode": "Block"
                },
                "target": "vda"
            }
        ]
    }
}`

func Test_virtualMachineContainsHostName(t *testing.T) {
	assert := require.New(t)
	vmi := &kubevirtv1.VirtualMachineInstance{}
	err := json.Unmarshal([]byte(vmiListString), vmi)
	assert.NoError(err, "exepcted no error during generation of vmi list")
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-kfs2c",
			Labels: map[string]string{
				"kubernetes.io/hostname":       "harvester-kfs2c",
				"network.harvesterhci.io/mgmt": "true",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-qhgd4",
			Labels: map[string]string{
				"kubernetes.io/hostname":       "harvester-qhgd4",
				"network.harvesterhci.io/mgmt": "true",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	node3 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-rmvzg",
			Labels: map[string]string{
				"kubernetes.io/hostname":       "harvester-rmvzg",
				"network.harvesterhci.io/mgmt": "true",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	k8sclientset := k8sfake.NewSimpleClientset(node1, node2, node3)

	ndc := &ControllerHandler{
		nodes:      fakeclients.NodeClient(k8sclientset.CoreV1().Nodes),
		nodeCache:  fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
		restConfig: nil,
		context:    context.TODO(),
	}

	var vmiList []*kubevirtv1.VirtualMachineInstance
	vmiList = append(vmiList, vmi)
	nonMigratableVMIs, err := ndc.CheckVMISchedulingRequirements(node1, vmiList)
	assert.NoError(err)
	assert.Len(nonMigratableVMIs, 1, "expected to find 1 VMI")
	vmi.Spec.Affinity = nil // simulate a masquerade network
	nonMigratableVMIs, err = ndc.CheckVMISchedulingRequirements(node1, vmiList)
	assert.NoError(err)
	assert.Len(nonMigratableVMIs, 1, "expected to find 1 VMI")

}
