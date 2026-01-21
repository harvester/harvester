package builder

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCPUCores(t *testing.T) {
	type testcase struct {
		description string
		builder     *VMBuilder
		expectation uint32
		expectError bool
	}

	testcases := []testcase{
		{
			description: "default value",
			builder:     NewVMBuilder("test"),
			expectation: defaultVMCPUCores,
			expectError: false,
		},
		{
			description: "set cpu cores",
			builder:     NewVMBuilder("test").CPUCores(4),
			expectation: 4,
			expectError: false,
		},
		{
			description: "invalid value",
			builder:     NewVMBuilder("test").CPUCores(-2),
			expectation: 1,
			expectError: true,
		},
	}

	for _, tc := range testcases {
		testVM, err := tc.builder.VM()

		if tc.expectError && err == nil {
			t.Errorf("test %s: unexpected error", tc.description)
			continue
		}

		if !tc.expectError && err != nil {
			t.Errorf("test %s: expected error", tc.description)
			continue
		}

		if tc.expectError {
			continue
		}

		if testVM == nil || testVM.Spec.Template.Spec.Domain.CPU == nil {
			t.Errorf("test %s: unexpected nil value for domain cpu", tc.description)
			continue
		}

		if tc.expectation != testVM.Spec.Template.Spec.Domain.CPU.Cores {
			t.Errorf("test %s: unexpected cpu coresvalue: %d, expected: %d",
				tc.description,
				testVM.Spec.Template.Spec.Domain.CPU.Cores,
				tc.expectation,
			)
			continue
		}
	}
}

func TestCPUSockets(t *testing.T) {
	type testcase struct {
		description string
		builder     *VMBuilder
		expectation uint32
		expectError bool
	}

	testcases := []testcase{
		{
			description: "default value",
			builder:     NewVMBuilder("test"),
			expectation: defaultVMCPUSockets,
			expectError: false,
		},
		{
			description: "set cpu cores",
			builder:     NewVMBuilder("test").CPUSockets(4),
			expectation: 4,
			expectError: false,
		},
		{
			description: "invalid value",
			builder:     NewVMBuilder("test").CPUSockets(-2),
			expectation: 1,
			expectError: true,
		},
	}

	for _, tc := range testcases {
		testVM, err := tc.builder.VM()

		if tc.expectError && err == nil {
			t.Errorf("test %s: unexpected error", tc.description)
			continue
		}

		if !tc.expectError && err != nil {
			t.Errorf("test %s: expected error", tc.description)
			continue
		}

		if tc.expectError {
			continue
		}

		if testVM == nil || testVM.Spec.Template.Spec.Domain.CPU == nil {
			t.Errorf("test %s: unexpected nil value for domain cpu", tc.description)
			continue
		}

		if tc.expectation != testVM.Spec.Template.Spec.Domain.CPU.Sockets {
			t.Errorf("test %s: unexpected cpu sockets value: %d, expected: %d",
				tc.description,
				testVM.Spec.Template.Spec.Domain.CPU.Sockets,
				tc.expectation,
			)
			continue
		}
	}
}

func TestCPUThreads(t *testing.T) {
	type testcase struct {
		description string
		builder     *VMBuilder
		expectation uint32
		expectError bool
	}

	testcases := []testcase{
		{
			description: "default value",
			builder:     NewVMBuilder("test"),
			expectation: defaultVMCPUThreads,
			expectError: false,
		},
		{
			description: "set cpu threads",
			builder:     NewVMBuilder("test").CPUThreads(4),
			expectation: 4,
			expectError: false,
		},
		{
			description: "invalid value",
			builder:     NewVMBuilder("test").CPUThreads(-2),
			expectation: 1,
			expectError: true,
		},
	}

	for _, tc := range testcases {
		testVM, err := tc.builder.VM()

		if tc.expectError && err == nil {
			t.Errorf("test %s: unexpected error", tc.description)
			continue
		}

		if !tc.expectError && err != nil {
			t.Errorf("test %s: expected error", tc.description)
			continue
		}

		if tc.expectError {
			continue
		}

		if testVM == nil || testVM.Spec.Template.Spec.Domain.CPU == nil {
			t.Errorf("test %s: unexpected nil value for domain cpu", tc.description)
			continue
		}

		if tc.expectation != testVM.Spec.Template.Spec.Domain.CPU.Threads {
			t.Errorf("test %s: unexpected cpu threads value: %d, expected: %d",
				tc.description,
				testVM.Spec.Template.Spec.Domain.CPU.Threads,
				tc.expectation,
			)
			continue
		}
	}
}

func TestGuestMemory(t *testing.T) {
	type testcase struct {
		description string
		builder     *VMBuilder
		expectation *resource.Quantity
		expectError bool
	}

	qtyPtr := func(qty string) *resource.Quantity {
		res := resource.MustParse(qty)
		return &res
	}

	testcases := []testcase{
		{
			description: "default value",
			builder:     NewVMBuilder("test"),
			expectation: nil,
			expectError: false,
		},
		{
			description: "set guest memory",
			builder:     NewVMBuilder("test").GuestMemory("2Gi"),
			expectation: qtyPtr("2Gi"),
			expectError: false,
		},
		{
			description: "invalid value",
			builder:     NewVMBuilder("test").GuestMemory("foobar"),
			expectation: nil,
			expectError: true,
		},
	}

	for _, tc := range testcases {
		testVM, err := tc.builder.VM()

		if tc.expectError && err == nil {
			t.Errorf("test %s: unexpected error", tc.description)
			continue
		}

		if !tc.expectError && err != nil {
			t.Errorf("test %s: expected error", tc.description)
			continue
		}

		if tc.expectation == nil {
			if testVM != nil && testVM.Spec.Template.Spec.Domain.Memory != nil {
				t.Errorf("test %s: unexpected non-nil value", tc.description)
				continue
			}
		}

		if tc.expectation != nil {
			if testVM.Spec.Template.Spec.Domain.Memory == nil {
				t.Errorf("test %s: unexpected nil value for domain memory", tc.description)
				continue
			}
			if testVM.Spec.Template.Spec.Domain.Memory.Guest == nil {
				t.Errorf("test %s: unexpected nil value for guest memory", tc.description)
				continue
			}
			if !tc.expectation.Equal(*testVM.Spec.Template.Spec.Domain.Memory.Guest) {
				t.Errorf("test %s: unexpected guest memory value", tc.description)
				continue
			}
		}
	}
}

func TestDedicatedCPUPlacement(t *testing.T) {

	type testcase struct {
		name        string
		builder     *VMBuilder
		expectation bool
	}

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

	type testcase struct {
		name        string
		builder     *VMBuilder
		expectation bool
	}

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
