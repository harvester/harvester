// Package listprocessor contains methods for filtering, sorting, and paginating lists of objects.
package listprocessor

import (
	"context"
	"fmt"
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
func ParseQuery(apiOp *types.APIRequest, namespaceCache Cache) (sqltypes.ListOptions, error) {
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
	if sortKeys != "" {
		sortList := *sqltypes.NewSortList()
		sortParts := strings.Split(sortKeys, ",")
		for _, sortPart := range sortParts {
			field := sortPart
			if len(field) > 0 {
				sortOrder := sqltypes.ASC
				if field[0] == '-' {
					sortOrder = sqltypes.DESC
					field = field[1:]
				}
				if len(field) > 0 {
					sortDirective := sqltypes.Sort{
						Fields: queryhelper.SafeSplit(field),
						Order:  sortOrder,
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

	op := sqltypes.Eq
	projectsOrNamespaces := q.Get(projectsOrNamespacesVar)
	if projectsOrNamespaces == "" {
		projectsOrNamespaces = q.Get(projectsOrNamespacesVar + notOp)
		if projectsOrNamespaces != "" {
			op = sqltypes.NotEq
		}
	}
	if projectsOrNamespaces != "" {
		projOrNSFilters, err := parseNamespaceOrProjectFilters(apiOp.Context(), projectsOrNamespaces, op, namespaceCache)
		if err != nil {
			return opts, err
		}
		if projOrNSFilters == nil {
			return opts, apierror.NewAPIError(validation.NotFound, fmt.Sprintf("could not find any namespaces named [%s] or namespaces belonging to project named [%s]", projectsOrNamespaces, projectsOrNamespaces))
		}
		if op == sqltypes.NotEq {
			for _, filter := range projOrNSFilters {
				opts.Filters = append(opts.Filters, sqltypes.OrFilter{Filters: []sqltypes.Filter{filter}})
			}
		} else {
			opts.Filters = append(opts.Filters, sqltypes.OrFilter{Filters: projOrNSFilters})
		}
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

func parseNamespaceOrProjectFilters(ctx context.Context, projOrNS string, op sqltypes.Op, namespaceInformer Cache) ([]sqltypes.Filter, error) {
	var filters []sqltypes.Filter
	for _, pn := range strings.Split(projOrNS, ",") {
		uList, _, _, err := namespaceInformer.ListByOptions(ctx, &sqltypes.ListOptions{
			Filters: []sqltypes.OrFilter{
				{
					Filters: []sqltypes.Filter{
						{
							Field:   []string{"metadata", "name"},
							Matches: []string{pn},
							Op:      sqltypes.Eq,
						},
						{
							Field:   []string{"metadata", "labels", "field.cattle.io/projectId"},
							Matches: []string{pn},
							Op:      sqltypes.Eq,
						},
					},
				},
			},
		}, []partition.Partition{{Passthrough: true}}, "")
		if err != nil {
			return filters, err
		}
		for _, item := range uList.Items {
			filters = append(filters, sqltypes.Filter{
				Field:   []string{"metadata", "namespace"},
				Matches: []string{item.GetName()},
				Op:      op,
				Partial: false,
			})
		}
		continue
	}

	return filters, nil
}
