package node

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

var (
	testNode = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-58rk8",
			Labels: map[string]string{
				"network.harvesterhci.io/mgmt":      "true",
				"network.harvesterhci.io/secondary": "true",
			},
		},
	}

	testNode2 = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-8999s",
			Labels: map[string]string{
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

	// node not ready
	testNode3 = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-8112a",
			Labels: map[string]string{
				"network.harvesterhci.io/mgmt":      "true",
				"network.harvesterhci.io/secondary": "true",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	// node ready but cordoned off
	testNode4 = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-1123s",
			Labels: map[string]string{
				"network.harvesterhci.io/mgmt":      "true",
				"network.harvesterhci.io/secondary": "true",
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
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

	seederAddon = &harvesterv1beta1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      seederAddonName,
			Namespace: defaultAddonNamespace,
		},
		Spec: harvesterv1beta1.AddonSpec{
			Enabled: true,
		},
	}

	dynamicInventoryObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "metal.harvesterhci.io/v1alpha1",
			"kind":       "Inventory",
			"metadata": map[string]interface{}{
				"name":      testNode.Name,
				"namespace": defaultAddonNamespace,
			},
			"status": map[string]interface{}{
				"status": nodeReady,
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

	vmWithPCIDevice = &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pcidevice-vm",
			Namespace: "default",
			Labels: map[string]string{
				kubevirtv1.NodeNameLabel: testNode.Name,
			},
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				Devices: kubevirtv1.Devices{
					HostDevices: []kubevirtv1.HostDevice{
						{
							Name:       "fake-pcidevice",
							DeviceName: "fakevendor.com/FAKE_DEVICE_NAME",
						},
					},
				},
			},
		},
		Status: kubevirtv1.VirtualMachineInstanceStatus{
			Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
				{
					Type:   kubevirtv1.VirtualMachineInstanceIsMigratable,
					Status: corev1.ConditionFalse,
					Reason: "HostDeviceNotLiveMigratable",
				},
			},
		},
	}

	vmWithSchedulingCriteriaMet = &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm-scheduling-possible",
			Namespace: "default",
			Labels: map[string]string{
				kubevirtv1.NodeNameLabel: testNode.Name,
			},
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "network.harvesterhci.io/mgmt",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	vmWithSchedulingCriteriaNotMet = &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm-scheduling-not-possible",
			Namespace: "default",
			Labels: map[string]string{
				kubevirtv1.NodeNameLabel: testNode.Name,
			},
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "network.harvesterhci.io/secondary",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
								},
							},
						},
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
	resp := []ListUnhealthyVM{}
	err = json.NewDecoder(fakeHTTP.Body).Decode(&resp)
	assert.NoError(err, "expected no error parsing json response")
	assert.Len(resp[0].VMs, 1, "expected to find one vm")
}

func Test_powerActionNotPossible(t *testing.T) {
	assert := require.New(t)

	err := harvesterv1beta1.AddToScheme(scheme.Scheme)
	assert.NoError(err, "expected no error building scheme")

	typedObjects := []runtime.Object{}
	client := fake.NewSimpleClientset(typedObjects...)
	k8sclientset := k8sfake.NewSimpleClientset(testNode)
	fakeDynamicClient := fakedynamic.NewSimpleDynamicClient(scheme.Scheme)

	h := ActionHandler{
		nodeCache:     fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
		nodeClient:    fakeclients.NodeClient(k8sclientset.CoreV1().Nodes),
		addonCache:    fakeclients.AddonCache(client.HarvesterhciV1beta1().Addons),
		dynamicClient: fakeDynamicClient,
	}
	fakeHTTP := httptest.NewRecorder()
	err = h.powerActionPossible(fakeHTTP, testNode.Name)
	assert.NoError(err, "expected no error while quering powerActionPossible")
	assert.Equal(fakeHTTP.Result().StatusCode, http.StatusFailedDependency, "expected http NotFound")

}

func Test_powerActionPossible(t *testing.T) {
	assert := require.New(t)

	err := harvesterv1beta1.AddToScheme(scheme.Scheme)
	assert.NoError(err, "expected no error building scheme")

	typedObjects := []runtime.Object{seederAddon}
	client := fake.NewSimpleClientset(typedObjects...)
	k8sclientset := k8sfake.NewSimpleClientset(testNode)
	fakeDynamicClient := fakedynamic.NewSimpleDynamicClient(scheme.Scheme, dynamicInventoryObj)

	h := ActionHandler{
		nodeCache:     fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
		nodeClient:    fakeclients.NodeClient(k8sclientset.CoreV1().Nodes),
		addonCache:    fakeclients.AddonCache(client.HarvesterhciV1beta1().Addons),
		dynamicClient: fakeDynamicClient,
	}
	fakeHTTP := httptest.NewRecorder()
	err = h.powerActionPossible(fakeHTTP, testNode.Name)
	assert.NoError(err, "expected no error while querying powerActionPossible")
	assert.Equal(fakeHTTP.Result().StatusCode, http.StatusNoContent, "expected to find node")
}

func Test_powerAction(t *testing.T) {
	assert := require.New(t)

	powerOperation := "shutdown"
	k8sclientset := k8sfake.NewSimpleClientset(testNode)
	fakeDynamicClient := fakedynamic.NewSimpleDynamicClient(scheme.Scheme, dynamicInventoryObj)
	h := ActionHandler{
		nodeCache:     fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
		nodeClient:    fakeclients.NodeClient(k8sclientset.CoreV1().Nodes),
		dynamicClient: fakeDynamicClient,
	}

	err := h.powerAction(testNode, powerOperation)
	assert.NoError(err, "expected no error performing power action")
	iObj, err := h.fetchInventoryObject(testNode.Name)
	assert.NoError(err, "expected no error querying inventory object")
	powerRequest, ok, err := unstructured.NestedString(iObj.Object, "spec", "powerActionRequested")
	assert.NoError(err, "expected no error querying power status map")
	assert.True(ok, "expected to find power status map")
	assert.Equal(powerRequest, "shutdown", "expected to find power action shutdown")
}

func Test_invalidPowerAction(t *testing.T) {
	assert := require.New(t)

	powerOperation := "something"
	k8sclientset := k8sfake.NewSimpleClientset(testNode)

	h := ActionHandler{
		nodeCache:  fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
		nodeClient: fakeclients.NodeClient(k8sclientset.CoreV1().Nodes),
	}

	err := h.powerAction(testNode, powerOperation)
	assert.Error(err, "expected to get error")
}

func Test_listUnmigratableVM(t *testing.T) {
	assert := require.New(t)
	typedObjects := []runtime.Object{workingVM, vmWithContainerDisk, vmWithCDROM}
	client := fake.NewSimpleClientset(typedObjects...)
	k8sclientset := k8sfake.NewSimpleClientset(testNode, testNode2)

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
	resp := []ListUnhealthyVM{}
	err = json.NewDecoder(fakeHTTP.Body).Decode(&resp)
	assert.NoError(err, "expected no error parsing json response")
	assert.Len(resp[0].VMs, 2, "expected to find 2 non migratable VMs")
}

func Test_vmWithPCIDevices(t *testing.T) {
	assert := require.New(t)
	typedObjects := []runtime.Object{workingVM, vmWithPCIDevice}
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
	resp := []ListUnhealthyVM{}
	err = json.NewDecoder(fakeHTTP.Body).Decode(&resp)
	assert.NoError(err, "expected no error parsing json response")
	// return message is: [{"message":"The following VMs cannot be migrated due to NodeSchedulingRequirementsNotMet condition","vms":["default/healthy-vm","default/pcidevice-vm"]},{"message":"The following VMs cannot be migrated due to HostDeviceNotLiveMigratable condition","vms":["default/pcidevice-vm"]}]
	// note: the list sequence is random, as it was converted from a map
	for i := range resp {
		if strings.Contains(resp[i].Message, kubevirtv1.VirtualMachineInstanceReasonHostDeviceNotMigratable) {
			assert.Len(resp[i].VMs, 1, "expected to find 1 vms")
		}
		if strings.Contains(resp[i].Message, util.NodeSchedulingRequirementsNotMetKey) {
			assert.Len(resp[i].VMs, 2, "expected to find 2 vms")
		}
	}
}

func Test_vmMigrationPossible(t *testing.T) {

	var testCases = []struct {
		name                       string
		vmi                        *kubevirtv1.VirtualMachineInstance
		expectedNonMigratableCount int
	}{
		{
			name:                       "vm migration possible",
			vmi:                        vmWithSchedulingCriteriaMet,
			expectedNonMigratableCount: 0,
		},
		{
			name:                       "vm migration not possible",
			vmi:                        vmWithSchedulingCriteriaNotMet,
			expectedNonMigratableCount: 1,
		},
	}

	for _, test := range testCases {
		assert := require.New(t)
		typedObjects := []runtime.Object{workingVM, test.vmi}
		client := fake.NewSimpleClientset(typedObjects...)
		k8sclientset := k8sfake.NewSimpleClientset(testNode, testNode2, testNode3, testNode4)

		h := ActionHandler{
			nodeCache:                   fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
			nodeClient:                  fakeclients.NodeClient(k8sclientset.CoreV1().Nodes),
			longhornVolumeCache:         fakeclients.LonghornVolumeCache(client.LonghornV1beta2().Volumes),
			longhornReplicaCache:        fakeclients.LonghornReplicaCache(client.LonghornV1beta2().Replicas),
			virtualMachineInstanceCache: fakeclients.VirtualMachineInstanceCache(client.KubevirtV1().VirtualMachineInstances),
		}
		fakeHTTP := httptest.NewRecorder()
		err := h.listUnhealthyVM(fakeHTTP, testNode)
		assert.NoError(err, "expected no error while listing unhealthy VM's for test case: %s", test.name)
		resp := []ListUnhealthyVM{}
		err = json.NewDecoder(fakeHTTP.Body).Decode(&resp)
		assert.NoError(err, "expected no error parsing json response for test case: %s", test.name)
		if test.expectedNonMigratableCount > 0 {
			assert.Len(resp[0].VMs, test.expectedNonMigratableCount, "failed check for test case: %s", test.name)
		}
	}
}
