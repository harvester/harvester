package informer

import (
	"regexp"
	"strconv"
	"time"

	rescommon "github.com/rancher/steve/pkg/resources/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// IndexedField represents a single field that can be indexed in the SQL cache.
// It provides the column name, SQL type, and value extraction logic.
type IndexedField interface {
	// ColumnName returns the column name.
	ColumnName() string

	// ColumnType returns the SQL type for this column.
	ColumnType() string

	// GetValue extracts the value from an unstructured object.
	// Returns nil for missing/invalid data.
	GetValue(obj *unstructured.Unstructured) (any, error)
}

// JSONPathField represents a standard field accessed via JSON path
type JSONPathField struct {
	Path []string
	Type string // Optional: TEXT (default), INTEGER, REAL, etc.
}

func (f *JSONPathField) ColumnName() string {
	return toColumnName(f.Path)
}

func (f *JSONPathField) ColumnType() string {
	sqlType := f.Type
	if sqlType == "" {
		sqlType = "TEXT"
	}
	return sqlType
}

func (f *JSONPathField) GetValue(obj *unstructured.Unstructured) (any, error) {
	col := toColumnName(f.Path)
	value, err := getField(obj, col)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// ComputedField represents a field with custom column name and value extraction
type ComputedField struct {
	Name         string
	Type         string
	GetValueFunc func(obj *unstructured.Unstructured) (any, error)
	IsTimestamp  bool
}

func (f *ComputedField) ColumnName() string {
	return f.Name
}

func (f *ComputedField) ColumnType() string {
	return f.Type
}

func (f *ComputedField) GetValue(obj *unstructured.Unstructured) (any, error) {
	return f.GetValueFunc(obj)
}

// restartsPattern matches "4 (3h38m ago)" or just "0"
var restartsPattern = regexp.MustCompile(`^(\d+)(?:\s+\((.+?)\s+ago\))?$`)

// timeNow is mockable for testing
var timeNow = time.Now

// podRestartFieldIndex is the index of the restart field in Pod metadata.fields
const podRestartFieldIndex = 3

// ExtractPodRestartCount extracts restart count from Pod metadata.fields[3].
func ExtractPodRestartCount(obj *unstructured.Unstructured) (any, error) {
	value, found, err := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "fields")
	if err != nil || !found {
		return int64(0), nil
	}

	fields, ok := value.([]interface{})
	if !ok || len(fields) <= podRestartFieldIndex {
		return int64(0), nil
	}

	restartField := fields[podRestartFieldIndex]

	if arr, ok := restartField.([]interface{}); ok {
		return toInt64(arr, 0), nil
	}

	if str, ok := restartField.(string); ok {
		count, _ := parseRestarts(str)
		return count, nil
	}

	return int64(0), nil
}

// ExtractPodRestartTimestamp extracts restart timestamp from Pod metadata.fields[3].
// Returns the Unix time in milliseconds of the last restart.
func ExtractPodRestartTimestamp(obj *unstructured.Unstructured) (any, error) {
	value, found, err := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "fields")
	if err != nil || !found {
		return int64(0), nil
	}

	fields, ok := value.([]interface{})
	if !ok || len(fields) <= podRestartFieldIndex {
		return int64(0), nil
	}

	restartField := fields[podRestartFieldIndex]

	if arr, ok := restartField.([]interface{}); ok {
		return toInt64(arr, 1), nil
	}

	if str, ok := restartField.(string); ok {
		_, timestamp := parseRestarts(str)
		return timestamp, nil
	}

	return int64(0), nil
}

func toInt64(arr []interface{}, idx int) int64 {
	if idx >= len(arr) || arr[idx] == nil {
		return 0
	}
	switch v := arr[idx].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

// parseRestarts parses pod restart values like "4 (3h38m ago)" into (count, timestamp_ms)
func parseRestarts(value string) (int64, int64) {
	matches := restartsPattern.FindStringSubmatch(value)
	if matches == nil {
		return 0, 0
	}

	count, _ := strconv.ParseInt(matches[1], 10, 64)

	var timestamp int64
	if matches[2] != "" {
		dur, err := rescommon.ParseHumanReadableDuration(matches[2])
		if err == nil {
			timestamp = timeNow().Add(-dur).UnixMilli()
		}
	}

	return count, timestamp
}
