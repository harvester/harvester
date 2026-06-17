// Package virtual provides functions/resources to define virtual fields (fields which don't exist in k8s
// but should be visible in the API) on resources
package virtual

import (
	"fmt"
	"time"

	rescommon "github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/steve/pkg/resources/virtual/clusters"
	"github.com/rancher/steve/pkg/resources/virtual/common"
	"github.com/rancher/steve/pkg/resources/virtual/dates"
	"github.com/rancher/steve/pkg/resources/virtual/events"
	"github.com/rancher/steve/pkg/resources/virtual/multivalue"
	"github.com/rancher/steve/pkg/resources/virtual/multivalue/parsers"
	"github.com/rancher/steve/pkg/resources/virtual/pods"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/jsonpath"
)

var now = time.Now

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

	// v1/Event
	if gvk == rescommon.EventGVK {
		converters = append(converters, events.TransformEventObject)

		// management.cattle.io/v3/Cluster
	} else if gvk == rescommon.MgmtClusterGVK {
		converters = append(converters, clusters.TransformManagedCluster)

		// v1/Pod
	} else if gvk == rescommon.PodGVK {
		converters = append(converters, pods.TransformPodObject)

		// Register multi-value converter
		multiValueConverter := &multivalue.Converter{
			Columns: columns,
			Fields: []multivalue.FieldConfig{
				{
					ColumnName: "Restarts",
					ParseFunc:  parsers.ParseRestarts,
				},
			},
		}
		converters = append(converters, multiValueConverter.Transform)
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
				logrus.Errorf("error in transform for gvk Kind:%s Group:%s Version:%s, error: %v", gvk.Kind, gvk.Group, gvk.Version, err)
			}
			obj = transformed
		}
		return obj, nil
	}
}
