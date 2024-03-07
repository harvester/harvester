package virtualmachine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corefake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1api "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_IsVMStopped(t *testing.T) {
	type input struct {
		vmi       *kubevirtv1api.VirtualMachineInstance
		vm        *kubevirtv1api.VirtualMachine
		namespace string
	}

	testCases := []struct {
		desc     string
		input    input
		expected func(isStopped bool, err error, desc string)
	}{
		{
			desc: "when vm is stopped inside vm case",
			input: input{
				namespace: "default",
				vmi: &kubevirtv1api.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1api.VirtualMachineInstanceStatus{
						Phase: kubevirtv1api.Succeeded,
					},
				},
				vm: &kubevirtv1api.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1api.VirtualMachineSpec{
						RunStrategy: runStrategyTransformerHelper(kubevirtv1api.RunStrategyRerunOnFailure),
					},
					Status: kubevirtv1api.VirtualMachineStatus{
						PrintableStatus: kubevirtv1api.VirtualMachineStatusStopped,
					},
				},
			},
			expected: func(isStopped bool, err error, desc string) {
				assert.Equal(t, true, isStopped, desc)
				assert.Equal(t, nil, err, desc)
			},
		},
		{
			desc: "when vm is stopped from GUI case",
			input: input{
				namespace: "default",
				vmi:       nil,
				vm: &kubevirtv1api.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1api.VirtualMachineSpec{
						RunStrategy: runStrategyTransformerHelper(kubevirtv1api.RunStrategyHalted),
					},
					Status: kubevirtv1api.VirtualMachineStatus{
						PrintableStatus: kubevirtv1api.VirtualMachineStatusStopped,
					},
				},
			},
			expected: func(isStopped bool, err error, desc string) {
				assert.Equal(t, true, isStopped, desc)
				assert.Equal(t, nil, err, desc)
			},
		},
		{
			desc: "when vm is running",
			input: input{
				namespace: "default",
				vmi:       nil,
				vm: &kubevirtv1api.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1api.VirtualMachineSpec{
						RunStrategy: runStrategyTransformerHelper(kubevirtv1api.RunStrategyRerunOnFailure),
					},
					Status: kubevirtv1api.VirtualMachineStatus{
						PrintableStatus: kubevirtv1api.VirtualMachineStatusRunning,
					},
				},
			},
			expected: func(isStopped bool, err error, desc string) {
				assert.Equal(t, false, isStopped, desc)
				assert.Equal(t, nil, err, desc)
			},
		},
	}

	for _, tc := range testCases {
		var (
			clientset     = fake.NewSimpleClientset()
			coreclientset = corefake.NewSimpleClientset()
		)

		if _, err := coreclientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: tc.input.namespace,
			},
		}, metav1.CreateOptions{}); err != nil {
			assert.Nil(t, err, "failed to create namespace", tc.desc)
		}

		if _, err := clientset.KubevirtV1().VirtualMachines(tc.input.namespace).Create(context.TODO(), tc.input.vm, metav1.CreateOptions{}); tc.input.vm != nil && err != nil {
			assert.Nil(t, err, "failed to create fake vm", tc.desc)
		}
		if _, err := clientset.KubevirtV1().VirtualMachineInstances(tc.input.namespace).Create(context.TODO(), tc.input.vmi, metav1.CreateOptions{}); tc.input.vmi != nil && err != nil {
			assert.Nil(t, err, "failed to create fake vmi", tc.desc)
		}

		vmiCache := fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances)
		isStopped, err := IsVMStopped(tc.input.vm, vmiCache)

		tc.expected(isStopped, err, tc.desc)
	}
}

func runStrategyTransformerHelper(input kubevirtv1api.VirtualMachineRunStrategy) *kubevirtv1api.VirtualMachineRunStrategy {
	temp := input
	return &temp
}

func Test_ListByInstanceLabels(t *testing.T) {
	vms := []*kubevirtv1api.VirtualMachine{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test01",
			},
			Spec: kubevirtv1api.VirtualMachineSpec{
				Template: &kubevirtv1api.VirtualMachineInstanceTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test02",
			},
			Spec: kubevirtv1api.VirtualMachineSpec{
				Template: &kubevirtv1api.VirtualMachineInstanceTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test03",
			},
			Spec: kubevirtv1api.VirtualMachineSpec{
				Template: &kubevirtv1api.VirtualMachineInstanceTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels.Set{
							"foo": "bar",
						},
					},
				},
			},
		},
	}

	var testCases = []struct {
		desc          string
		namespace     string
		selector      labels.Selector
		expectedCount int
	}{
		{
			desc:          "test01",
			namespace:     "default",
			selector:      labels.Everything(),
			expectedCount: 3,
		},
		{
			desc:          "test02",
			namespace:     "default",
			selector:      labels.Set{"foo": "bar"}.AsSelector(),
			expectedCount: 1,
		},
	}

	for _, tc := range testCases {
		clientSet := fake.NewSimpleClientset()
		coreClientSet := corefake.NewSimpleClientset()

		if _, err := coreClientSet.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: tc.namespace,
			},
		}, metav1.CreateOptions{}); err != nil {
			assert.Nil(t, err, "failed to create namespace", tc.desc)
		}

		for _, vm := range vms {
			if _, err := clientSet.KubevirtV1().VirtualMachines(tc.namespace).Create(context.TODO(), vm, metav1.CreateOptions{}); vm != nil && err != nil {
				assert.Nil(t, err, "failed to create fake vm", tc.desc, vm.Name)
			}
		}

		vmCache := fakeclients.VirtualMachineCache(clientSet.KubevirtV1().VirtualMachines)

		res, err := ListByInstanceLabels(tc.namespace, tc.selector, vmCache)
		assert.Len(t, res, tc.expectedCount)
		assert.NoError(t, err)
	}
}
