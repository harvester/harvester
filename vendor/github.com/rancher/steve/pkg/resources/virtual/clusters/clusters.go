package clusters

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TransformManagedClusters does special-case handling on <management.cattle.io v3 Cluster>s:
// creates a new virtual `status.connected` boolean field that looks for `type = "Ready"` in any
// of the status.conditions records.
//
// It also converts the annotated status.requested.memory and status.allocatable.memory fields into
// their underlying byte values.

func TransformManagedCluster(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	conditions, ok, err := unstructured.NestedFieldNoCopy(obj.Object, "status", "conditions")
	if err != nil {
		return obj, err
	}
	if !ok {
		return obj, fmt.Errorf("failed to find status.conditions block in cluster %s", obj.GetName())
	}
	connectedStatus := false
	conditionsAsArray, ok := conditions.([]interface{})
	if !ok {
		return obj, fmt.Errorf("failed to parse status.conditions as array")
	}
	for _, condition := range conditionsAsArray {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			return obj, fmt.Errorf("failed to parse a condition (type %t) as a map", condition)
		}
		if conditionMap["type"] == "Ready" && conditionMap["status"] == "True" {
			connectedStatus = true
			break
		}
	}
	err = unstructured.SetNestedField(obj.Object, connectedStatus, "status", "connected")
	if err != nil {
		return obj, err
	}

	for _, statusName := range []string{"requested", "allocatable"} {
		mapx, ok, err := unstructured.NestedMap(obj.Object, "status", statusName)
		if ok && err == nil {
			mem, ok := mapx["memory"]
			madeChange := false
			if ok {
				quantity, err := resource.ParseQuantity(mem.(string))
				if err == nil {
					mapx["memoryRaw"] = quantity.AsApproximateFloat64()
					madeChange = true
				} else {
					logrus.Errorf("Failed to parse memory quantity <%v>: %v", mem, err)
				}
			}
			cpu, ok := mapx["cpu"]
			if ok {
				quantity, err := resource.ParseQuantity(cpu.(string))
				if err == nil {
					mapx["cpuRaw"] = quantity.AsApproximateFloat64()
					madeChange = true
				} else {
					logrus.Errorf("Failed to parse cpu quantity <%v>: %v", cpu, err)
				}
			}
			if madeChange {
				unstructured.SetNestedMap(obj.Object, mapx, "status", statusName)
			}
		}
	}
	return obj, nil
}
