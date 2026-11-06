package dates

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"time"

	rescommon "github.com/rancher/steve/pkg/resources/common"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/jsonpath"
)

var Now = time.Now

// Converter handles the transformation of date/time columns in resources.
// There are two cases:
//
//  1. CRD Date Columns:
//     For CRDs, columns defined with type `date` are expected by the UI to contain a timestamp, not a duration. We
//     use the associated JSONPath to extract the raw value from the object. This ensures we
//     pass the correct timestamp format to the UI. We do it in the transformer so that we can
//     correctly sort / filter in the SQLite database.
//
//  2. Built-in Date Columns
//     Kubernetes often returns time fields (like "Age") as human-readable durations (e.g., "6h4m").
//     Storing this static string in the cache is problematic because it becomes stale immediately.
//     For example, if we store "6h4m", an hour later the resource is effectively "7h4m" old,
//     but the cache still serves "6h4m".
//
//     To fix this, we parse the duration string, subtract it from the current time to calculate
//     the approximate absolute timestamp, and store that timestamp (in Unix milliseconds).
//     The UI can then use this timestamp to display a live, accurate relative time (e.g., "7 hours ago").
type Converter struct {
	GVK       schema.GroupVersionKind
	Columns   []rescommon.ColumnDefinition
	IsCRD     bool
	JSONPaths map[string]*jsonpath.JSONPath
}

func (d *Converter) Transform(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	fields, got, err := unstructured.NestedSlice(obj.Object, "metadata", "fields")
	if err != nil || !got {
		return obj, err
	}

	updated := false

	for _, col := range d.Columns {
		if !d.isDateColumn(col) {
			continue
		}

		index := rescommon.GetIndexValueFromString(col.Field)
		if index == -1 {
			logrus.Warnf("field index not found at column.Field struct variable: %s", col.Field)
			continue
		}

		if index >= len(fields) || fields[index] == nil {
			continue
		}

		value, ok := fields[index].(string)
		if !ok {
			logrus.Errorf("could not cast metadata.fields[%d] to string, original value: <%v>", index, fields[index])
			continue
		}

		// Handle CRD date columns using JSONPath to get the raw value
		if d.IsCRD {
			if jp, ok := d.JSONPaths[col.Name]; ok {
				if v, ok := getValueFromJSONPath(obj.Object, jp); ok {
					value = v
				}

				if _, err := time.Parse(time.RFC3339, value); err == nil {
					if fields[index] != value {
						fields[index] = value
						updated = true
					}
					continue
				}
			}
		}

		// Handle duration strings (e.g. "5m", "1d") by converting to absolute timestamp
		duration, err := rescommon.ParseTimestampOrHumanReadableDuration(value)
		if err != nil {
			logrus.Errorf("parse timestamp %s, failed with error: %s", value, err)
			continue
		}

		fields[index] = fmt.Sprintf("%d", Now().Add(-duration).UnixMilli())
		updated = true
	}

	if updated {
		if err := unstructured.SetNestedSlice(obj.Object, fields, "metadata", "fields"); err != nil {
			return obj, err
		}
	}

	return obj, nil
}

func (d *Converter) isDateColumn(col rescommon.ColumnDefinition) bool {
	// Check for CRD date columns
	if d.IsCRD && col.Type == "date" {
		return true
	}
	// Check for built-in resource date columns
	return slices.Contains(rescommon.DateFieldsByGVK[d.GVK], col.Name)
}

func getValueFromJSONPath(obj map[string]any, jp *jsonpath.JSONPath) (string, bool) {
	results, err := jp.FindResults(obj)
	if err == nil && len(results) > 0 && len(results[0]) > 0 {
		v := results[0][0].Interface()
		if v != nil {
			buf := &bytes.Buffer{}
			if err := jp.PrintResults(buf, []reflect.Value{reflect.ValueOf(v)}); err == nil {
				return buf.String(), true
			}
		}
	}
	return "", false
}
