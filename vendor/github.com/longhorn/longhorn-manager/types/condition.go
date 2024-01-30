package types

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

// GetCondition returns a copy of conditions[conditionType], and automatically fill the unknown condition
func GetCondition(conditions []longhorn.Condition, conditionType string) longhorn.Condition {
	if conditions == nil {
		return getUnknownCondition(conditionType)
	}

	for i := range conditions {
		if conditions[i].Type == conditionType {
			return conditions[i]
		}
	}

	return getUnknownCondition(conditionType)
}

func getUnknownCondition(conditionType string) longhorn.Condition {
	condition := longhorn.Condition{
		Type:   conditionType,
		Status: longhorn.ConditionStatusUnknown,
	}
	return condition
}

func SetConditionAndRecord(conditions []longhorn.Condition, conditionType string, conditionValue longhorn.ConditionStatus,
	reason, message string, eventRecorder record.EventRecorder, obj runtime.Object, eventtype string) []longhorn.Condition {

	condition := GetCondition(conditions, conditionType)

	if condition.Status != conditionValue {
		eventRecorder.Event(obj, eventtype, conditionType, message)
	}
	return SetCondition(conditions, conditionType, conditionValue, reason, message)
}

func SetCondition(originConditions []longhorn.Condition, conditionType string, conditionValue longhorn.ConditionStatus, reason, message string) []longhorn.Condition {
	return setCondition(originConditions, conditionType, conditionValue, reason, message, true)
}

func SetConditionWithoutTimestamp(originConditions []longhorn.Condition, conditionType string, conditionValue longhorn.ConditionStatus, reason, message string) []longhorn.Condition {
	return setCondition(originConditions, conditionType, conditionValue, reason, message, false)
}

func setCondition(originConditions []longhorn.Condition, conditionType string, conditionValue longhorn.ConditionStatus, reason, message string, withTimestamp bool) []longhorn.Condition {
	conditions := []longhorn.Condition{}
	if originConditions != nil {
		conditions = originConditions
	}

	condition := GetCondition(conditions, conditionType)
	if withTimestamp &&
		condition.Status != conditionValue {
		condition.LastTransitionTime = util.Now()
	}

	condition.Status = conditionValue
	condition.Reason = reason
	condition.Message = message

	return updateOrAppendCondition(conditions, condition)
}

func updateOrAppendCondition(conditions []longhorn.Condition, condition longhorn.Condition) []longhorn.Condition {
	for i := range conditions {
		if conditions[i].Type == condition.Type {
			conditions[i] = condition
			return conditions
		}
	}

	return append(conditions, condition)
}
