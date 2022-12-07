package util

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	testFinalizer = "harvesterhci.io/test-finalizer"
)

func Test_AddFinalizer(t *testing.T) {
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Finalizers: []string{
				"some-finalizer",
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{},
	}

	AddFinalizer(vm, testFinalizer)
	assert := require.New(t)
	assert.True(ContainsFinalizer(vm, testFinalizer), "expected to find finalizer")
}

func Test_RemoveFinalizer(t *testing.T) {
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Finalizers: []string{
				"some-finalizer",
				testFinalizer,
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{},
	}

	RemoveFinalizer(vm, testFinalizer)
	assert := require.New(t)
	assert.False(ContainsFinalizer(vm, testFinalizer), "expected to find finalizer")
}
