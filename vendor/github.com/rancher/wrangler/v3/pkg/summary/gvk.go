package summary

import (
	"encoding/json"
	"fmt"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	gvkRegExp = regexp.MustCompile(`^(.*?)/(.*),\s*Kind=(.+)$`)
)

// conditionTypeStatusJSON is a custom JSON to map a complex object into a standard JSON object. It maps Groups,
// Versions and Kinds to Conditions, Types, and Status, indicating with a flag if a certain condition with specific
// status represents an error or not. It is expected to be something like:
//
//	{
//		"gvk": 			"helm.cattle.io/v1, Kind=HelmChart",
//		"conditionMappings": [
//			{
//				"type": "JobCreated"	// This means JobCreated is mostly informational and True or False
//			},				// doesn't mean error
//			{
//				"type": "Failed",	// This means Failed is considered error if it's status is True
//				"status": ["True"]
//			},
//		}
//	}
type conditionTypeStatusJSON struct {
	GVK               string                     `json:"gvk"`
	ConditionMappings []conditionStatusErrorJSON `json:"conditionMappings"`
}

type conditionStatusErrorJSON struct {
	Type   string                   `json:"type"`
	Status []metav1.ConditionStatus `json:"status,omitempty"`
}

type ConditionTypeStatusErrorMapping map[schema.GroupVersionKind]map[string]sets.Set[metav1.ConditionStatus]

func (m ConditionTypeStatusErrorMapping) MarshalJSON() ([]byte, error) {
	output := []conditionTypeStatusJSON{}
	for gvk, mapping := range m {
		typeStatus := conditionTypeStatusJSON{GVK: gvk.String()}
		for condition, statuses := range mapping {
			typeStatus.ConditionMappings = append(typeStatus.ConditionMappings, conditionStatusErrorJSON{
				Type:   condition,
				Status: statuses.UnsortedList(),
			})
		}
		output = append(output, typeStatus)
	}
	return json.Marshal(output)
}

func (m ConditionTypeStatusErrorMapping) UnmarshalJSON(data []byte) error {
	var conditionMappingsJSON []conditionTypeStatusJSON
	err := json.Unmarshal(data, &conditionMappingsJSON)
	if err != nil {
		return err
	}

	for _, mapping := range conditionMappingsJSON {
		// checking if mapping.GVK is in the right format: <group>/[version], Kind=[kind]
		mx := gvkRegExp.FindStringSubmatch(mapping.GVK)
		if len(mx) == 0 {
			return fmt.Errorf("gvk parsing failed: wrong GVK format: <%s>\n", mapping.GVK)
		}

		conditionMappings := map[string]sets.Set[metav1.ConditionStatus]{}
		for _, condition := range mapping.ConditionMappings {
			conditionMappings[condition.Type] = sets.New[metav1.ConditionStatus](condition.Status...)
		}

		m[schema.GroupVersionKind{
			Group:   mx[1],
			Version: mx[2],
			Kind:    mx[3],
		}] = conditionMappings
	}
	return nil
}
