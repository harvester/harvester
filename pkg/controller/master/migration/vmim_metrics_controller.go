package migration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"github.com/harvester/harvester/pkg/util"
)

const (
	migrationDataProcessedBytes    = "kubevirt_vmi_migration_data_processed_bytes"
	migrationDataRemainingBytes    = "kubevirt_vmi_migration_data_remaining_bytes"
	migrationDirtyMemoryRateBytes  = "kubevirt_vmi_migration_dirty_memory_rate_bytes"
	migrationDiskTransferRateBytes = "kubevirt_vmi_migration_disk_transfer_rate_bytes"

	virtHandlerPrefix = "virt-handler"
	metricsPortName   = "metrics"

	metricNameLabel      = "name"
	metricNamespaceLabel = "namespace"

	updatePeriod = 1 * time.Minute
)

var migrationMetricNames = []string{
	migrationDataProcessedBytes,
	migrationDataRemainingBytes,
	migrationDirtyMemoryRateBytes,
	migrationDiskTransferRateBytes,
}

type migrationMetrics struct {
	Metrics             map[string]interface{} `json:"metrics,omitempty"`
	LastUpdateTimestamp int64                  `json:"lastUpdateTimestamp,omitempty"`
}

// OnVmimChangedUpdateMetrics updates the metrics annotation for the VMIM
func (h *Handler) OnVmimChangedUpdateMetrics(_ string, vmim *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	if vmim == nil {
		return nil, nil
	}

	if vmim.DeletionTimestamp != nil {
		return vmim, nil
	}

	fields := logrus.Fields{
		"apiVersion": vmim.APIVersion,
		"kind":       vmim.Kind,
		"namespace":  vmim.Namespace,
		"name":       vmim.Name,
	}

	// clear the metrics annotation if the migration is not running
	if vmim.Status.Phase != kubevirtv1.MigrationRunning {
		copyVMIM := vmim.DeepCopy()
		delete(copyVMIM.Annotations, util.AnnotationMigrationMetrics)
		// do not update if the copyVMIM is the same as the original VMIM
		if reflect.DeepEqual(copyVMIM, vmim) {
			return vmim, nil
		}
		logrus.WithFields(fields).Infof("Clean metrics annotation")
		return h.vmims.Update(copyVMIM)
	}

	needUpdate, reason := shouldUpdateMetrics(vmim)
	logrus.WithFields(fields).Infof("Metrics need to update: %t, reason: %s", needUpdate, reason)

	if needUpdate {
		vmi, err := h.vmiCache.Get(vmim.Namespace, vmim.Spec.VMIName)
		if err != nil {
			return vmim, fmt.Errorf("failed to get the VMI %s/%s: %w", vmim.Namespace, vmim.Spec.VMIName, err)
		}

		endpoint, err := h.endpointCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtPrometheusMetricsEndpointName)
		if err != nil {
			return vmim, fmt.Errorf("failed to get the endpoint %s/%s: %w", util.HarvesterSystemNamespaceName, util.KubeVirtPrometheusMetricsEndpointName, err)
		}

		ip, port, err := getVirtHandlerIPPort(endpoint, vmi.Status.NodeName)
		if err != nil {
			return vmim, fmt.Errorf("failed to get virt-handler endpoint: %w", err)
		}

		// TODO: remove this line
		logrus.WithFields(fields).Infof("Send request to https://%s:%d/metrics", ip, port)

		address := fmt.Sprintf("https://%s:%d/metrics", ip, port)
		resp, err := h.httpClient.Get(address)
		if err != nil {
			return vmim, fmt.Errorf("failed to get metrics from virt-handler %s: %w", address, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return vmim, fmt.Errorf("unexpected response code: %d from %s ", resp.StatusCode, address)
		}

		metrics, err := getVMIMigrationMetrics(resp.Body, vmi)
		if err != nil {
			return vmim, fmt.Errorf("failed to get the metrics: %w", err)
		}

		migrationMetricsBytes, err := json.Marshal(migrationMetrics{
			Metrics:             metrics,
			LastUpdateTimestamp: time.Now().Unix(),
		})
		if err != nil {
			return vmim, fmt.Errorf("failed to marshal the metrics: %w", err)
		}

		// TODO: remove this line
		logrus.WithFields(fields).Info("EnqueueAfter")

		// enqueue the VMIM for the next metrics update
		h.vmims.EnqueueAfter(vmim.Namespace, vmim.Name, updatePeriod)

		copyVMIM := vmim.DeepCopy()
		if copyVMIM.Annotations == nil {
			copyVMIM.Annotations = make(map[string]string)
		}
		// update the VMIM with the metrics annotation
		copyVMIM.Annotations[util.AnnotationMigrationMetrics] = string(migrationMetricsBytes)
		// do not update if the copyVMIM is the same as the original VMIM
		if reflect.DeepEqual(copyVMIM, vmim) {
			return vmim, nil
		}

		// TODO: remove this line
		logrus.WithFields(fields).Info("Metrics update")

		return h.vmims.Update(copyVMIM)
	}

	return vmim, nil
}

// shouldUpdateMetrics checks if the metrics for the VMIM should be updated
func shouldUpdateMetrics(vmim *kubevirtv1.VirtualMachineInstanceMigration) (bool, string) {
	if vmim == nil || vmim.Status.Phase != kubevirtv1.MigrationRunning {
		return false, "vmim is nil or not running"
	}

	if _, ok := vmim.Annotations[util.AnnotationMigrationMetrics]; !ok {
		return true, "metrics annotation is missing"
	}

	migrationMetrics := migrationMetrics{}
	err := json.Unmarshal([]byte(vmim.Annotations[util.AnnotationMigrationMetrics]), &migrationMetrics)
	if err != nil {
		return true, fmt.Sprintf("failed to unmarshal the metrics: %v", err)
	}
	// update the metrics once per minute
	currentTS := time.Now().Unix()
	if (currentTS - migrationMetrics.LastUpdateTimestamp) >= int64(updatePeriod.Seconds()) {
		logrus.Infof("Metrics are outdated for %s/%s, current: %d, lastUpdate: %d", vmim.Namespace, vmim.Name, currentTS, migrationMetrics.LastUpdateTimestamp)
		return true, "metrics are outdated"
	}
	return false, "metrics are up to date"
}

// getVirtHandlerIPPort find virt-handler endpoint address for the node that the VMI is running on
func getVirtHandlerIPPort(endpoints *corev1.Endpoints, nodeName string) (string, int32, error) {
	var endpointIP string
	var endpointPort int32
	for _, subset := range endpoints.Subsets {
		for _, address := range subset.Addresses {
			if address.NodeName != nil && *address.NodeName == nodeName && address.TargetRef != nil && strings.HasPrefix(address.TargetRef.Name, virtHandlerPrefix) {
				// update metrics
				endpointIP = address.IP
			}
		}

		for _, port := range subset.Ports {
			if port.Name == metricsPortName && port.Protocol == corev1.ProtocolTCP {
				endpointPort = port.Port
			}
		}
	}
	if endpointIP == "" {
		err := fmt.Errorf("failed to find the virt-handler endpoint IP for the node %s", nodeName)
		return endpointIP, endpointPort, err
	}
	if endpointPort == 0 {
		err := fmt.Errorf("failed to find the metrics endpoint port for the node %s", nodeName)
		return endpointIP, endpointPort, err
	}
	return endpointIP, endpointPort, nil
}

// getVMIMigrationMetrics parses the metrics in exposition formats from the reader
// and returns the metrics that are relevant to the migration of specific VMI
func getVMIMigrationMetrics(reader io.Reader, vmi *kubevirtv1.VirtualMachineInstance) (map[string]interface{}, error) {
	parser := expfmt.TextParser{}
	metricFamilies, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}

	metrics := make(map[string]interface{}, len(migrationMetricNames))

	namespaceMatch := func(label *dto.LabelPair) bool {
		return label.GetName() == metricNamespaceLabel && label.GetValue() == vmi.Namespace
	}
	nameMatch := func(label *dto.LabelPair) bool {
		return label.GetName() == metricNameLabel && label.GetValue() == vmi.Name
	}

	// filter out the metrics that are not relevant to the migration
	for name, mf := range metricFamilies {
		if slices.Contains(migrationMetricNames, name) {
			for _, metric := range mf.Metric {
				if labelMatches(metric.Label, namespaceMatch) && labelMatches(metric.Label, nameMatch) {
					metrics[name] = metric.Gauge.GetValue()
				}
			}
		}
	}

	return metrics, nil
}

// labelMatches checks if any of the labels in the slice matches the test function
func labelMatches(ss []*dto.LabelPair, test func(*dto.LabelPair) bool) bool {
	for _, s := range ss {
		if test(s) {
			return true
		}
	}
	return false
}
