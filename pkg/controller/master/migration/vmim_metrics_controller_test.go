//go:build linux && amd64

package migration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

func Test_getVMIMigrationMetrics(t *testing.T) {
	metricData := `
# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 2.3892e-05
go_gc_duration_seconds{quantile="0.25"} 6.2596e-05
go_gc_duration_seconds{quantile="0.5"} 8.9073e-05
go_gc_duration_seconds{quantile="0.75"} 0.000175126
go_gc_duration_seconds{quantile="1"} 0.002836034
go_gc_duration_seconds_sum 0.008440045
go_gc_duration_seconds_count 49
# HELP kubevirt_vmi_migration_data_processed_bytes Number of bytes transferred from the beginning of the job.
# TYPE kubevirt_vmi_migration_data_processed_bytes gauge
kubevirt_vmi_migration_data_processed_bytes{kubernetes_vmi_label_harvesterhci_io_vmName="test",kubernetes_vmi_label_kubevirt_io_migrationTargetNodeName="harvester-node-0",kubernetes_vmi_label_kubevirt_io_nodeName="harvester-node-1",name="test",namespace="default",node="harvester-node-1"} 3.39098348e+08
kubevirt_vmi_migration_data_processed_bytes{kubernetes_vmi_label_harvesterhci_io_vmName="test2",kubernetes_vmi_label_kubevirt_io_migrationTargetNodeName="harvester-node-0",kubernetes_vmi_label_kubevirt_io_nodeName="harvester-node-1",name="test2",namespace="default",node="harvester-node-1"} 3.28214507e+08
# HELP kubevirt_vmi_migration_data_remaining_bytes Number of bytes that still need to be transferred
# TYPE kubevirt_vmi_migration_data_remaining_bytes gauge
kubevirt_vmi_migration_data_remaining_bytes{kubernetes_vmi_label_harvesterhci_io_vmName="test",kubernetes_vmi_label_kubevirt_io_migrationTargetNodeName="harvester-node-0",kubernetes_vmi_label_kubevirt_io_nodeName="harvester-node-1",name="test",namespace="default",node="harvester-node-1"} 7.64030976e+08
kubevirt_vmi_migration_data_remaining_bytes{kubernetes_vmi_label_harvesterhci_io_vmName="test2",kubernetes_vmi_label_kubevirt_io_migrationTargetNodeName="harvester-node-0",kubernetes_vmi_label_kubevirt_io_nodeName="harvester-node-1",name="test2",namespace="default",node="harvester-node-1"} 7.83855616e+08
# HELP kubevirt_vmi_migration_dirty_memory_rate_bytes Number of memory pages dirtied by the guest per second.
# TYPE kubevirt_vmi_migration_dirty_memory_rate_bytes gauge
kubevirt_vmi_migration_dirty_memory_rate_bytes{kubernetes_vmi_label_harvesterhci_io_vmName="test",kubernetes_vmi_label_kubevirt_io_migrationTargetNodeName="harvester-node-0",kubernetes_vmi_label_kubevirt_io_nodeName="harvester-node-1",name="test",namespace="default",node="harvester-node-1"} 0
kubevirt_vmi_migration_dirty_memory_rate_bytes{kubernetes_vmi_label_harvesterhci_io_vmName="test2",kubernetes_vmi_label_kubevirt_io_migrationTargetNodeName="harvester-node-0",kubernetes_vmi_label_kubevirt_io_nodeName="harvester-node-1",name="test2",namespace="default",node="harvester-node-1"} 0
# HELP kubevirt_vmi_migration_disk_transfer_rate_bytes Network throughput used while migrating memory in Bytes per second.
# TYPE kubevirt_vmi_migration_disk_transfer_rate_bytes gauge
kubevirt_vmi_migration_disk_transfer_rate_bytes{kubernetes_vmi_label_harvesterhci_io_vmName="test",kubernetes_vmi_label_kubevirt_io_migrationTargetNodeName="harvester-node-0",kubernetes_vmi_label_kubevirt_io_nodeName="harvester-node-1",name="test",namespace="default",node="harvester-node-1"} 1.06722e+06
kubevirt_vmi_migration_disk_transfer_rate_bytes{kubernetes_vmi_label_harvesterhci_io_vmName="test2",kubernetes_vmi_label_kubevirt_io_migrationTargetNodeName="harvester-node-0",kubernetes_vmi_label_kubevirt_io_nodeName="harvester-node-1",name="test2",namespace="default",node="harvester-node-1"} 1.06722e+06
`
	vmiTest := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test"},
	}

	metrics, err := getVMIMigrationMetrics(strings.NewReader(metricData), vmiTest)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Len(t, metrics, 4)
	assert.Equal(t, 3.39098348e+08, metrics[migrationDataProcessedBytes])
	assert.Equal(t, 7.64030976e+08, metrics[migrationDataRemainingBytes])
	assert.Equal(t, 0.0, metrics[migrationDirtyMemoryRateBytes])
	assert.Equal(t, 1.06722e+06, metrics[migrationDiskTransferRateBytes])

	vmiTest2 := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test2"},
	}

	metrics, err = getVMIMigrationMetrics(strings.NewReader(metricData), vmiTest2)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Len(t, metrics, 4)
	assert.Equal(t, 3.28214507e+08, metrics[migrationDataProcessedBytes])
	assert.Equal(t, 7.83855616e+08, metrics[migrationDataRemainingBytes])
	assert.Equal(t, 0.0, metrics[migrationDirtyMemoryRateBytes])
	assert.Equal(t, 1.06722e+06, metrics[migrationDiskTransferRateBytes])
}

func Test_getVirtHandlerIPPort(t *testing.T) {
	tests := []struct {
		name         string
		endpoints    *corev1.Endpoints
		nodeName     string
		expectedIP   string
		expectedPort int32
		expectedErr  error
	}{
		{
			name: "success case",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								NodeName: stringPointer("harvester-node-0"),
								IP:       "10.52.1.5",
								TargetRef: &corev1.ObjectReference{
									Name: "virt-handler-vvrs4",
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     metricsPortName,
								Port:     8432,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			},
			nodeName:     "harvester-node-0",
			expectedIP:   "10.52.1.5",
			expectedPort: 8432,
			expectedErr:  nil,
		},
		{
			name: "missing virt-handler prefix",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								NodeName: stringPointer("harvester-node-0"),
								IP:       "10.52.1.5",
								TargetRef: &corev1.ObjectReference{
									Name: "virt-controller-5c58cbf4fd-sffvg",
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     metricsPortName,
								Port:     8432,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			},
			nodeName:    "harvester-node-0",
			expectedErr: fmt.Errorf("failed to find the virt-handler endpoint IP for the node %s", "harvester-node-0"),
		},
		{
			name: "missing endpoint port",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								NodeName: stringPointer("harvester-node-0"),
								IP:       "10.52.1.5",
								TargetRef: &corev1.ObjectReference{
									Name: "virt-handler-harvester-node-0",
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     "other-port",
								Port:     8432,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			},
			nodeName:    "harvester-node-0",
			expectedErr: fmt.Errorf("failed to find the metrics endpoint port for the node %s", "harvester-node-0"),
		},
		{
			name: "missing address",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{},
						Ports: []corev1.EndpointPort{
							{
								Name:     metricsPortName,
								Port:     8432,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			},
			nodeName:    "harvester-node-0",
			expectedErr: fmt.Errorf("failed to find the virt-handler endpoint IP for the node %s", "harvester-node-0"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, port, err := getVirtHandlerIPPort(tt.endpoints, tt.nodeName)

			// Assert the error is as expected
			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
				// Assert the IP and port are as expected
				assert.Equal(t, tt.expectedIP, ip)
				assert.Equal(t, tt.expectedPort, port)
			}
		})
	}
}

func stringPointer(s string) *string {
	return &s
}
