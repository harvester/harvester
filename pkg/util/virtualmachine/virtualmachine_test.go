package virtualmachine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_IsVMStopped(t *testing.T) {
	type input struct {
		vmi       *kubevirtv1.VirtualMachineInstance
		vm        *kubevirtv1.VirtualMachine
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
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Succeeded,
					},
				},
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						RunStrategy: runStrategyTransformerHelper(kubevirtv1.RunStrategyRerunOnFailure),
					},
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusStopped,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						RunStrategy: runStrategyTransformerHelper(kubevirtv1.RunStrategyHalted),
					},
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusStopped,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						RunStrategy: runStrategyTransformerHelper(kubevirtv1.RunStrategyRerunOnFailure),
					},
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusRunning,
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

func runStrategyTransformerHelper(input kubevirtv1.VirtualMachineRunStrategy) *kubevirtv1.VirtualMachineRunStrategy {
	temp := input
	return &temp
}
