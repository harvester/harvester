// Package common provides cache.TransformFunc's which are common to all types
package common

import (
	"fmt"
	"strings"

	"github.com/rancher/steve/pkg/summarycache"
	"github.com/rancher/wrangler/v3/pkg/data"
	wranglerSummary "github.com/rancher/wrangler/v3/pkg/summary"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// SummaryCache provides an interface to get a summary/relationships for an object. Implemented by the summaryCache
// struct from pkg/summarycache
type SummaryCache interface {
	SummaryAndRelationship(runtime.Object) (*wranglerSummary.SummarizedObject, []summarycache.Relationship)
}

// DefaultFields produces a VirtualTransformFunc through GetTransform() that applies to all k8s types
type DefaultFields struct {
	Cache SummaryCache
}

// TransformCommon implements virtual.VirtualTransformFunc, and adds reserved fields/summary
func (d *DefaultFields) TransformCommon(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	obj = addIDField(obj)
	obj, err := addSummaryFields(obj, d.Cache)
	if err != nil {
		return nil, fmt.Errorf("unable to add summary fields: %w", err)
	}
	return obj, nil
}

// addSummaryFields adds the virtual fields for object state.
func addSummaryFields(raw *unstructured.Unstructured, cache SummaryCache) (*unstructured.Unstructured, error) {
	s, relationships := cache.SummaryAndRelationship(raw)
	if s != nil {
		data.PutValue(raw.Object, map[string]interface{}{
			"name":          s.State,
			"error":         s.Error,
			"transitioning": s.Transitioning,
			"message":       strings.Join(s.Message, ":"),
		}, "metadata", "state")

	}
	var rels []any
	for _, relationship := range relationships {
		rel, err := toMap(relationship)
		if err != nil {
			return nil, fmt.Errorf("unable to convert relationship to map: %w", err)
		}
		rels = append(rels, rel)
	}
	data.PutValue(raw.Object, rels, "metadata", "relationships")

	normalizeConditions(raw)
	return raw, nil
}

// addIDField adds the ID field based on namespace/name, and moves the current id field to _id if present
func addIDField(raw *unstructured.Unstructured) *unstructured.Unstructured {
	objectID := raw.GetName()
	namespace := raw.GetNamespace()
	if namespace != "" {
		objectID = fmt.Sprintf("%s/%s", namespace, objectID)
	}
	currentIDValue, ok := raw.Object["id"]
	if ok {
		raw.Object["_id"] = currentIDValue
	}
	raw.Object["id"] = objectID
	return raw
}

func normalizeConditions(raw *unstructured.Unstructured) {
	var (
		obj           data.Object
		newConditions []any
	)

	obj = raw.Object
	for _, condition := range obj.Slice("status", "conditions") {
		var summary wranglerSummary.Summary
		for _, summarizer := range wranglerSummary.ConditionSummarizers {
			summary = summarizer(obj, []wranglerSummary.Condition{{Object: condition}}, summary)
		}
		condition.Set("error", summary.Error)
		condition.Set("transitioning", summary.Transitioning)

		if condition.String("lastUpdateTime") == "" {
			condition.Set("lastUpdateTime", condition.String("lastTransitionTime"))
		}
		// needs to be reconverted back to a map[string]any or we can have encoding problems with unregistered types
		var mapCondition map[string]any = condition
		newConditions = append(newConditions, mapCondition)
	}

	if len(newConditions) > 0 {
		obj.SetNested(newConditions, "status", "conditions")
	}
}
