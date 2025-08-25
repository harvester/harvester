package common

import (
	"strconv"
	"strings"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/attributes"
)

// GetIndexValueFromString looks for values between [ ].
// e.g: $.metadata.fields[2], in this case it would return 2
// In case it doesn't find any value between brackets it returns -1
func GetIndexValueFromString(pathString string) int {
	idxStart := strings.Index(pathString, "[")
	if idxStart == -1 {
		return -1
	}
	idxEnd := strings.Index(pathString[idxStart+1:], "]")
	if idxEnd == -1 {
		return -1
	}
	idx, err := strconv.Atoi(pathString[idxStart+1 : idxStart+1+idxEnd])
	if err != nil {
		return -1
	}
	return idx
}

// GetColumnDefinitions returns ColumnDefinitions from an APISchema
func GetColumnDefinitions(schema *types.APISchema) []ColumnDefinition {
	columns := attributes.Columns(schema)
	if columns == nil {
		return nil
	}
	colDefs, ok := columns.([]ColumnDefinition)
	if !ok {
		return nil
	}
	return colDefs
}
