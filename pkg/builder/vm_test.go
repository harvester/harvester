package builder

import (
	"testing"
)

type testcase struct {
	name        string
	builder     *VMBuilder
	expectation bool
}

func TestDedicatedCPUPlacement(t *testing.T) {
	testcases := []testcase{
		{
			name:        "default value",
			builder:     NewVMBuilder("test"),
			expectation: false,
		},
		{
			name:        "dedicated cpu placement true",
			builder:     NewVMBuilder("test").DedicatedCPUPlacement(true),
			expectation: true,
		},
		{
			name:        "dedicated cpu placement false",
			builder:     NewVMBuilder("test").DedicatedCPUPlacement(false),
			expectation: false,
		},
	}

	for _, tc := range testcases {
		testVM, err := tc.builder.VM()

		if err != nil || testVM.Spec.Template.Spec.Domain.CPU.DedicatedCPUPlacement != tc.expectation {
			t.Error("generated VM object did not match expectations")
		}
	}
}

func TestIsolateEmulatorThread(t *testing.T) {
	testcases := []testcase{
		{
			name:        "default value",
			builder:     NewVMBuilder("test"),
			expectation: false,
		},
		{
			name:        "isolated emulator thread true",
			builder:     NewVMBuilder("test").IsolateEmulatorThread(true),
			expectation: true,
		},
		{
			name:        "isolated emulator thread false",
			builder:     NewVMBuilder("test").IsolateEmulatorThread(false),
			expectation: false,
		},
	}

	for _, tc := range testcases {
		testVM, err := tc.builder.VM()

		if err != nil || testVM.Spec.Template.Spec.Domain.CPU.IsolateEmulatorThread != tc.expectation {
			t.Error("generated VM object did not match expectations")
		}
	}
}
