package summary

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/rancher/wrangler/pkg/data"
)

func getRawConditions(obj data.Object) []data.Object {
	statusAnn := obj.String("metadata", "annotations", "cattle.io/status")
	if statusAnn != "" {
		status := data.Object{}
		if err := json.Unmarshal([]byte(statusAnn), &status); err == nil {
			return append(obj.Slice("status", "conditions"), status.Slice("conditions")...)
		}
	}
	return obj.Slice("status", "conditions")
}

func getConditions(obj data.Object) (result []Condition) {
	for _, condition := range getRawConditions(obj) {
		result = append(result, Condition{d: condition})
	}
	return
}

type Condition struct {
	d data.Object
}

func (c Condition) Type() string {
	return c.d.String("type")
}

func (c Condition) Status() string {
	return c.d.String("status")
}

func (c Condition) Reason() string {
	return c.d.String("reason")
}

func (c Condition) Message() string {
	return c.d.String("message")
}

func NormalizeConditions(runtimeObj runtime.Object) {
	var (
		obj           data.Object
		newConditions []map[string]interface{}
	)

	unstr, ok := runtimeObj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	obj = unstr.Object
	for _, condition := range obj.Slice("status", "conditions") {
		var summary Summary
		for _, summarizer := range ConditionSummarizers {
			summary = summarizer(obj, []Condition{{d: condition}}, summary)
		}
		condition.Set("error", summary.Error)
		condition.Set("transitioning", summary.Transitioning)

		if condition.String("lastUpdateTime") == "" {
			condition.Set("lastUpdateTime", condition.String("lastTransitionTime"))
		}
		newConditions = append(newConditions, condition)
	}

	if len(newConditions) > 0 {
		obj.SetNested(newConditions, "status", "conditions")
	}

}
