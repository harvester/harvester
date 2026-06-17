package sqltypes

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Op string

const (
	Eq          Op = "="
	NotEq       Op = "!="
	Exists      Op = "Exists"
	NotExists   Op = "NotExists"
	In          Op = "In"
	NotIn       Op = "NotIn"
	Contains    Op = "Contains"
	NotContains Op = "NotContains"
	Lt          Op = "Lt"
	Gt          Op = "Gt"
)

// SortOrder represents whether the list should be ascending or descending.
type SortOrder int

const (
	// ASC stands for ascending order.
	ASC SortOrder = iota
	// DESC stands for descending (reverse) order.
	DESC
)

// ListOptions represents the query parameters that may be included in a list request.
type ListOptions struct {
	Filters               []OrFilter
	ProjectsOrNamespaces  OrFilter
	SortList              SortList
	SummaryFieldList      SummaryFieldList
	SummaryOnly           bool
	SummaryNamespaced     bool
	Pagination            Pagination
	IncludeAssociatedData bool
	Revision              string
}

// Filter represents a field to filter by.
// A subfield in an object is represented in a request query using . notation, e.g. 'metadata.name'.
// The subfield is internally represented as a slice, e.g. [metadata, name].
// Complex subfields need to be expressed with square brackets, as in `metadata.labels[example.com/moose]`,
// but are mapped to the string slice ["metadata", "labels", "example.com/moose"]
//
// If more than one value is given for the `Match` field, we do an "IN (<values>)" test
type Filter struct {
	Field   []string
	Matches []string
	Op      Op
	Partial bool
}

// OrFilter represents a set of possible fields to filter by, where an item may match any filter in the set to be included in the result.
type OrFilter struct {
	Filters []Filter
}

// Sort represents the criteria to sort on.
// The subfield to sort by is represented in a request query using . notation, e.g. 'metadata.name'.
// The subfield is internally represented as a slice, e.g. [metadata, name].
// The order is represented by prefixing the sort key by '-', e.g. sort=-metadata.name.
// e.g. To sort internal clusters first followed by clusters in alpha order: sort=-spec.internal,spec.displayName
type Sort struct {
	Fields   []string
	Order    SortOrder
	SortAsIP bool
}

type SortList struct {
	SortDirectives []Sort
}

type SummaryFieldList [][]string

// Pagination represents how to return paginated results.
type Pagination struct {
	PageSize int
	Page     int
}

type ExternalDependency struct {
	SourceGVK            string
	SourceFieldName      string
	TargetGVK            string
	TargetKeyFieldName   string
	TargetFinalFieldName string
}

type ExternalLabelDependency struct {
	SourceGVK string
	TargetGVK string

	SourceLabelTargetField map[string]string

	TargetFinalFieldName string

	generatedQuery string
}

// NewExternalLabelDependency pre-generates the query needed for the label and key pairs. No labels in
// ExternalLabelDependency are user supplied, making risk of potential SQL injection minimal.
func NewExternalLabelDependency(eld ExternalLabelDependency) (ExternalLabelDependency, error) {
	if len(eld.SourceLabelTargetField) == 0 {
		return ExternalLabelDependency{}, fmt.Errorf("ExternalLabelDependency must have at least 1 label and field pair")
	}

	type pair struct {
		sourceLabelName string
		targetFieldName string
	}

	pairs := make([]pair, 0, len(eld.SourceLabelTargetField))
	for sourceLabelName, targetFieldName := range eld.SourceLabelTargetField {
		pairs = append(pairs, pair{sourceLabelName: sourceLabelName, targetFieldName: targetFieldName})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].sourceLabelName == pairs[j].sourceLabelName {
			return pairs[i].targetFieldName < pairs[j].targetFieldName
		}

		return pairs[i].sourceLabelName < pairs[j].sourceLabelName
	})

	joinClauses := make([]string, 0, len(pairs))
	onClauses := make([]string, 0, len(pairs))
	whereClauses := make([]string, 0, len(pairs))

	for i, pair := range pairs {
		joinClauses = append(joinClauses, fmt.Sprintf(`LEFT OUTER JOIN "%s_labels" lt%d ON f.key = lt%d.key`, eld.SourceGVK, i, i))
		onClauses = append(onClauses, fmt.Sprintf(`lt%d.value = ex2."%s"`, i, pair.targetFieldName))
		whereClauses = append(whereClauses, fmt.Sprintf(`lt%d.label = "%s"`, i, pair.sourceLabelName))
	}

	eld.generatedQuery = fmt.Sprintf(`SELECT DISTINCT f.key, ex2."%s" FROM "%s_fields" f
	%s
	JOIN "%s_fields" ex2 ON (%s)
	WHERE (%s) AND f."%s" != ex2."%s"`,
		eld.TargetFinalFieldName,
		eld.SourceGVK,
		strings.Join(joinClauses, "\n\t"),
		eld.TargetGVK,
		strings.Join(onClauses, " AND "),
		strings.Join(whereClauses, " AND "),
		eld.TargetFinalFieldName,
		eld.TargetFinalFieldName,
	)

	return eld, nil
}

func MustNewExternalLabelDependency(eld ExternalLabelDependency) ExternalLabelDependency {
	eld, err := NewExternalLabelDependency(eld)
	if err != nil {
		panic(err)
	}

	return eld
}

func (d ExternalLabelDependency) Query() string {
	return d.generatedQuery
}

type ExternalGVKUpdates struct {
	AffectedGVK               schema.GroupVersionKind
	ExternalDependencies      []ExternalDependency
	ExternalLabelDependencies []ExternalLabelDependency
}

type ExternalGVKDependency map[schema.GroupVersionKind]*ExternalGVKUpdates

type ExternalInfoPacket struct {
	ExternalDependencies      []ExternalDependency
	ExternalLabelDependencies []ExternalLabelDependency
	ExternalGVKDependencies   ExternalGVKDependency
}

func NewSortList() *SortList {
	return &SortList{
		SortDirectives: []Sort{},
	}
}
