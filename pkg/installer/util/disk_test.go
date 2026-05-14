package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePartitionSize(t *testing.T) {
	testCases := []struct {
		diskSize      uint64
		partitionSize string
		result        uint64
		err           string
	}{
		{
			diskSize:      300 * GiByteMultiplier,
			partitionSize: "150Gi",
			result:        150 * GiByteMultiplier,
		},
		{
			diskSize:      500 * GiByteMultiplier,
			partitionSize: "153600Mi",
			result:        153600 * MiByteMultiplier,
		},
		{
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "1999Gi",
			err:           "partition size is too large. Maximum 1926Gi is allowed",
		},
		{
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "0Gi",
			err:           "partition size is too small. Minimum 150Gi is required",
		},
		{
			diskSize:      500 * GiByteMultiplier,
			partitionSize: "0Mi",
			err:           "partition size is too small. Minimum 150Gi is required",
		},
		{
			diskSize:      100 * GiByteMultiplier,
			partitionSize: "50Gi",
			err:           "installation disk size is too small. Minimum 250Gi is required",
		},
		{
			diskSize:      249 * GiByteMultiplier,
			partitionSize: "50Gi",
			err:           "installation disk size is too small. Minimum 250Gi is required",
		},
		{
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "abcd",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "1Ti",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "50Ki",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "5.5",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			diskSize:      400 * GiByteMultiplier,
			partitionSize: "385933Mi",
			err:           "partition size is too large. Maximum 326Gi is allowed",
		},
	}

	for _, tc := range testCases {
		result, err := ParsePartitionSize(tc.diskSize, tc.partitionSize, false)
		assert.Equal(t, tc.result, result)
		if err != nil {
			assert.EqualError(t, err, tc.err)
		}
	}
}

func TestParsePartitionSizeWithSkipChecks(t *testing.T) {
	testCases := []struct {
		testName      string
		diskSize      uint64
		partitionSize string
		result        uint64
		err           string
	}{
		{
			testName:      "Valid partition size",
			diskSize:      300 * GiByteMultiplier,
			partitionSize: "150Gi",
			result:        150 * GiByteMultiplier,
		},
		{
			testName:      "Valid partition size with Mi suffix",
			diskSize:      500 * GiByteMultiplier,
			partitionSize: "153600Mi",
			result:        153600 * MiByteMultiplier,
		},
		{
			testName:      "Partition size should be overwritten when too large",
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "1999Gi",
			result:        1973134 * MiByteMultiplier,
		},
		{
			testName:      "Should fail when partition size is too small",
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "0Gi",
			err:           "partition size is too small. Minimum 150Gi is required",
		},
		{
			testName:      "Should fail when partition size is too small",
			diskSize:      500 * GiByteMultiplier,
			partitionSize: "0Mi",
			err:           "partition size is too small. Minimum 150Gi is required",
		},
		{
			testName:      "Should allow for small installation disk size (249GB)",
			diskSize:      249 * GiByteMultiplier,
			partitionSize: "50Gi",
			result:        50 * GiByteMultiplier,
		},
		{
			testName:      "Should allow for small installation disk size (100GB)",
			diskSize:      100 * GiByteMultiplier,
			partitionSize: "50Gi",
			result:        27534 * MiByteMultiplier,
		},
		{
			testName:      "partition size must end with 'Mi' or 'Gi'",
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "abcd",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			testName:      "partition size must end with 'Mi' or 'Gi'",
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "1Ti",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			testName:      "partition size must end with 'Mi' or 'Gi'",
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "50Ki",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			testName:      "partition size must end with 'Mi' or 'Gi'",
			diskSize:      2000 * GiByteMultiplier,
			partitionSize: "5.5",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			testName:      "Partition size should be overwritten",
			diskSize:      400 * GiByteMultiplier,
			partitionSize: "385933Mi",
			result:        334734 * MiByteMultiplier,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			result, err := ParsePartitionSize(tc.diskSize, tc.partitionSize, true)
			assert.Equal(t, tc.result, result)
			if err != nil {
				assert.EqualError(t, err, tc.err)
			}
		})
	}
}
