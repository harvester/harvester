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
	"github.com/rancher/lasso/pkg/cache/sql/informer"
	"github.com/rancher/lasso/pkg/cache/sql/partition"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
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

// ListOptions represents the query parameters that may be included in a list request.
type ListOptions struct {
	ChunkSize  int
	Resume     string
	Filters    []informer.OrFilter
	Sort       informer.Sort
	Pagination informer.Pagination
}

type Cache interface {
	// ListByOptions returns objects according to the specified list options and partitions.
	// Specifically:
	//   - an unstructured list of resources belonging to any of the specified partitions
	//   - the total number of resources (returned list might be a subset depending on pagination options in lo)
	//   - a continue token, if there are more pages after the returned one
	//   - an error instead of all of the above if anything went wrong
	ListByOptions(ctx context.Context, lo informer.ListOptions, partitions []partition.Partition, namespace string) (*unstructured.UnstructuredList, int, string, error)
}

// ParseQuery parses the query params of a request and returns a ListOptions.
func ParseQuery(apiOp *types.APIRequest, namespaceCache Cache) (informer.ListOptions, error) {
	opts := informer.ListOptions{}

	opts.ChunkSize = getLimit(apiOp)

	q := apiOp.Request.URL.Query()
	cont := q.Get(continueParam)
	opts.Resume = cont

	filterParams := q[filterParam]
	filterOpts := []informer.OrFilter{}
	for _, filters := range filterParams {
		orFilters := strings.Split(filters, orOp)
		orFilter := informer.OrFilter{}
		for _, filter := range orFilters {
			var op informer.Op
			if strings.Contains(filter, "!=") {
				op = "!="
			}
			filter := opReg.Split(filter, -1)
			if len(filter) != 2 {
				continue
			}
			usePartialMatch := !(strings.HasPrefix(filter[1], `'`) && strings.HasSuffix(filter[1], `'`))
			value := strings.TrimSuffix(strings.TrimPrefix(filter[1], "'"), "'")
			orFilter.Filters = append(orFilter.Filters, informer.Filter{Field: strings.Split(filter[0], "."), Match: value, Op: op, Partial: usePartialMatch})
		}
		filterOpts = append(filterOpts, orFilter)
	}
	opts.Filters = filterOpts

	sortOpts := informer.Sort{}
	sortKeys := q.Get(sortParam)
	if sortKeys != "" {
		sortParts := strings.SplitN(sortKeys, ",", 2)
		primaryField := sortParts[0]
		if primaryField != "" && primaryField[0] == '-' {
			sortOpts.PrimaryOrder = informer.DESC
			primaryField = primaryField[1:]
		}
		if primaryField != "" {
			sortOpts.PrimaryField = strings.Split(primaryField, ".")
		}
		if len(sortParts) > 1 {
			secondaryField := sortParts[1]
			if secondaryField != "" && secondaryField[0] == '-' {
				sortOpts.SecondaryOrder = informer.DESC
				secondaryField = secondaryField[1:]
			}
			if secondaryField != "" {
				sortOpts.SecondaryField = strings.Split(secondaryField, ".")
			}
		}
	}
	opts.Sort = sortOpts

	var err error
	pagination := informer.Pagination{}
	pagination.PageSize, err = strconv.Atoi(q.Get(pageSizeParam))
	if err != nil {
		pagination.PageSize = 0
	}
	pagination.Page, err = strconv.Atoi(q.Get(pageParam))
	if err != nil {
		pagination.Page = 1
	}
	opts.Pagination = pagination

	var op informer.Op
	projectsOrNamespaces := q.Get(projectsOrNamespacesVar)
	if projectsOrNamespaces == "" {
		projectsOrNamespaces = q.Get(projectsOrNamespacesVar + notOp)
		if projectsOrNamespaces != "" {
			op = informer.NotEq
		}
	}
	if projectsOrNamespaces != "" {
		projOrNSFilters, err := parseNamespaceOrProjectFilters(apiOp.Context(), projectsOrNamespaces, op, namespaceCache)
		if err != nil {
			return opts, err
		}
		if projOrNSFilters == nil {
			return opts, apierror.NewAPIError(validation.NotFound, fmt.Sprintf("could not find any namespacess named [%s] or namespaces belonging to project named [%s]", projectsOrNamespaces, projectsOrNamespaces))
		}
		if op == informer.NotEq {
			for _, filter := range projOrNSFilters {
				opts.Filters = append(opts.Filters, informer.OrFilter{Filters: []informer.Filter{filter}})
			}
		} else {
			opts.Filters = append(opts.Filters, informer.OrFilter{Filters: projOrNSFilters})
		}
	}

	return opts, nil
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

func parseNamespaceOrProjectFilters(ctx context.Context, projOrNS string, op informer.Op, namespaceInformer Cache) ([]informer.Filter, error) {
	var filters []informer.Filter
	for _, pn := range strings.Split(projOrNS, ",") {
		uList, _, _, err := namespaceInformer.ListByOptions(ctx, informer.ListOptions{
			Filters: []informer.OrFilter{
				{
					Filters: []informer.Filter{
						{
							Field: []string{"metadata", "name"},
							Match: pn,
							Op:    informer.Eq,
						},
						{
							Field: []string{"metadata", "labels[field.cattle.io/projectId]"},
							Match: pn,
							Op:    informer.Eq,
						},
					},
				},
			},
		}, []partition.Partition{{Passthrough: true}}, "")
		if err != nil {
			return filters, err
		}
		for _, item := range uList.Items {
			filters = append(filters, informer.Filter{
				Field:   []string{"metadata", "namespace"},
				Match:   item.GetName(),
				Op:      op,
				Partial: false,
			})
		}
		continue
	}

	return filters, nil
}
