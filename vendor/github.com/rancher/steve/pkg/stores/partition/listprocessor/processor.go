// Package listprocessor contains methods for filtering, sorting, and paginating lists of objects.
package listprocessor

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/stores/queryhelper"
	"github.com/rancher/wrangler/v3/pkg/data"
	"github.com/rancher/wrangler/v3/pkg/data/convert"
	corecontrollers "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	defaultLimit            = 100000
	continueParam           = "continue"
	limitParam              = "limit"
	filterParam             = "filter"
	sortParam               = "sort"
	pageSizeParam           = "pagesize"
	pageParam               = "page"
	revisionParam           = "revision"
	projectsOrNamespacesVar = "projectsornamespaces"
	projectIDFieldLabel     = "field.cattle.io/projectId"

	orOp  = ","
	notOp = "!"
)

var opReg = regexp.MustCompile(`[!]?=`)

type op string

const (
	eq    op = ""
	notEq op = "!="
)

// ListOptions represents the query parameters that may be included in a list request.
type ListOptions struct {
	ChunkSize            int
	Resume               string
	Filters              []OrFilter
	Sort                 Sort
	Pagination           Pagination
	Revision             string
	ProjectsOrNamespaces ProjectsOrNamespacesFilter
}

// Filter represents a field to filter by.
// A subfield in an object is represented in a request query using . notation, e.g. 'metadata.name'.
// The subfield is internally represented as a slice, e.g. [metadata, name].
type Filter struct {
	field []string
	match string
	op    op
}

// String returns the filter as a query string.
func (f Filter) String() string {
	field := strings.Join(f.field, ".")
	return field + "=" + f.match
}

// OrFilter represents a set of possible fields to filter by, where an item may match any filter in the set to be included in the result.
type OrFilter struct {
	filters []Filter
}

// String returns the filter as a query string.
func (f OrFilter) String() string {
	var fields strings.Builder
	for i, field := range f.filters {
		fields.WriteString(strings.Join(field.field, "."))
		fields.WriteByte('=')
		fields.WriteString(field.match)
		if i < len(f.filters)-1 {
			fields.WriteByte(',')
		}
	}
	return fields.String()
}

// SortOrder represents whether the list should be ascending or descending.
type SortOrder int

const (
	// ASC stands for ascending order.
	ASC SortOrder = iota
	// DESC stands for descending (reverse) order.
	DESC
)

// Sort represents the criteria to sort on.
// The subfield to sort by is represented in a request query using . notation, e.g. 'metadata.name'.
// The subfield is internally represented as a slice, e.g. [metadata, name].
// The order is represented by prefixing the sort key by '-', e.g. sort=-metadata.name.
type Sort struct {
	Fields [][]string
	Orders []SortOrder
}

// String returns the sort parameters as a query string.
func (s Sort) String() string {
	nonIdentifierField := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	fields := make([]string, len(s.Fields))
	for i, field := range s.Fields {
		lastIndex := len(field) - 1
		newField := strings.Join(field[0:lastIndex], ".")
		if nonIdentifierField.MatchString(field[lastIndex]) {
			// label keys may contain non-identifier characters `/`, `.` and `-`
			newField += fmt.Sprintf("[%s]", field[lastIndex])
		} else {
			newField += fmt.Sprintf(".%s", field[lastIndex])
		}
		if s.Orders[i] == DESC {
			newField = fmt.Sprintf("-%s", newField)
		}
		fields[i] = newField
	}
	return strings.Join(fields, ",")
}

// Pagination represents how to return paginated results.
type Pagination struct {
	pageSize int
	page     int
}

// PageSize returns the integer page size.
func (p Pagination) PageSize() int {
	return p.pageSize
}

type ProjectsOrNamespacesFilter struct {
	filter map[string]struct{}
	op     op
}

// ParseQuery parses the query params of a request and returns a ListOptions.
func ParseQuery(apiOp *types.APIRequest) *ListOptions {
	opts := ListOptions{}

	opts.ChunkSize = getLimit(apiOp)

	q := apiOp.Request.URL.Query()
	cont := q.Get(continueParam)
	opts.Resume = cont

	filterParams := q[filterParam]
	filterOpts := []OrFilter{}
	for _, filters := range filterParams {
		orFilters := strings.Split(filters, orOp)
		orFilter := OrFilter{}
		for _, filter := range orFilters {
			var op op
			if strings.Contains(filter, "!=") {
				op = "!="
			}
			filter := opReg.Split(filter, -1)
			if len(filter) != 2 {
				continue
			}
			orFilter.filters = append(orFilter.filters, Filter{field: strings.Split(filter[0], "."), match: filter[1], op: op})
		}
		filterOpts = append(filterOpts, orFilter)
	}
	// sort the filter fields so they can be used as a cache key in the store
	for _, orFilter := range filterOpts {
		sort.Slice(orFilter.filters, func(i, j int) bool {
			fieldI := strings.Join(orFilter.filters[i].field, ".")
			fieldJ := strings.Join(orFilter.filters[j].field, ".")
			return fieldI < fieldJ
		})
	}
	sort.Slice(filterOpts, func(i, j int) bool {
		var fieldI, fieldJ strings.Builder
		for _, f := range filterOpts[i].filters {
			fieldI.WriteString(strings.Join(f.field, "."))
		}
		for _, f := range filterOpts[j].filters {
			fieldJ.WriteString(strings.Join(f.field, "."))
		}
		return fieldI.String() < fieldJ.String()
	})
	opts.Filters = filterOpts

	sortOpts := Sort{}
	sortKeys := q.Get(sortParam)
	if sortKeys != "" {
		sortParts := strings.Split(sortKeys, ",")
		for _, field := range sortParts {
			sortOrder := ASC
			if len(field) > 0 {
				if field[0] == '-' {
					sortOrder = DESC
					field = field[1:]
				}
			}
			if len(field) == 0 {
				// Old semantics: if we have a sort query like
				// sort=,metadata.someOtherField
				// we *DON'T sort
				// Fixed: skip empty-strings.
				continue
			}
			sortOpts.Fields = append(sortOpts.Fields, queryhelper.SafeSplit(field))
			sortOpts.Orders = append(sortOpts.Orders, sortOrder)
		}
	}
	opts.Sort = sortOpts

	var err error
	pagination := Pagination{}
	pagination.pageSize, err = strconv.Atoi(q.Get(pageSizeParam))
	if err != nil {
		pagination.pageSize = 0
	}
	pagination.page, err = strconv.Atoi(q.Get(pageParam))
	if err != nil {
		pagination.page = 1
	}
	opts.Pagination = pagination

	revision := q.Get(revisionParam)
	opts.Revision = revision

	projectsOptions := ProjectsOrNamespacesFilter{}
	var op op
	projectsOrNamespaces := q.Get(projectsOrNamespacesVar)
	if projectsOrNamespaces == "" {
		projectsOrNamespaces = q.Get(projectsOrNamespacesVar + notOp)
		if projectsOrNamespaces != "" {
			op = notEq
		}
	}
	if projectsOrNamespaces != "" {
		projectsOptions.filter = make(map[string]struct{})
		for _, pn := range strings.Split(projectsOrNamespaces, ",") {
			projectsOptions.filter[pn] = struct{}{}
		}
		projectsOptions.op = op
		opts.ProjectsOrNamespaces = projectsOptions
	}
	return &opts
}

// getLimit extracts the limit parameter from the request or sets a default of 100000.
// The default limit can be explicitly disabled by setting it to zero or negative.
// If the default is accepted, clients must be aware that the list may be incomplete, and use the "continue" token to get the next chunk of results.
func getLimit(apiOp *types.APIRequest) int {
	limitString := apiOp.Request.URL.Query().Get(limitParam)
	limit, err := strconv.Atoi(limitString)
	if err != nil {
		limit = defaultLimit
	}
	return limit
}

// FilterList accepts a channel of unstructured objects and a slice of filters and returns the filtered list.
// Filters are ANDed together.
func FilterList(list <-chan []unstructured.Unstructured, filters []OrFilter) []unstructured.Unstructured {
	result := []unstructured.Unstructured{}
	for items := range list {
		for _, item := range items {
			if len(filters) == 0 {
				result = append(result, item)
				continue
			}
			if matchesAll(item.Object, filters) {
				result = append(result, item)
			}
		}
	}
	return result
}

func matchesOne(obj map[string]interface{}, filter Filter) bool {
	var objValue interface{}
	var ok bool
	subField := []string{}
	for !ok && len(filter.field) > 0 {
		objValue, ok = data.GetValue(obj, filter.field...)
		if !ok {
			subField = append(subField, filter.field[len(filter.field)-1])
			filter.field = filter.field[:len(filter.field)-1]
		}
	}
	if !ok {
		return false
	}
	switch typedVal := objValue.(type) {
	case string, int, bool:
		if len(subField) > 0 {
			return false
		}
		stringVal := convert.ToString(typedVal)
		if strings.Contains(stringVal, filter.match) {
			return true
		}
	case []interface{}:
		filter = Filter{field: subField, match: filter.match, op: filter.op}
		if matchesOneInList(typedVal, filter) {
			return true
		}
	}
	return false
}

func matchesOneInList(obj []interface{}, filter Filter) bool {
	for _, v := range obj {
		switch typedItem := v.(type) {
		case string, int, bool:
			stringVal := convert.ToString(typedItem)
			if strings.Contains(stringVal, filter.match) {
				return true
			}
		case map[string]interface{}:
			if matchesOne(typedItem, filter) {
				return true
			}
		case []interface{}:
			if matchesOneInList(typedItem, filter) {
				return true
			}
		}
	}
	return false
}

func matchesAny(obj map[string]interface{}, filter OrFilter) bool {
	for _, f := range filter.filters {
		matches := matchesOne(obj, f)
		if (matches && f.op == eq) || (!matches && f.op == notEq) {
			return true
		}
	}
	return false
}

func matchesAll(obj map[string]interface{}, filters []OrFilter) bool {
	for _, f := range filters {
		if !matchesAny(obj, f) {
			return false
		}
	}
	return true
}

// SortList sorts the slice by the provided sort criteria.
func SortList(list []unstructured.Unstructured, s Sort) []unstructured.Unstructured {
	if len(s.Fields) == 0 {
		return list
	}
	sort.Slice(list, func(i, j int) bool {
		leftNode := list[i].Object
		rightNode := list[j].Object
		for i, field := range s.Fields {
			leftValue := convert.ToString(data.GetValueN(leftNode, field...))
			rightValue := convert.ToString(data.GetValueN(rightNode, field...))
			if leftValue != rightValue {
				if s.Orders[i] == ASC {
					return leftValue < rightValue
				}
				return rightValue < leftValue
			}
		}
		return false
	})
	return list
}

// PaginateList returns a subset of the result based on the pagination criteria as well as the total number of pages the caller can expect.
func PaginateList(list []unstructured.Unstructured, p Pagination) ([]unstructured.Unstructured, int) {
	if p.pageSize <= 0 {
		return list, 0
	}
	page := p.page - 1
	if p.page < 1 {
		page = 0
	}
	pages := len(list) / p.pageSize
	if len(list)%p.pageSize != 0 {
		pages++
	}
	offset := p.pageSize * page
	if offset > len(list) {
		return []unstructured.Unstructured{}, pages
	}
	if offset+p.pageSize > len(list) {
		return list[offset:], pages
	}
	return list[offset : offset+p.pageSize], pages
}

func FilterByProjectsAndNamespaces(list []unstructured.Unstructured, projectsOrNamespaces ProjectsOrNamespacesFilter, namespaceCache corecontrollers.NamespaceCache) []unstructured.Unstructured {
	if len(projectsOrNamespaces.filter) == 0 {
		return list
	}
	result := []unstructured.Unstructured{}
	for _, obj := range list {
		namespaceName := obj.GetNamespace()
		if namespaceName == "" {
			continue
		}
		namespace, err := namespaceCache.Get(namespaceName)
		if namespace == nil || err != nil {
			continue
		}
		projectLabel, _ := namespace.GetLabels()[projectIDFieldLabel]
		_, matchesProject := projectsOrNamespaces.filter[projectLabel]
		_, matchesNamespace := projectsOrNamespaces.filter[namespaceName]
		matches := matchesProject || matchesNamespace
		if projectsOrNamespaces.op == eq && matches {
			result = append(result, obj)
		}
		if projectsOrNamespaces.op == notEq && !matches {
			result = append(result, obj)
		}
	}
	return result
}
