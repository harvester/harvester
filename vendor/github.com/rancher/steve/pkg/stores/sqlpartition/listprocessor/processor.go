// Package listprocessor contains methods for filtering, sorting, and paginating lists of objects.
package listprocessor

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/sqlcache/partition"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"github.com/rancher/steve/pkg/stores/queryhelper"
	"github.com/rancher/steve/pkg/stores/sqlpartition/queryparser"
	"github.com/rancher/steve/pkg/stores/sqlpartition/selection"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	defaultLimit            = 100000
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

var endsWithBracket = regexp.MustCompile(`^(.+)\[(.+)]$`)
var mapK8sOpToRancherOp = map[selection.Operator]sqltypes.Op{
	selection.Equals:           sqltypes.Eq,
	selection.DoubleEquals:     sqltypes.Eq,
	selection.PartialEquals:    sqltypes.Eq,
	selection.NotEquals:        sqltypes.NotEq,
	selection.NotPartialEquals: sqltypes.NotEq,
	selection.In:               sqltypes.In,
	selection.NotIn:            sqltypes.NotIn,
	selection.Exists:           sqltypes.Exists,
	selection.DoesNotExist:     sqltypes.NotExists,
	selection.LessThan:         sqltypes.Lt,
	selection.GreaterThan:      sqltypes.Gt,
}

type Cache interface {
	// ListByOptions returns objects according to the specified list options and partitions.
	// Specifically:
	//   - an unstructured list of resources belonging to any of the specified partitions
	//   - the total number of resources (returned list might be a subset depending on pagination options in lo)
	//   - a continue token, if there are more pages after the returned one
	//   - an error instead of all of the above if anything went wrong
	ListByOptions(ctx context.Context, lo *sqltypes.ListOptions, partitions []partition.Partition, namespace string) (*unstructured.UnstructuredList, int, string, error)
}

func k8sOpToRancherOp(k8sOp selection.Operator) (sqltypes.Op, bool, error) {
	v, ok := mapK8sOpToRancherOp[k8sOp]
	if ok {
		return v, k8sOp == selection.PartialEquals || k8sOp == selection.NotPartialEquals, nil
	}
	return "", false, fmt.Errorf("unknown k8sOp: %s", k8sOp)
}

func k8sRequirementToOrFilter(requirement queryparser.Requirement) (sqltypes.Filter, error) {
	values := requirement.Values()
	queryFields := splitQuery(requirement.Key())
	op, usePartialMatch, err := k8sOpToRancherOp(requirement.Operator())
	return sqltypes.Filter{
		Field:   queryFields,
		Matches: values,
		Op:      op,
		Partial: usePartialMatch,
	}, err
}

// ParseQuery parses the query params of a request and returns a ListOptions.
func ParseQuery(apiOp *types.APIRequest, gvKind string) (sqltypes.ListOptions, error) {
	opts := sqltypes.ListOptions{}

	q := apiOp.Request.URL.Query()

	filterParams := q[filterParam]
	filterOpts := []sqltypes.OrFilter{}
	for _, filters := range filterParams {
		requirements, err := queryparser.ParseToRequirements(filters)
		if err != nil {
			return sqltypes.ListOptions{}, err
		}
		orFilter := sqltypes.OrFilter{}
		for _, requirement := range requirements {
			filter, err := k8sRequirementToOrFilter(requirement)
			if err != nil {
				return opts, err
			}
			orFilter.Filters = append(orFilter.Filters, filter)
		}
		filterOpts = append(filterOpts, orFilter)
	}
	opts.Filters = filterOpts

	sortKeys := q.Get(sortParam)
	callsIPFunctionRegex := regexp.MustCompile(`^ip\(.+\)$`)
	if sortKeys != "" {
		sortList := *sqltypes.NewSortList()
		sortParts := strings.Split(sortKeys, ",")
		for _, sortPart := range sortParts {
			field := sortPart
			sortAsIP := false
			if callsIPFunctionRegex.MatchString(sortPart) {
				field = sortPart[3 : len(sortPart)-1]
				sortAsIP = true
			}
			if len(field) > 0 {
				sortOrder := sqltypes.ASC
				if field[0] == '-' {
					sortOrder = sqltypes.DESC
					field = field[1:]
				}
				if len(field) > 0 {
					sortDirective := sqltypes.Sort{
						Fields:   queryhelper.SafeSplit(field),
						Order:    sortOrder,
						SortAsIP: sortAsIP,
					}
					sortList.SortDirectives = append(sortList.SortDirectives, sortDirective)
				}
			}
		}
		opts.SortList = sortList
	}

	var err error
	pagination := sqltypes.Pagination{}
	pagination.PageSize, err = strconv.Atoi(q.Get(pageSizeParam))
	if err != nil {
		pagination.PageSize = 0
	}
	pagination.Page, err = strconv.Atoi(q.Get(pageParam))
	if err != nil {
		pagination.Page = 1
	}
	opts.Pagination = pagination

	op := sqltypes.In
	projectsOrNamespaces := q.Get(projectsOrNamespacesVar)
	if projectsOrNamespaces == "" {
		projectsOrNamespaces = q.Get(projectsOrNamespacesVar + notOp)
		if projectsOrNamespaces != "" {
			op = sqltypes.NotIn
		}
	}
	if projectsOrNamespaces != "" {
		if gvKind == "Namespace" {
			projNSFilter := parseNamespaceOrProjectFilters(projectsOrNamespaces, op)
			if len(projNSFilter.Filters) == 2 {
				if op == sqltypes.In {
					opts.Filters = append(opts.Filters, projNSFilter)
				} else {
					opts.Filters = append(opts.Filters, sqltypes.OrFilter{Filters: []sqltypes.Filter{projNSFilter.Filters[0]}})
					opts.Filters = append(opts.Filters, sqltypes.OrFilter{Filters: []sqltypes.Filter{projNSFilter.Filters[1]}})
				}
			} else if len(projNSFilter.Filters) == 0 {
				// do nothing
			} else {
				logrus.Infof("Ignoring unexpected filter for query %q: parseNamespaceOrProjectFilters returned %d filters, expecting 2", q, len(projNSFilter.Filters))
			}
		} else {
			opts.ProjectsOrNamespaces = parseNamespaceOrProjectFilters(projectsOrNamespaces, op)
		}
	}

	revision := q.Get(revisionParam)
	if revision != "" {
		if _, err := strconv.ParseInt(revision, 10, 64); err != nil {
			return opts, apierror.NewAPIError(validation.ErrorCode{Code: "invalid revision query param", Status: http.StatusBadRequest},
				fmt.Sprintf("value %s for revision query param is not valid", revision))
		}
		opts.Revision = revision
	}

	return opts, nil
}

// splitQuery takes a single-string k8s object accessor and returns its separate fields in a slice.
// "Simple" accessors of the form `metadata.labels.foo` => ["metadata", "labels", "foo"]
// but accessors with square brackets need to be broken on the brackets, as in
// "metadata.annotations[k8s.io/this-is-fun]" => ["metadata", "annotations", "k8s.io/this-is-fun"]
// We assume in the kubernetes/rancher world json keys are always alphanumeric-underscorish, so
// we only look for square brackets at the end of the string.
func splitQuery(query string) []string {
	m := endsWithBracket.FindStringSubmatch(query)
	if m != nil {
		return append(strings.Split(m[1], "."), m[2])
	}
	return strings.Split(query, ".")
}

func parseNamespaceOrProjectFilters(projOrNS string, op sqltypes.Op) sqltypes.OrFilter {
	var filters []sqltypes.Filter
	projOrNs := strings.Split(projOrNS, ",")
	if len(projOrNs) > 0 {
		filters = []sqltypes.Filter{
			sqltypes.Filter{
				Field:   []string{"metadata", "name"},
				Matches: projOrNs,
				Op:      op,
			},
			sqltypes.Filter{
				Field:   []string{"metadata", "labels", projectIDFieldLabel},
				Matches: projOrNs,
				Op:      op,
			},
		}
	}
	return sqltypes.OrFilter{Filters: filters}
}
