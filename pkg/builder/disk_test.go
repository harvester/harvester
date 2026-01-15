package builder

import (
	"testing"

	kubevirtv1 "kubevirt.io/api/core/v1"
)

func TestDiskCacheMode(t *testing.T) {
	type testcase struct {
		description string
		builder     *VMBuilder
		expectError bool
		expectation kubevirtv1.DriverCache
	}
	testcases := []testcase{
		{
			description: "cache mode `none`",
			builder: NewVMBuilder("test").
				Disk("vda", "virtio", false, 1).
				DiskCacheMode("vda", kubevirtv1.CacheNone),
			expectError: false,
			expectation: kubevirtv1.CacheNone,
		},
		{
			description: "cache mode `writethrough`",
			builder: NewVMBuilder("test").
				Disk("vda", "virtio", false, 1).
				DiskCacheMode("vda", kubevirtv1.CacheWriteThrough),
			expectError: false,
			expectation: kubevirtv1.CacheWriteThrough,
		},
		{
			description: "cache mode `writeback`",
			builder: NewVMBuilder("test").
				Disk("vda", "virtio", false, 1).
				DiskCacheMode("vda", kubevirtv1.CacheWriteBack),
			expectError: false,
			expectation: kubevirtv1.CacheWriteBack,
		},
		{
			description: "multiple disks",
			builder: NewVMBuilder("test").
				Disk("vda", "virtio", false, 1).
				DiskCacheMode("vda", kubevirtv1.CacheWriteBack).
				Disk("vdb", "virtio", false, 2).
				DiskCacheMode("vdb", kubevirtv1.CacheWriteBack),
			expectError: false,
			expectation: kubevirtv1.CacheWriteBack,
		},
		{
			description: "no disks",
			builder: NewVMBuilder("test").
				DiskCacheMode("vda", kubevirtv1.CacheWriteBack),
			expectError: true,
			expectation: kubevirtv1.CacheWriteBack,
		},
		{
			description: "wrong disk",
			builder: NewVMBuilder("test").
				Disk("vda", "virtio", false, 1).
				DiskCacheMode("vdb", kubevirtv1.CacheWriteBack),
			expectError: true,
			expectation: kubevirtv1.CacheWriteBack,
		},
	}

	for _, tc := range testcases {
		testVM, err := tc.builder.VM()

		if tc.expectError {
			if err == nil {
				t.Errorf("failed test %s: generating VM should have failed with error", tc.description)
			}
		} else {
			for _, disk := range testVM.Spec.Template.Spec.Domain.Devices.Disks {
				if disk.Cache != tc.expectation {
					t.Errorf("failed test %s: cache mode %s did not match expectation %s for disk %s", tc.description, disk.Cache, tc.expectation, disk.Name)
				}
			}
		}
	}
}
