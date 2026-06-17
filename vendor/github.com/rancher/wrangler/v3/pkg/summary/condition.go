package summary

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/rancher/wrangler/v3/pkg/data"
)

const capiAPIVersionV1Beta2 = "cluster.x-k8s.io/v1beta2"

func GetUnstructuredConditions(obj map[string]interface{}) []Condition {
	return getConditions(obj)
}

func getRawConditions(obj data.Object) []data.Object {
	// For CAPI v1beta2 resources, use the deprecated v1beta1 conditions if they exist
	// Otherwise, use standard status.conditions
	conditions := getDeprecatedV1beta1Conditions(obj)
	if len(conditions) == 0 {
		conditions = obj.Slice("status", "conditions")
	}

	// Append conditions from cattle.io/status annotation
	conditions = append(conditions, getAnnotationConditions(obj)...)

	return conditions
}

// getDeprecatedV1beta1Conditions returns the deprecated v1beta1 conditions for CAPI v1beta2 resources.
// Returns nil if not a CAPI v1beta2 resource or if no deprecated conditions exist.
func getDeprecatedV1beta1Conditions(obj data.Object) []data.Object {
	if obj.String("apiVersion") != capiAPIVersionV1Beta2 {
		return nil
	}
	return obj.Slice("status", "deprecated", "v1beta1", "conditions")
}

// getAnnotationConditions extracts conditions from the cattle.io/status annotation.
// Returns an empty slice if the annotation doesn't exist or cannot be parsed.
func getAnnotationConditions(obj data.Object) []data.Object {
	statusAnn := obj.String("metadata", "annotations", "cattle.io/status")
	if statusAnn == "" {
		return []data.Object{}
	}

	var status data.Object
	if err := json.Unmarshal([]byte(statusAnn), &status); err != nil {
		return []data.Object{}
	}

	conditions := status.Slice("conditions")
	if conditions == nil {
		return []data.Object{}
	}
	return conditions
}

func getConditions(obj data.Object) (result []Condition) {
	for _, condition := range getRawConditions(obj) {
		result = append(result, Condition{Object: condition})
	}
	return
}

type Condition struct {
	data.Object
}

func NewCondition(conditionType, status, reason, message string) Condition {
	return Condition{
		Object: map[string]interface{}{
			"type":    conditionType,
			"status":  status,
			"reason":  reason,
			"message": message,
		},
	}
}

func (c Condition) Type() string {
	return c.String("type")
}

func (c Condition) Status() string {
	return c.String("status")
}

func (c Condition) Reason() string {
	return c.String("reason")
}

func (c Condition) Message() string {
	return c.String("message")
}

func (c Condition) Equals(other Condition) bool {
	return c.Type() == other.Type() &&
		c.Status() == other.Status() &&
		c.Reason() == other.Reason() &&
		c.Message() == other.Message()
}

func NormalizeConditions(runtimeObj runtime.Object) {
	if runtimeObj == nil {
		return
	}

	unstr, ok := runtimeObj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	obj := data.Object(unstr.Object)

	// For CAPI v1beta2 resources, normalize the deprecated v1beta1 conditions if they exist
	if deprecatedConditions := getDeprecatedV1beta1Conditions(obj); len(deprecatedConditions) > 0 {
		normalizeAndSetConditions(obj, deprecatedConditions, "status", "deprecated", "v1beta1", "conditions")
	}

	// For all resources, normalize the standard status.conditions
	if conditions := obj.Slice("status", "conditions"); len(conditions) > 0 {
		normalizeAndSetConditions(obj, conditions, "status", "conditions")
	}
}

func normalizeAndSetConditions(obj data.Object, conditions []data.Object, path ...string) {
	var newConditions []interface{}
	for _, condition := range conditions {
		var summary Summary
		for _, summarizer := range ConditionSummarizers {
			summary = summarizer(obj, []Condition{{Object: condition}}, summary)
		}
		condition.Set("error", summary.Error)
		condition.Set("transitioning", summary.Transitioning)

		if condition.String("lastUpdateTime") == "" {
			condition.Set("lastUpdateTime", condition.String("lastTransitionTime"))
		}
		newConditions = append(newConditions, map[string]interface{}(condition))
	}

	if len(newConditions) > 0 {
		obj.SetNested(newConditions, path...)
	}
}
