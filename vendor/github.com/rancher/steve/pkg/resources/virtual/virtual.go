// Package virtual provides functions/resources to define virtual fields (fields which don't exist in k8s
// but should be visible in the API) on resources
package virtual

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	rescommon "github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/steve/pkg/resources/virtual/clusters"
	"github.com/rancher/steve/pkg/resources/virtual/common"
	"github.com/rancher/steve/pkg/resources/virtual/dates"
	"github.com/rancher/steve/pkg/resources/virtual/events"
	"github.com/rancher/steve/pkg/resources/virtual/pods"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/jsonpath"
)

var now = time.Now

var restartsPattern = regexp.MustCompile(`^(\d+)(?:\s+\((.+?)\s+ago\))?`)

// ParseRestarts parses pod restart values like "4 (3h38m ago)" into a map
func ParseRestarts(value string) (map[string]any, error) {
	matches := restartsPattern.FindStringSubmatch(value)
	if matches == nil {
		return nil, fmt.Errorf("invalid restarts format: %q", value)
	}
	count, _ := strconv.ParseInt(matches[1], 10, 64)
	var timestamp int64
	if matches[2] != "" {
		dur, err := rescommon.ParseTimestampOrHumanReadableDuration(matches[2])
		if err != nil {
			logrus.Errorf("failed to parse restart duration %q: %v", matches[2], err)
		} else {
			timestamp = now().Add(-dur).UnixMilli()
		}
	}
	return map[string]any{"count": count, "timestamp": timestamp}, nil
}

// TransformBuilder builds transform functions for specified GVKs through GetTransformFunc
type TransformBuilder struct {
	defaultFields *common.DefaultFields
}

// NewTransformBuilder returns a TransformBuilder using the given summary cache
func NewTransformBuilder(cache common.SummaryCache) *TransformBuilder {
	return &TransformBuilder{
		defaultFields: &common.DefaultFields{
			Cache: cache,
		},
	}
}

// GetTransformFunc returns the func to transform a raw object into a fixed object, if needed
func (t *TransformBuilder) GetTransformFunc(gvk schema.GroupVersionKind, columns []rescommon.ColumnDefinition, isCRD bool, jsonPaths map[string]*jsonpath.JSONPath) cache.TransformFunc {
	converters := make([]func(*unstructured.Unstructured) (*unstructured.Unstructured, error), 0)
	converters = append(converters, t.defaultFields.TransformCommon)
	if gvk.Group == "" && gvk.Version == "v1" {
		if gvk.Kind == "Event" {
			converters = append(converters, events.TransformEventObject)
		} else if gvk.Kind == "Pod" {
			converters = append(converters, pods.TransformPodObject)
		}
	} else if gvk.Kind == "Cluster" && gvk.Group == "management.cattle.io" && gvk.Version == "v3" {
		converters = append(converters, clusters.TransformManagedCluster)
	}

	// Pod Logic
	if gvk.Kind == "Pod" && gvk.Version == "v1" {
		for _, col := range columns {
			if col.Name == "Restarts" {
				converters = append(converters, func(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
					index := rescommon.GetIndexValueFromString(col.Field)
					if index == -1 {
						return obj, nil
					}

					fields, found, err := unstructured.NestedSlice(obj.Object, "metadata", "fields")
					if err != nil || !found || index >= len(fields) {
						return obj, nil
					}

					val, ok := fields[index].(string)
					if !ok {
						return obj, nil
					}

					parsed, err := ParseRestarts(val)
					if err != nil {
						logrus.Warnf("Failed to parse restarts: %v", err)
						return obj, nil
					}

					fields[index] = parsed
					unstructured.SetNestedSlice(obj.Object, fields, "metadata", "fields")
					return obj, nil
				})
			}
		}
	}

	// Detecting if we need to convert date fields
	dateConverter := &dates.Converter{
		GVK:       gvk,
		Columns:   columns,
		IsCRD:     isCRD,
		JSONPaths: jsonPaths,
	}
	converters = append(converters, dateConverter.Transform)

	return func(raw interface{}) (interface{}, error) {
		obj, isSignal, err := common.GetUnstructured(raw)
		if isSignal {
			// isSignal= true overrides any error
			return raw, err
		}
		if err != nil {
			return nil, fmt.Errorf("GetUnstructured: failed to get underlying object: %w", err)
		}
		// Conversions are run in this loop:
		for _, f := range converters {
			transformed, err := f(obj)
			if err != nil {
				// If we return an error here, the upstream k8s library will retry a transform, and we don't want that,
				// as it's likely to loop forever and the server will hang.
				// Instead, log this error and try the remaining transform functions
				logrus.Errorf("error in transform: %v", err)
			}
			obj = transformed
		}
		return obj, nil
	}
}
