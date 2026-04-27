package vm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"
	kubevirtv1 "kubevirt.io/api/core/v1"
	backendstorage "kubevirt.io/kubevirt/pkg/storage/backend-storage"
	kubevirtutil "kubevirt.io/kubevirt/pkg/virt-operator/util"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/migration"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

func TestMigrateAction(t *testing.T) {
	type input struct {
		namespace  string
		name       string
		nodeName   string
		vmInstance *kubevirtv1.VirtualMachineInstance
		kubeVirt   *kubevirtv1.KubeVirt
	}
	type output struct {
		vmInstanceMigrations []*kubevirtv1.VirtualMachineInstanceMigration
		err                  error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "VMI not found",
			given: input{
				namespace:  "default",
				name:       "test",
				vmInstance: nil,
				kubeVirt:   nil,
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  apierrors.NewNotFound(kubevirtv1.Resource("virtualmachineinstances"), "test"),
			},
		},
		{
			name: "VMI is not running",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Pending,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  errors.New("The VM is not in running state"),
			},
		},
		{
			name: "VMI's ready status is false",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
							{
								Type:   kubevirtv1.VirtualMachineInstanceReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				kubeVirt: nil,
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  errors.New("Can't migrate the VM, the VM is not in ready status"),
			},
		},
		{
			name: "VMI's ready status is unknown",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
							{
								Type:   kubevirtv1.VirtualMachineInstanceReady,
								Status: corev1.ConditionUnknown,
							},
						},
					},
				},
				kubeVirt: nil,
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  errors.New("Can't migrate the VM, the VM is not in ready status"),
			},
		},
		{
			name: "kubevirt not found",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
							{
								Type:   kubevirtv1.VirtualMachineInstanceReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				kubeVirt: nil,
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  apierrors.NewNotFound(kubevirtv1.Resource("kubevirts"), "kubevirt"),
			},
		},
		{
			name: "kubevirt condition not found",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
							{
								Type:   kubevirtv1.VirtualMachineInstanceReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				kubeVirt: &kubevirtv1.KubeVirt{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "harvester-system",
						Name:      "kubevirt",
					},
					Status: kubevirtv1.KubeVirtStatus{},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  errors.New("KubeVirt is not ready"),
			},
		},
		{
			name: "Migration is triggered",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
							{
								Type:   kubevirtv1.VirtualMachineInstanceReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				kubeVirt: &kubevirtv1.KubeVirt{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "harvester-system",
						Name:      "kubevirt",
					},
					Status: kubevirtv1.KubeVirtStatus{
						Conditions: []kubevirtv1.KubeVirtCondition{
							{
								Type:   kubevirtv1.KubeVirtConditionAvailable,
								Status: corev1.ConditionTrue,
								Reason: kubevirtutil.ConditionReasonDeploymentReady,
							},
						},
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:    "default",
							GenerateName: "test-",
						},
						Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
							VMIName: "test",
						},
					},
				},
				err: nil,
			},
		},
	}

	fakeNodeList := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "fake-node1",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "fake-node2",
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.vmInstance != nil {
			err := clientset.Tracker().Add(tc.given.vmInstance)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		if tc.given.kubeVirt != nil {
			err := clientset.Tracker().Add(tc.given.kubeVirt)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		for _, node := range fakeNodeList {
			err := clientset.Tracker().Add(node)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			nodeCache:     fakeclients.NodeCache(clientset.CoreV1().Nodes),
			kubevirtCache: fakeclients.KubeVirtCache(clientset.KubevirtV1().KubeVirts),
			vmiClient:     fakeclients.VirtualMachineInstanceClient(clientset.KubevirtV1().VirtualMachineInstances),
			vmiCache:      fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
			vmimClient:    fakeclients.VirtualMachineInstanceMigrationClient(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
			vmimCache:     fakeclients.VirtualMachineInstanceMigrationCache(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
		}

		var actual output
		var err error
		actual.err = handler.migrate(context.Background(), tc.given.namespace, tc.given.name, tc.given.nodeName)
		actual.vmInstanceMigrations, err = handler.vmimCache.List(tc.given.namespace, labels.Everything())
		assert.Nil(t, err, "List should return no error")

		assert.Equal(t, tc.expected.vmInstanceMigrations, actual.vmInstanceMigrations, "case %q", tc.name)
		if tc.expected.err != nil && actual.err != nil {
			//errors from pkg/errors track stacks so we only compare the error string here
			assert.Equal(t, tc.expected.err.Error(), actual.err.Error(), "case %q", tc.name)
		} else {
			assert.Equal(t, tc.expected.err, actual.err, "case %q", tc.name)
		}
	}

}

func TestAbortMigrateAction(t *testing.T) {
	type input struct {
		namespace           string
		name                string
		vmInstance          *kubevirtv1.VirtualMachineInstance
		vmInstanceMigration *kubevirtv1.VirtualMachineInstanceMigration
	}
	type output struct {
		vmInstanceMigrations []*kubevirtv1.VirtualMachineInstanceMigration
		err                  error
	}
	createVMIMigration := func(phase kubevirtv1.VirtualMachineInstanceMigrationPhase) *kubevirtv1.VirtualMachineInstanceMigration {
		return &kubevirtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test-migration",
				UID:       "test-uid",
			},
			Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
				VMIName: "test",
			},
			Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
				Phase: phase,
			},
		}
	}
	createVMI := func(annotations map[string]string) *kubevirtv1.VirtualMachineInstance {
		return &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "default",
				Name:        "test",
				Annotations: annotations,
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: kubevirtv1.Running,
				MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
					Completed:    false,
					MigrationUID: "test-uid",
				},
			},
		}
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "VMI migration is not started",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase:          kubevirtv1.Pending,
						MigrationState: nil,
					},
				},
				vmInstanceMigration: nil,
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  errors.New("The VM is not in migrating state"),
			},
		},
		{
			name: "VMI migration is completed",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Pending,
						MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
							Completed: true,
						},
					},
				},
				vmInstanceMigration: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-migration",
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
						VMIName: "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationSucceeded,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-migration",
						},
						Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
							VMIName: "test",
						},
						Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
							Phase: kubevirtv1.MigrationSucceeded,
						},
					},
				},
				err: errors.New("The VM is not in migrating state"),
			},
		},
		{
			name: "Abort VMI migration successfully",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: createVMI(map[string]string{
					util.AnnotationMigrationTarget: "test-uid",
					util.AnnotationMigrationState:  migration.StateMigrating,
				}),
				vmInstanceMigration: createVMIMigration(kubevirtv1.MigrationRunning),
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  nil,
			},
		},
		{
			name: "Abort VMI migration successfully in pending state",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationMigrationTarget: "test-uid",
							util.AnnotationMigrationState:  migration.StatePending,
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
							Completed:    false,
							MigrationUID: "test-uid",
						},
					},
				},
				vmInstanceMigration: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-migration",
						UID:       "test-uid",
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
						VMIName: "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationPending,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  nil,
			},
		},
		{
			name: "Cannot abort VMI migration in failed state",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: createVMI(map[string]string{
					util.AnnotationMigrationTarget: "test-uid",
					util.AnnotationMigrationState:  migration.StateMigrating,
				}),
				vmInstanceMigration: createVMIMigration(kubevirtv1.MigrationFailed),
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{
					createVMIMigration(kubevirtv1.MigrationFailed),
				},
				err: errors.New("cannot abort the migration as it is in \"Failed\" phase"),
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.vmInstance != nil {
			err := clientset.Tracker().Add(tc.given.vmInstance)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		if tc.given.vmInstanceMigration != nil {
			err := clientset.Tracker().Add(tc.given.vmInstanceMigration)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			vmiClient:  fakeclients.VirtualMachineInstanceClient(clientset.KubevirtV1().VirtualMachineInstances),
			vmiCache:   fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
			vmimClient: fakeclients.VirtualMachineInstanceMigrationClient(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
			vmimCache:  fakeclients.VirtualMachineInstanceMigrationCache(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
		}

		var actual output
		var err error
		actual.err = handler.abortMigration(tc.given.namespace, tc.given.name)
		actual.vmInstanceMigrations, err = handler.vmimCache.List(tc.given.namespace, labels.Everything())
		assert.Nil(t, err, "List should return no error")

		assert.Equal(t, tc.expected.vmInstanceMigrations, actual.vmInstanceMigrations, "case %q", tc.name)
		if tc.expected.err != nil && actual.err != nil {
			//errors from pkg/errors track stacks so we only compare the error string here
			assert.Equal(t, tc.expected.err.Error(), actual.err.Error(), "case %q", tc.name)
		} else {
			assert.Equal(t, tc.expected.err, actual.err, "case %q", tc.name)
		}
	}

}

func TestAddVolume(t *testing.T) {
	type input struct {
		namespace string
		name      string
		input     AddVolumeInput
		pvc       *corev1.PersistentVolumeClaim
	}
	type output struct {
		err error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "Volume source not found",
			given: input{
				namespace: "default",
				name:      "test",
				input: AddVolumeInput{
					DiskName:         "disk",
					VolumeSourceName: "not-exist",
				},
			},
			expected: output{
				err: errors.New("persistentvolumeclaims \"not-exist\" not found"),
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.pvc != nil {
			err := clientset.Tracker().Add(tc.given.pvc)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			pvcCache: fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
		}

		var actual output
		actual.err = handler.addVolume(context.Background(), tc.given.namespace, tc.given.namespace, tc.given.input)

		if tc.expected.err != nil && actual.err != nil {
			//errors from pkg/errors track stacks so we only compare the error string here
			assert.Equal(t, tc.expected.err.Error(), actual.err.Error(), "case %q", tc.name)
		} else {
			assert.Equal(t, tc.expected.err, actual.err, "case %q", tc.name)
		}
	}
}

func TestRemoveVolume(t *testing.T) {
	type input struct {
		namespace string
		name      string
		input     RemoveVolumeInput
		vm        *kubevirtv1.VirtualMachine
	}
	type output struct {
		err error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "VM instance not found",
			given: input{
				namespace: "default",
				name:      "test",
				input: RemoveVolumeInput{
					DiskName: "test",
				},
			},
			expected: output{
				err: errors.New("virtualmachines.kubevirt.io \"test\" not found"),
			},
		},
		{
			name: "Hotplug disk not found",
			given: input{
				namespace: "default",
				name:      "test",
				input: RemoveVolumeInput{
					DiskName: "not-exist",
				},
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: nil,
							},
						},
					},
				},
			},
			expected: output{
				err: errors.New("disk `not-exist` not found in virtual machine `default/test`"),
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.vm != nil {
			err := clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			vmCache:  fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
			pvcCache: fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
		}

		var actual output
		actual.err = handler.removeVolume(context.Background(), tc.given.namespace, tc.given.name, tc.given.input)

		if tc.expected.err != nil && actual.err != nil {
			//errors from pkg/errors track stacks so we only compare the error string here
			assert.Equal(t, tc.expected.err.Error(), actual.err.Error(), "case %q", tc.name)
		} else {
			assert.Equal(t, tc.expected.err, actual.err, "case %q", tc.name)
		}
	}
}

func Test_vmActionHandler_findMigratableNodesByVMI(t *testing.T) {
	type args struct {
		vmi  *kubevirtv1.VirtualMachineInstance
		pods []*corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want []string
		err  error
	}{
		{
			name: "Get migratable nodes by network selector from pod",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-network",
						Namespace: "default",
						UID:       "vmi-network-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
						ActivePods: map[types.UID]string{
							"pod-network-uid": "node1",
						},
					},
				},
				pods: []*corev1.Pod{{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virt-launcher-test-network",
						Namespace: "default",
						UID:       "pod-network-uid",
						Labels: map[string]string{
							kubevirtv1.CreatedByLabel: "vmi-network-uid",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
						NodeSelector: map[string]string{
							"network":                  "a",
							kubevirtv1.NodeSchedulable: "true",
						},
					},
				}},
			},
			want: []string{
				"node2",
			},
		},
		{
			name: "Get migratable nodes by network and zone selectors from pod",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-zone",
						Namespace: "default",
						UID:       "vmi-zone-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
						ActivePods: map[types.UID]string{
							"pod-zone-uid": "node1",
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "virt-launcher-test-zone",
							Namespace: "default",
							UID:       "pod-zone-uid",
							Labels: map[string]string{
								kubevirtv1.CreatedByLabel: "vmi-zone-uid",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node1",
							NodeSelector: map[string]string{
								"network":                  "a",
								"zone":                     "zone2",
								kubevirtv1.NodeSchedulable: "true",
							},
						},
					},
				},
			},
			want: []string{
				"node2",
			},
		},
		{
			name: "User defined custom selector from pod",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-custom",
						Namespace: "default",
						UID:       "vmi-custom-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
						ActivePods: map[types.UID]string{
							"pod-custom-uid": "node1",
						},
					},
				},
				pods: []*corev1.Pod{{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virt-launcher-test-custom",
						Namespace: "default",
						UID:       "pod-custom-uid",
						Labels: map[string]string{
							kubevirtv1.CreatedByLabel: "vmi-custom-uid",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
						NodeSelector: map[string]string{
							"user.custom/label":        "a",
							kubevirtv1.NodeSchedulable: "true",
						},
					},
				}},
			},
			want: []string{
				"node3",
			},
		},
		{
			name: "Get migratable nodes by CPU model and features from pod",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cpu",
						Namespace: "default",
						UID:       "vmi-cpu-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node4",
						ActivePods: map[types.UID]string{
							"pod-cpu-uid": "node4",
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "virt-launcher-test-cpu",
							Namespace: "default",
							UID:       "pod-cpu-uid",
							Labels: map[string]string{
								kubevirtv1.CreatedByLabel: "vmi-cpu-uid",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node4",
							NodeSelector: map[string]string{
								kubevirtv1.CPUModelLabel + "EPYC-Rome": "true",
								kubevirtv1.CPUFeatureLabel + "xsaves":  "true",
								kubevirtv1.NodeSchedulable:             "true",
							},
						},
					},
				},
			},
			want: []string{},
		},
		{
			name: "No pod found returns error when no active pods",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-no-pod",
						Namespace: "default",
						UID:       "vmi-no-pod-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
				pods: nil,
			},
			want: nil,
			err:  errors.New("there are no active pods for VMI: default/test-no-pod, migration target cannot be picked unless only 1 pod is active"),
		},
		{
			name: "Multiple active pods returns error during migration",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-multi-pod",
						Namespace: "default",
						UID:       "vmi-multi-pod-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "virt-launcher-test-multi-pod-1",
							Namespace: "default",
							UID:       "pod-multi-1-uid",
							Labels: map[string]string{
								kubevirtv1.CreatedByLabel: "vmi-multi-pod-uid",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "virt-launcher-test-multi-pod-2",
							Namespace: "default",
							UID:       "pod-multi-2-uid",
							Labels: map[string]string{
								kubevirtv1.CreatedByLabel: "vmi-multi-pod-uid",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node2",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
				},
			},
			want: nil,
			err:  errors.New("there are multiple active pods for VMI: default/test-multi-pod, migration target can be picked only when at most 1 pod is active"),
		},
		{
			name: "Hostname label should be skipped from pod node selector",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hostname-skip",
						Namespace: "default",
						UID:       "vmi-hostname-skip-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
						ActivePods: map[types.UID]string{
							"pod-hostname-skip-uid": "node1",
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "virt-launcher-test-hostname-skip",
							Namespace: "default",
							UID:       "pod-hostname-skip-uid",
							Labels: map[string]string{
								kubevirtv1.CreatedByLabel: "vmi-hostname-skip-uid",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node1",
							NodeSelector: map[string]string{
								corev1.LabelHostname:       "node1",
								"network":                  "a",
								kubevirtv1.NodeSchedulable: "true",
							},
						},
					},
				},
			},
			want: []string{
				"node2",
			},
		},
		{
			name: "Get migratable nodes by NodeAffinity with zone requirement",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-affinity-zone",
						Namespace: "default",
						UID:       "vmi-affinity-zone-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
						ActivePods: map[types.UID]string{
							"pod-affinity-zone-uid": "node1",
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "virt-launcher-test-affinity-zone",
							Namespace: "default",
							UID:       "pod-affinity-zone-uid",
							Labels: map[string]string{
								kubevirtv1.CreatedByLabel: "vmi-affinity-zone-uid",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node1",
							NodeSelector: map[string]string{
								kubevirtv1.NodeSchedulable: "true",
							},
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "zone",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"zone2"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{
				"node2",
			},
		},
		{
			name: "Get migratable nodes by NodeAffinity with NotIn operator",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-affinity-notin",
						Namespace: "default",
						UID:       "vmi-affinity-notin-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
						ActivePods: map[types.UID]string{
							"pod-affinity-notin-uid": "node1",
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "virt-launcher-test-affinity-notin",
							Namespace: "default",
							UID:       "pod-affinity-notin-uid",
							Labels: map[string]string{
								kubevirtv1.CreatedByLabel: "vmi-affinity-notin-uid",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node1",
							NodeSelector: map[string]string{
								kubevirtv1.NodeSchedulable: "true",
							},
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "zone",
														Operator: corev1.NodeSelectorOpNotIn,
														Values:   []string{"zone1"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{
				"node2", "node3", "node4", "node5",
			},
		},
		{
			name: "Get migratable nodes combining NodeSelector and NodeAffinity",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-combined",
						Namespace: "default",
						UID:       "vmi-combined-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
						ActivePods: map[types.UID]string{
							"pod-combined-uid": "node1",
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "virt-launcher-test-combined",
							Namespace: "default",
							UID:       "pod-combined-uid",
							Labels: map[string]string{
								kubevirtv1.CreatedByLabel: "vmi-combined-uid",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node1",
							NodeSelector: map[string]string{
								"network":                  "a",
								kubevirtv1.NodeSchedulable: "true",
							},
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "user.custom/label",
														Operator: corev1.NodeSelectorOpExists,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{},
		},
		{
			name: "Get migratable nodes with host-model CPU features from pod",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-host-model",
						Namespace: "default",
						UID:       "vmi-host-model-uid",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node4",
						ActivePods: map[types.UID]string{
							"pod-host-model-uid": "node4",
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "virt-launcher-test-host-model",
							Namespace: "default",
							UID:       "pod-host-model-uid",
							Labels: map[string]string{
								kubevirtv1.CreatedByLabel: "vmi-host-model-uid",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node4",
							NodeSelector: map[string]string{
								kubevirtv1.CPUFeatureLabel + "spec-ctrl": "true",
								kubevirtv1.CPUFeatureLabel + "ssbd":      "true",
								kubevirtv1.CPUModelLabel + "EPYC-Rome":   "true",
								kubevirtv1.NodeSchedulable:               "true",
							},
						},
					},
				},
			},
			want: []string{
				"node5",
			},
		},
	}

	fakeNodeList := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
				Labels: map[string]string{
					"user.custom/label":        "a",
					"network":                  "a",
					"zone":                     "zone1",
					kubevirtv1.NodeSchedulable: "true",
					kubevirtv1.CPUModelLabel + "EPYC-Rome-default": "true",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
				Labels: map[string]string{
					"network":                  "a",
					"zone":                     "zone2",
					kubevirtv1.NodeSchedulable: "true",
					kubevirtv1.CPUModelLabel + "EPYC-Rome-default": "true",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node3",
				Labels: map[string]string{
					"user.custom/label":        "a",
					"network":                  "b",
					"zone":                     "zone3",
					kubevirtv1.NodeSchedulable: "true",
					kubevirtv1.CPUModelLabel + "EPYC-Rome-default": "true",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node4",
				Labels: map[string]string{
					kubevirtv1.NodeSchedulable:             "true",
					kubevirtv1.CPUModelLabel + "EPYC-Rome": "true",
					kubevirtv1.CPUFeatureLabel + "xsaves":  "true",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node5",
				Labels: map[string]string{
					kubevirtv1.NodeSchedulable:               "true",
					kubevirtv1.CPUModelLabel + "EPYC-Rome":   "true",
					kubevirtv1.CPUFeatureLabel + "spec-ctrl": "true",
					kubevirtv1.CPUFeatureLabel + "ssbd":      "true",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "unschedulable-node",
				Labels: map[string]string{
					"network": "a",
					"zone":    "zone2",
				},
			},
			Spec: corev1.NodeSpec{
				Unschedulable: true,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "unschedulable-node2",
				Labels: map[string]string{
					"user.custom/label": "a",
					"network":           "a",
					"zone":              "zone2",
				},
			},
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{Key: corev1.TaintNodeUnschedulable},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "unreachable-node",
				Labels: map[string]string{
					"user.custom/label": "a",
					"network":           "a",
					"zone":              "zone2",
				},
			},
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{Key: corev1.TaintNodeUnreachable},
				},
			},
		},
	}

	var clientset = fake.NewSimpleClientset()
	for _, node := range fakeNodeList {
		err := clientset.Tracker().Add(node)
		assert.Nil(t, err, "Mock resource should add into fake controller tracker")
	}
	var nodeCache = fakeclients.NodeCache(clientset.CoreV1().Nodes)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh clientset for each test to avoid pod interference
			clientset := fake.NewSimpleClientset()
			for _, pod := range tt.args.pods {
				err := clientset.Tracker().Add(pod)
				assert.Nil(t, err, "Mock pod should add into fake controller tracker")
			}

			var podCache = fakeclients.PodCache(clientset.CoreV1().Pods)

			h := &vmActionHandler{
				nodeCache: nodeCache,
				podCache:  podCache,
			}
			got, err := h.findMigratableNodesByVMI(tt.args.vmi)
			if tt.err != nil && err != nil {
				assert.Equal(t, tt.err.Error(), err.Error(), "case %q", tt.name)
			}
			assert.Equalf(t, tt.want, got, "findMigratableNodesByVMI(%v)", tt.args.vmi)
		})
	}
}

func Test_isVmNetworkHotpluggable(t *testing.T) {
	type input struct {
		nad *cniv1.NetworkAttachmentDefinition
	}
	type output struct {
		isHotpluggable bool
	}
	testCases := []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "L2VlanNetwork is hot-pluggable",
			given: input{
				nad: &cniv1.NetworkAttachmentDefinition{
					Spec: cniv1.NetworkAttachmentDefinitionSpec{
						Config: `{"cniVersion":"0.3.1","name":"vlan1","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":1,"ipam":{}}`,
					},
				},
			},
			expected: output{
				isHotpluggable: true,
			},
		},
		{
			name: "L2VlanTrunkNetwork is hot-pluggable",
			given: input{
				nad: &cniv1.NetworkAttachmentDefinition{
					Spec: cniv1.NetworkAttachmentDefinitionSpec{
						Config: `{"cniVersion":"0.3.1","name":"vlan1-10","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":0,"ipam":{},"vlanTrunk":[{"minID":1,"maxID":10}]}`,
					},
				},
			},
			expected: output{
				isHotpluggable: true,
			},
		},
		{
			name: "UntaggedNetwork is hot-pluggable",
			given: input{
				nad: &cniv1.NetworkAttachmentDefinition{
					Spec: cniv1.NetworkAttachmentDefinitionSpec{
						Config: `{"cniVersion":"0.3.1","name":"untagged","type":"bridge","bridge":"mgmt-br","promiscMode":true,"ipam":{}}`,
					},
				},
			},
			expected: output{
				isHotpluggable: true,
			},
		},
		{
			name: "OverlayNetwork isn't hot-pluggable",
			given: input{
				nad: &cniv1.NetworkAttachmentDefinition{
					Spec: cniv1.NetworkAttachmentDefinitionSpec{
						Config: `{"cniVersion":"0.3.1","name":"overlay","type":"kube-ovn","provider":"overlay.default.ovn","server_socket":"/run/openvswitch/kube-ovn-daemon.sock"}`,
					},
				},
			},
			expected: output{
				isHotpluggable: false,
			},
		},
		{
			name: "migration network isn't hot-pluggable, although it's already filtered out from FE",
			given: input{
				nad: &cniv1.NetworkAttachmentDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "vm-migration-network-rqmmg",
						Namespace: "harvester-system",
						Annotations: map[string]string{
							"vm-migration-network.settings.harvesterhci.io": "true",
						},
						Labels: map[string]string{
							"network.harvesterhci.io/clusternetwork":             "migration",
							"network.harvesterhci.io/ready":                      "true",
							"network.harvesterhci.io/type":                       "L2VlanNetwork",
							"network.harvesterhci.io/vlan-id":                    "1",
							"vm-migration-network.settings.harvesterhci.io/hash": "194c07126764f0ebcd996eee3c7c4c0735fa3145",
						},
					},
					Spec: cniv1.NetworkAttachmentDefinitionSpec{
						Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"migration-br","promiscMode":true,"vlan":1,"ipam":{"type":"whereabouts","range":"172.16.0.0/24"}}`,
					},
				},
			},
			expected: output{
				isHotpluggable: false,
			},
		},
		{
			name: "storage network isn't hot-pluggable, although it's already filtered out from FE",
			given: input{
				nad: &cniv1.NetworkAttachmentDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "storagenetwork-bz9vj",
						Namespace: "harvester-system",
						Annotations: map[string]string{
							"storage-network.settings.harvesterhci.io": "true",
						},
						Labels: map[string]string{
							"network.harvesterhci.io/clusternetwork":        "storage",
							"network.harvesterhci.io/ready":                 "true",
							"network.harvesterhci.io/type":                  "L2VlanNetwork",
							"network.harvesterhci.io/vlan-id":               "99",
							"storage-network.settings.harvesterhci.io/hash": "c8c41cfbdc1a85e7cbd0d3204b441fd13cc109e3",
						},
					},
					Spec: cniv1.NetworkAttachmentDefinitionSpec{
						Config: `{"cniVersion":"0.3.1","type":"bridge","bridge":"storage-br","promiscMode":true,"vlan":99,"ipam":{"type":"whereabouts","range":"10.0.99.0/24"}}`,
					},
				},
			},
			expected: output{
				isHotpluggable: false,
			},
		},
		{
			name: "deletion requested NAD is not hot-pluggable",
			given: input{
				nad: &cniv1.NetworkAttachmentDefinition{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
					Spec: cniv1.NetworkAttachmentDefinitionSpec{
						Config: `{"cniVersion":"0.3.1","name":"untagged","type":"bridge","bridge":"mgmt-br","promiscMode":true,"ipam":{}}`,
					},
				},
			},
			expected: output{
				isHotpluggable: false,
			},
		},
	}

	for _, tc := range testCases {
		result, err := isVmNetworkHotpluggable(tc.given.nad)
		assert.Nil(t, err, "case %q", tc.name)
		assert.Equal(t, tc.expected.isHotpluggable, result, tc.name)
	}

}

func TestInsertCdRomVolumeAction(t *testing.T) {
	vmName := "vm1"
	vmNamespace := "default"
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: vmNamespace,
			Annotations: map[string]string{
				util.AnnotationVolumeClaimTemplates: `[]`,
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							Disks: []kubevirtv1.Disk{
								{
									Name: "cdrom1",
									DiskDevice: kubevirtv1.DiskDevice{
										CDRom: &kubevirtv1.CDRomTarget{
											Bus: kubevirtv1.DiskBusSATA,
										},
									},
								},
							},
						},
					},
					Volumes: nil,
				},
			},
		},
	}

	vmImageName := "image-8jpqp"
	vmImageNamespace := "default"
	vmImage := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmImageName,
			Namespace: vmNamespace,
		},
		Status: harvesterv1.VirtualMachineImageStatus{
			VirtualSize:      25165824,
			StorageClassName: "longhorn-image-cn2w6",
		},
	}

	clientset := fake.NewSimpleClientset(vm, vmImage)

	handler := &vmActionHandler{
		vmClient:     fakeclients.VirtualMachineClient(clientset.KubevirtV1().VirtualMachines),
		vmCache:      fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		vmImageCache: fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
	}

	input := InsertCdRomVolumeActionInput{
		DeviceName: "cdrom1",
		ImageName:  vmImageNamespace + "/" + vmImageName,
	}

	err := handler.insertCdRomVolume(vmName, vmNamespace, input)
	assert.Nil(t, err, "insertCdRomVolume shouldn't return error")

	vmUpdated, err := handler.vmCache.Get(vmNamespace, vmName)
	assert.Nil(t, err, "Should get the updated VM")

	assert.Len(t, vmUpdated.Spec.Template.Spec.Volumes, 1, "Should have hot-plug new volume")

	volumeClaimTemplates := vmUpdated.ObjectMeta.Annotations[util.AnnotationVolumeClaimTemplates]
	entries, err := util.UnmarshalVolumeClaimTemplates(volumeClaimTemplates)
	assert.Nil(t, err, "Should unmarshal volumeClaimTemplates annotation correctly")
	assert.Len(t, entries, 1, "Should add to volumeClaimTemplates annotation")
}

func TestEjectCdRomVolumeAction(t *testing.T) {
	vmName := "vm1"
	vmNamespace := "default"

	pvcName := "vm1-cdrom1-52h6j"
	pvcNamespace := "default"
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: vmNamespace,
			Annotations: map[string]string{
				util.AnnotationVolumeClaimTemplates: fmt.Sprintf(`[{"metadata":{"name":"%s","creationTimestamp":null,"annotations":{"harvesterhci.io/imageId":"default/image-8jpqp"}},"spec":{"accessModes":["ReadWriteMany"],"resources":{"requests":{"storage":"24Mi"}},"storageClassName":"longhorn-image-cn2w6","volumeMode":"Block"},"status":{}}]`, pvcName),
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							Disks: []kubevirtv1.Disk{
								{
									Name: "cdrom1",
									DiskDevice: kubevirtv1.DiskDevice{
										CDRom: &kubevirtv1.CDRomTarget{
											Bus: kubevirtv1.DiskBusSATA,
										},
									},
								},
							},
						},
					},
					Volumes: []kubevirtv1.Volume{
						{
							Name: "cdrom1",
							VolumeSource: kubevirtv1.VolumeSource{
								PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: pvcName,
									},
									Hotpluggable: true,
								},
							},
						},
					},
				},
			},
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: pvcNamespace,
		},
	}

	clientset := fake.NewSimpleClientset(vm, pvc)

	handler := &vmActionHandler{
		vmClient:  fakeclients.VirtualMachineClient(clientset.KubevirtV1().VirtualMachines),
		vmCache:   fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		pvcClient: fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims),
		pvcCache:  fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
	}

	input := EjectCdRomVolumeActionInput{
		DeviceName: "cdrom1",
	}

	pvcCache := handler.pvcCache
	_, err1 := pvcCache.Get(pvcNamespace, pvcName)
	assert.Nil(t, err1, "Should find pvc initially")

	err := handler.ejectCdRomVolume(context.Background(), vmName, vmNamespace, input)
	assert.Nil(t, err, "ejectCdRomVolume shouldn't return error")

	vmUpdated, err := handler.vmCache.Get(vmNamespace, vmName)
	assert.Nil(t, err, "Should get the updated VM")

	assert.Len(t, vmUpdated.Spec.Template.Spec.Volumes, 0, "Should have hot-unplug volume")

	volumeClaimTemplates := vmUpdated.ObjectMeta.Annotations[util.AnnotationVolumeClaimTemplates]
	entries, err := util.UnmarshalVolumeClaimTemplates(volumeClaimTemplates)
	assert.Nil(t, err, "Should unmarshal volumeClaimTemplates annotation correctly")
	assert.Len(t, entries, 0, "Should remove from volumeClaimTemplates annotation")

	_, err = pvcCache.Get(pvcNamespace, pvcName)
	assert.True(t, apierrors.IsNotFound(err), "Should delete pvc")
}

func TestBuildEFICopyJob(t *testing.T) {
	h := &vmActionHandler{}
	targetVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "target-vm",
			Namespace: "default",
			UID:       "test-uid",
		},
	}
	sourcePVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "persistent-state-for-source-vm-abc123",
			Namespace: "default",
		},
	}
	targetPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "persistent-state-for-target-vm-def456",
			Namespace: "default",
		},
	}
	script := "test-script"

	job := h.buildEFICopyJob(targetVM, sourcePVC, targetPVC, script)

	assert.Contains(t, job.Name, "efi-copy-target-vm-")
	assert.Equal(t, "default", job.Namespace)
	assert.Len(t, job.OwnerReferences, 1)
	assert.Equal(t, "target-vm", job.OwnerReferences[0].Name)
	assert.Equal(t, types.UID("test-uid"), job.OwnerReferences[0].UID)
	assert.Equal(t, int32(efiRenameJobBackoffLimit), *job.Spec.BackoffLimit)
	assert.Equal(t, int32(efiRenameJobTTL), *job.Spec.TTLSecondsAfterFinished)
	assert.Equal(t, corev1.RestartPolicyNever, job.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, int64(0), *job.Spec.Template.Spec.SecurityContext.RunAsUser)
	assert.Equal(t, int64(0), *job.Spec.Template.Spec.SecurityContext.RunAsGroup)

	assert.Len(t, job.Spec.Template.Spec.Containers, 1)
	c := job.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "efi-copy", c.Name)
	assert.Equal(t, efiRenameJobImage, c.Image)
	assert.Equal(t, []string{"sh", "-c", script}, c.Command)
	assert.Len(t, c.VolumeMounts, 2)
	assert.Equal(t, "src-pvc", c.VolumeMounts[0].Name)
	assert.Equal(t, "/src", c.VolumeMounts[0].MountPath)
	assert.Equal(t, "dst-pvc", c.VolumeMounts[1].Name)
	assert.Equal(t, "/dst", c.VolumeMounts[1].MountPath)

	assert.Len(t, job.Spec.Template.Spec.Volumes, 2)
	assert.Equal(t, sourcePVC.Name, job.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName)
	assert.Equal(t, targetPVC.Name, job.Spec.Template.Spec.Volumes[1].VolumeSource.PersistentVolumeClaim.ClaimName)
}

func TestCopyEFIFile(t *testing.T) {
	sourceVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "source-vm", Namespace: "default"},
	}
	targetVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "target-vm", Namespace: "default", UID: "uid-1"},
	}
	sourcePVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "persistent-state-for-source-vm-xyz", Namespace: "default"},
	}
	targetPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "persistent-state-for-target-vm-xyz", Namespace: "default"},
	}

	t.Run("create job fails", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("mock create job error")
		})

		h := &vmActionHandler{
			jobClient: fakeclients.JobClient(clientset.BatchV1().Jobs),
		}

		err := h.copyEFIFile(context.Background(), sourceVM, targetVM, sourcePVC, targetPVC)
		assert.EqualError(t, err, "create job: mock create job error")
	})

	t.Run("success", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
			ga := action.(k8stesting.GetAction)
			return true, &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ga.GetName(),
					Namespace: ga.GetNamespace(),
				},
				Status: batchv1.JobStatus{Succeeded: 1},
			}, nil
		})

		h := &vmActionHandler{
			jobClient: fakeclients.JobClient(clientset.BatchV1().Jobs),
		}

		err := h.copyEFIFile(context.Background(), sourceVM, targetVM, sourcePVC, targetPVC)
		assert.NoError(t, err)

		jobList, err := clientset.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, jobList.Items, 1)

		createdJob := jobList.Items[0]
		assert.Contains(t, createdJob.Name, "efi-copy-target-vm-")
		assert.Equal(t, []string{"sh", "-c", `set -e
src="/src/nvram/source-vm_VARS.fd"
dst="/dst/nvram/target-vm_VARS.fd"
[ -f "$src" ] && cp "$src" "$dst"
[ -f "$dst" ] && chown 107:107 "$dst"
[ -f "$dst" ] && chmod 600 "$dst"`}, createdJob.Spec.Template.Spec.Containers[0].Command)
		assert.Len(t, createdJob.Spec.Template.Spec.Volumes, 2)
		assert.Equal(t, sourcePVC.Name, createdJob.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName)
		assert.Equal(t, targetPVC.Name, createdJob.Spec.Template.Spec.Volumes[1].VolumeSource.PersistentVolumeClaim.ClaimName)
	})
}

func TestWaitForPVCBound(t *testing.T) {
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "source-vm", Namespace: "default"},
	}
	boundPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "persistent-state-for-source-vm-abc",
			Namespace: "default",
			Labels:    map[string]string{backendstorage.PVCPrefix: "source-vm"},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	secondPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "persistent-state-for-source-vm-def",
			Namespace: "default",
			Labels:    map[string]string{backendstorage.PVCPrefix: "source-vm"},
		},
	}

	tests := []struct {
		name             string
		objects          []runtime.Object
		setupReactors    func(*fake.Clientset)
		expectedPVCName  string
		expectedErrorMsg string
	}{
		{
			name:            "no pvc returns nil",
			expectedPVCName: "",
		},
		{
			name:             "list fails",
			expectedErrorMsg: "cannot list source persistent state PVC for VM default/source-vm, err: mock list error",
			setupReactors: func(clientset *fake.Clientset) {
				clientset.PrependReactor("list", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("mock list error")
				})
			},
		},
		{
			name:             "multiple pvcs fail",
			objects:          []runtime.Object{boundPVC, secondPVC},
			expectedErrorMsg: "expect 1 source persistent state PVC for VM default/source-vm, got 2",
		},
		{
			name:            "bound pvc succeeds",
			objects:         []runtime.Object{boundPVC},
			expectedPVCName: boundPVC.Name,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.objects...)
			if tc.setupReactors != nil {
				tc.setupReactors(clientset)
			}

			h := &vmActionHandler{
				pvcCache: fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
			}

			pvc, err := h.waitForPVCBound(context.Background(), vm)
			if tc.expectedErrorMsg != "" {
				assert.EqualError(t, err, tc.expectedErrorMsg)
				return
			}

			assert.NoError(t, err)
			if tc.expectedPVCName == "" {
				assert.Nil(t, pvc)
				return
			}

			assert.NotNil(t, pvc)
			assert.Equal(t, tc.expectedPVCName, pvc.Name)
		})
	}
}

func TestCopyEFIPersistent(t *testing.T) {
	sourceVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "source-vm", Namespace: "default"},
	}
	targetVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "target-vm", Namespace: "default", UID: "uid-1"},
	}
	sourcePVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "persistent-state-for-source-vm-abc",
			Namespace: "default",
			Labels:    map[string]string{backendstorage.PVCPrefix: "source-vm"},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	targetPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "persistent-state-for-target-vm-xyz",
			Namespace: "default",
			Labels:    map[string]string{backendstorage.PVCPrefix: "target-vm"},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}

	tests := []struct {
		name             string
		objects          []runtime.Object
		setupReactors    func(*fake.Clientset)
		expectedErrorMsg string
	}{
		{
			name:             "source pvc list fails",
			objects:          nil,
			expectedErrorMsg: "cannot wait for source persistent state PVC for VM default/source-vm: cannot list source persistent state PVC for VM default/source-vm, err: mock source list error",
			setupReactors: func(clientset *fake.Clientset) {
				clientset.PrependReactor("list", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
					listAction := action.(k8stesting.ListAction)
					if listAction.GetListRestrictions().Labels.String() == labels.SelectorFromSet(map[string]string{backendstorage.PVCPrefix: "source-vm"}).String() {
						return true, nil, errors.New("mock source list error")
					}
					return false, nil, nil
				})
			},
		},
		{
			name:             "target pvc list fails",
			objects:          []runtime.Object{sourcePVC},
			expectedErrorMsg: "cannot wait for target persistent state PVC for VM default/target-vm: cannot list source persistent state PVC for VM default/target-vm, err: mock target list error",
			setupReactors: func(clientset *fake.Clientset) {
				clientset.PrependReactor("list", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
					listAction := action.(k8stesting.ListAction)
					if listAction.GetListRestrictions().Labels.String() == labels.SelectorFromSet(map[string]string{backendstorage.PVCPrefix: "target-vm"}).String() {
						return true, nil, errors.New("mock target list error")
					}
					return false, nil, nil
				})
			},
		},
		{
			name:             "copy job creation fails",
			objects:          []runtime.Object{sourcePVC, targetPVC},
			expectedErrorMsg: "create job: mock create job error",
			setupReactors: func(clientset *fake.Clientset) {
				clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("mock create job error")
				})
			},
		},
		{
			name:    "success",
			objects: []runtime.Object{sourcePVC, targetPVC},
			setupReactors: func(clientset *fake.Clientset) {
				clientset.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					ga := action.(k8stesting.GetAction)
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ga.GetName(),
							Namespace: ga.GetNamespace(),
						},
						Status: batchv1.JobStatus{Succeeded: 1},
					}, nil
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.objects...)
			if tc.setupReactors != nil {
				tc.setupReactors(clientset)
			}

			h := &vmActionHandler{
				pvcCache:  fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				jobClient: fakeclients.JobClient(clientset.BatchV1().Jobs),
			}

			err := h.copyEFIPersistent(sourceVM, targetVM)
			if tc.expectedErrorMsg != "" {
				assert.EqualError(t, err, tc.expectedErrorMsg)
				return
			}

			assert.NoError(t, err)

			jobList, err := clientset.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
			assert.NoError(t, err)
			assert.Len(t, jobList.Items, 1)
			assert.Equal(t, sourcePVC.Name, jobList.Items[0].Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName)
			assert.Equal(t, targetPVC.Name, jobList.Items[0].Spec.Template.Spec.Volumes[1].VolumeSource.PersistentVolumeClaim.ClaimName)
		})
	}
}
