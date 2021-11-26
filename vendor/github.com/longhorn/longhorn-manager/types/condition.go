package types

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	"github.com/longhorn/longhorn-manager/util"
)

// GetCondition returns a copy of conditions[conditionType], and automatically fill the unknown condition
func GetCondition(conditions map[string]longhorn.Condition, conditionType string) longhorn.Condition {
	if conditions == nil {
		return getUnknownCondition(conditionType)
	}
	condition, exists := conditions[conditionType]
	if !exists {
		condition = getUnknownCondition(conditionType)
	}
	return condition
}

func getUnknownCondition(conditionType string) longhorn.Condition {
	condition := longhorn.Condition{
		Type:   conditionType,
		Status: longhorn.ConditionStatusUnknown,
	}
	return condition
}

func SetConditionAndRecord(conditions map[string]longhorn.Condition, conditionType string, conditionValue longhorn.ConditionStatus,
	reason, message string, eventRecorder record.EventRecorder, obj runtime.Object, eventtype string) map[string]longhorn.Condition {

	condition := GetCondition(conditions, conditionType)
	if condition.Status != conditionValue {
		eventRecorder.Event(obj, eventtype, conditionType, message)
	}
	return SetCondition(conditions, conditionType, conditionValue, reason, message)
}

func SetCondition(originConditions map[string]longhorn.Condition, conditionType string, conditionValue longhorn.ConditionStatus, reason, message string) map[string]longhorn.Condition {
	conditions := map[string]longhorn.Condition{}
	if originConditions != nil {
		conditions = originConditions
	}
	condition := GetCondition(conditions, conditionType)
	if condition.Status != conditionValue {
		condition.LastTransitionTime = util.Now()
	}
	condition.Status = conditionValue
	condition.Reason = reason
	condition.Message = message
	conditions[conditionType] = condition
	return conditions
}
