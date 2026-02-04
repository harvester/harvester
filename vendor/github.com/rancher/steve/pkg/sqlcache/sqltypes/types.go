package sqltypes

import "k8s.io/apimachinery/pkg/runtime/schema"

type Op string

const (
	Eq        Op = "="
	NotEq     Op = "!="
	Exists    Op = "Exists"
	NotExists Op = "NotExists"
	In        Op = "In"
	NotIn     Op = "NotIn"
	Lt        Op = "Lt"
	Gt        Op = "Gt"
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
	Filters              []OrFilter
	ProjectsOrNamespaces OrFilter
	SortList             SortList
	SummaryFieldList     SummaryFieldList
	Pagination           Pagination
	Revision             string
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
	SourceGVK            string
	SourceLabelName      string
	TargetGVK            string
	TargetKeyFieldName   string
	TargetFinalFieldName string
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
