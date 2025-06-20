package vm

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corefake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"
	kubevirtutil "kubevirt.io/kubevirt/pkg/virt-operator/util"

	"github.com/harvester/harvester/pkg/controller/master/migration"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
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
		var coreclientset = corefake.NewSimpleClientset()
		if tc.given.vmInstance != nil {
			err := clientset.Tracker().Add(tc.given.vmInstance)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		if tc.given.kubeVirt != nil {
			err := clientset.Tracker().Add(tc.given.kubeVirt)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		for _, node := range fakeNodeList {
			err := coreclientset.Tracker().Add(node)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			nodeCache:     fakeclients.NodeCache(coreclientset.CoreV1().Nodes),
			kubevirtCache: fakeclients.KubeVirtCache(clientset.KubevirtV1().KubeVirts),
			vmis:          fakeclients.VirtualMachineInstanceClient(clientset.KubevirtV1().VirtualMachineInstances),
			vmiCache:      fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
			vmims:         fakeclients.VirtualMachineInstanceMigrationClient(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
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
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationMigrationTarget: "test-uid",
							util.AnnotationMigrationState:  migration.StateMigrating,
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
						Phase: kubevirtv1.MigrationRunning,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  nil,
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
			vmis:      fakeclients.VirtualMachineInstanceClient(clientset.KubevirtV1().VirtualMachineInstances),
			vmiCache:  fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
			vmims:     fakeclients.VirtualMachineInstanceMigrationClient(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
			vmimCache: fakeclients.VirtualMachineInstanceMigrationCache(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
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
		var coreclientset = corefake.NewSimpleClientset()
		if tc.given.pvc != nil {
			err := coreclientset.Tracker().Add(tc.given.pvc)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			pvcCache: fakeclients.PersistentVolumeClaimCache(coreclientset.CoreV1().PersistentVolumeClaims),
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
		var coreclientset = corefake.NewSimpleClientset()
		if tc.given.vm != nil {
			err := clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			vmCache:  fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
			pvcCache: fakeclients.PersistentVolumeClaimCache(coreclientset.CoreV1().PersistentVolumeClaims),
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
		vmi *kubevirtv1.VirtualMachineInstance
	}
	tests := []struct {
		name string
		args args
		want []string
		err  error
	}{
		{
			name: "Get migratable nodes by network affinity",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "network",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"a"},
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
				"node1", "node2",
			},
		},
		{
			name: "Get migratable nodes by network affinity and zone",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "network",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"a"},
												},
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
			want: []string{
				"node2",
			},
		},
		{
			name: "User defined custom affinity",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "user.custom/label",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"a"},
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
				"node1", "node3",
			},
		},
	}

	fakeNodeList := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
				Labels: map[string]string{
					"user.custom/label": "a",
					"network":           "a",
					"zone":              "zone1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
				Labels: map[string]string{
					"network": "a",
					"zone":    "zone2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node3",
				Labels: map[string]string{
					"user.custom/label": "a",
					"network":           "b",
					"zone":              "zone3",
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
	var coreclientset = corefake.NewSimpleClientset()
	for _, node := range fakeNodeList {
		err := coreclientset.Tracker().Add(node)
		assert.Nil(t, err, "Mock resource should add into fake controller tracker")
	}
	var nodeCache = fakeclients.NodeCache(coreclientset.CoreV1().Nodes)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &vmActionHandler{
				nodeCache: nodeCache,
			}
			got, err := h.findMigratableNodesByVMI(tt.args.vmi)
			if tt.err != nil && err != nil {
				assert.Equal(t, tt.err.Error(), err.Error(), "case %q", tt.name)
			}
			assert.Equalf(t, tt.want, got, "findMigratableNodesByVMI(%v)", tt.args.vmi)
		})
	}
}
