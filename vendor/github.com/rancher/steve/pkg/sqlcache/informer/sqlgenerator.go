package informer

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/partition"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
)

type filterComponentsT struct {
	withParts       []withPart
	joinParts       []joinPart
	whereClauses    []string
	orderByClauses  []string
	limitClause     string
	limitParam      int
	offsetClause    string
	offsetParam     int
	params          []any
	queryUsesLabels bool
	isEmpty         bool
}

type joinPart struct {
	joinCommand    string
	isView         bool
	tableName      string
	tableNameAlias string
	onPrefix       string
	onField        string
	otherPrefix    string
	otherField     string
}

type withPart struct {
	labelName       string
	mainFieldPrefix string
	labelIndex      int
	labelPrefix     string
}

type summaryInfo struct {
	Property string         `json:"property"`
	Counts   map[string]any `json:"counts"`
}

const (
	escapeBackslashDirective = ` ESCAPE '\'` // The leading space is crucial for unit tests only '
	matchFmt                 = `%%%s%%`
	strictMatchFmt           = `%s`
)

var (
	containsNonNumericRegex = regexp.MustCompile(`\D`)
	failedToGetFromSliceFmt = "[listoption indexer] failed to get subfield [%s] from slice items"
	namespacesDbName        = "_v1_Namespace"
	projectIDFieldLabel     = "field.cattle.io/projectId"
	subfieldRegex           = regexp.MustCompile(`([a-zA-Z]+)|(\[[-a-zA-Z./]+])|(\[[0-9]+])`)

	ErrInvalidColumn   = errors.New("supplied column is invalid")
	ErrUnknownRevision = errors.New("unknown revision")
)

// Internal sql-generation methods on ListOptionIndexer in alphabetical order

// buildORClause creates an SQLite compatible query that ORs conditions built from passed filters
func (l *ListOptionIndexer) buildORClauseFromFilters(orFilters sqltypes.OrFilter, dbName string, mainFieldPrefix string, isSummaryFilter bool, joinTableIndexByLabelName map[string]int) (string, []any, error) {
	var params []any
	clauses := make([]string, 0, len(orFilters.Filters))
	var newParams []any
	var newClause string
	var err error

	for _, filter := range orFilters.Filters {
		if isLabelFilter(&filter) {
			var index int
			index, err = internLabel(filter.Field[2], joinTableIndexByLabelName, -1)
			if err != nil {
				return "", nil, err
			}
			newClause, newParams, err = l.getLabelFilter(index, filter, mainFieldPrefix, isSummaryFilter, dbName)
		} else {
			newClause, newParams, err = l.getFieldFilter(filter, mainFieldPrefix)
		}
		if err != nil {
			return "", nil, err
		}
		clauses = append(clauses, newClause)
		params = append(params, newParams...)
	}
	switch len(clauses) {
	case 0:
		return "", params, nil
	case 1:
		return clauses[0], params, nil
	}
	return fmt.Sprintf("(%s)", strings.Join(clauses, ") OR (")), params, nil
}

func (l *ListOptionIndexer) buildClauseFromProjectsOrNamespaces(orFilters sqltypes.OrFilter, dbName string, joinTableIndexByLabelName map[string]int) (string, []any, error) {
	var params []any
	var newParams []any
	var newClause string
	var err error
	var index int

	if len(orFilters.Filters) == 0 {
		return "", params, nil
	}

	clauses := make([]string, 0, len(orFilters.Filters))
	for _, filter := range orFilters.Filters {
		if isLabelFilter(&filter) {
			if index, err = internLabel(filter.Field[2], joinTableIndexByLabelName, -1); err != nil {
				return "", nil, err
			}
			newClause, newParams, err = l.getProjectsOrNamespacesLabelFilter(index, filter, dbName)
		} else {
			newClause, newParams, err = l.getProjectsOrNamespacesFieldFilter(filter)
		}
		if err != nil {
			return "", nil, err
		}
		clauses = append(clauses, newClause)
		params = append(params, newParams...)
	}

	if orFilters.Filters[0].Op == sqltypes.In {
		return fmt.Sprintf("(%s)", strings.Join(clauses, ") OR (")), params, nil
	}

	if orFilters.Filters[0].Op == sqltypes.NotIn {
		return fmt.Sprintf("(%s)", strings.Join(clauses, ") AND (")), params, nil
	}

	return "", nil, fmt.Errorf("project or namespaces supports only 'IN' or 'NOT IN' operation. op: %s is not valid",
		orFilters.Filters[0].Op)
}

func (l *ListOptionIndexer) checkRevision(lo *sqltypes.ListOptions) error {
	l.lock.RLock()
	latestRV := l.latestRV
	l.lock.RUnlock()

	if len(lo.Revision) > 0 {
		currentRevision, err := strconv.ParseInt(latestRV, 10, 64)
		if err != nil {
			return err
		}

		requestRevision, err := strconv.ParseInt(lo.Revision, 10, 64)
		if err != nil {
			return err
		}

		if currentRevision < requestRevision {
			return ErrUnknownRevision
		}
	}
	return nil
}

func (l *ListOptionIndexer) compileQuery(lo *sqltypes.ListOptions,
	partitions []partition.Partition,
	namespace string,
	dbName string,
	mainFieldPrefix string, // usually "f" for full-data queries, "f1" for summaries
	joinTableIndexByLabelName map[string]int,
	includeSort bool,
	isSummaryFilter bool) (*filterComponentsT, error) {

	var unboundSortLabels []string
	filterComponents := filterComponentsT{
		joinParts:    make([]joinPart, 0),
		whereClauses: make([]string, 0),
		params:       make([]any, 0),
		isEmpty:      true,
	}
	if !includeSort && lo.Pagination.PageSize > 0 {
		// If we want a slice of the data we need to sort it so we group on the correct slice
		includeSort = true
	}
	if includeSort {
		unboundSortLabels = getUnboundSortLabels(lo)
	}
	queryUsesLabels := hasLabelFilter(lo.Filters) || len(lo.ProjectsOrNamespaces.Filters) > 0

	if len(unboundSortLabels) > 0 {
		var err error
		filterComponents.withParts, err = getWithPartsForCompiling(unboundSortLabels, joinTableIndexByLabelName, mainFieldPrefix)
		if err != nil {
			return nil, err
		}

		for _, wp := range filterComponents.withParts {
			filterComponents.joinParts = append(filterComponents.joinParts,
				joinPart{joinCommand: "LEFT OUTER JOIN",
					isView:         true,
					tableNameAlias: wp.labelPrefix,
					onPrefix:       mainFieldPrefix,
					onField:        "key",
					otherPrefix:    wp.labelPrefix,
					otherField:     "key",
				})
		}
		filterComponents.isEmpty = false
		filterComponents.queryUsesLabels = true
	}
	if queryUsesLabels {
		for _, orFilter := range lo.Filters {
			for _, filter := range orFilter.Filters {
				if isLabelFilter(&filter) {
					labelName := filter.Field[2]
					_, ok := joinTableIndexByLabelName[labelName]
					if !ok {
						// Make the lt index 1-based for readability
						jtIndex := len(joinTableIndexByLabelName) + 1
						joinTableIndexByLabelName[labelName] = jtIndex
						jtPrefix := fmt.Sprintf("lt%d", jtIndex)
						filterComponents.joinParts = append(filterComponents.joinParts,
							joinPart{joinCommand: "LEFT OUTER JOIN",
								tableName:      fmt.Sprintf("%s_labels", dbName),
								tableNameAlias: jtPrefix,
								onPrefix:       mainFieldPrefix,
								onField:        "key",
								otherPrefix:    jtPrefix,
								otherField:     "key",
							})
						filterComponents.isEmpty = false
					}
				}
			}
		}
		filterComponents.queryUsesLabels = true
	}

	if len(lo.ProjectsOrNamespaces.Filters) > 0 {
		jtIndex := len(joinTableIndexByLabelName) + 1
		i, exists := joinTableIndexByLabelName[projectIDFieldLabel]
		if !exists {
			joinTableIndexByLabelName[projectIDFieldLabel] = jtIndex
		} else {
			jtIndex = i
		}
		jtPrefix := fmt.Sprintf("lt%d", jtIndex)
		filterComponents.joinParts = append(filterComponents.joinParts, joinPart{
			joinCommand:    "LEFT OUTER JOIN",
			tableName:      namespacesDbName + "_fields",
			tableNameAlias: "nsf",
			onPrefix:       mainFieldPrefix,
			onField:        `"metadata.namespace"`,
			otherPrefix:    "nsf",
			otherField:     `"metadata.name"`})
		filterComponents.joinParts = append(filterComponents.joinParts, joinPart{
			joinCommand:    "LEFT OUTER JOIN",
			tableName:      fmt.Sprintf("%s_labels", namespacesDbName),
			tableNameAlias: jtPrefix,
			onPrefix:       "nsf",
			onField:        "key",
			otherPrefix:    jtPrefix,
			otherField:     "key"})
		filterComponents.isEmpty = false
	}

	// 2- Filtering: WHERE clauses (from lo.Filters)
	for _, orFilters := range lo.Filters {
		orClause, orParams, err := l.buildORClauseFromFilters(orFilters, dbName, mainFieldPrefix, isSummaryFilter, joinTableIndexByLabelName)
		if err != nil {
			return nil, err
		}
		if orClause == "" {
			continue
		}
		filterComponents.whereClauses = append(filterComponents.whereClauses, orClause)
		filterComponents.params = append(filterComponents.params, orParams...)
		filterComponents.isEmpty = false
	}

	// WHERE clauses (from lo.ProjectsOrNamespaces)
	if len(lo.ProjectsOrNamespaces.Filters) > 0 {
		projOrNsClause, projOrNsParams, err := l.buildClauseFromProjectsOrNamespaces(lo.ProjectsOrNamespaces, dbName, joinTableIndexByLabelName)
		if err != nil {
			return nil, err
		}
		filterComponents.whereClauses = append(filterComponents.whereClauses, projOrNsClause)
		filterComponents.params = append(filterComponents.params, projOrNsParams...)
		filterComponents.isEmpty = false
	}

	// WHERE clauses (from namespace)
	if namespace != "" && namespace != "*" {
		filterComponents.whereClauses = append(filterComponents.whereClauses, fmt.Sprintf(`%s."metadata.namespace" = ?`, mainFieldPrefix))
		filterComponents.params = append(filterComponents.params, namespace)
		filterComponents.isEmpty = false
	}

	// WHERE clauses (from partitions and their corresponding parameters)
	partitionClauses, partitionClausesParams := generatePartitionClauses(namespace, partitions, mainFieldPrefix)
	if n := len(partitionClauses); n > 0 {
		if n == 1 {
			filterComponents.whereClauses = append(filterComponents.whereClauses, partitionClauses[0])
		} else {
			filterComponents.whereClauses = append(filterComponents.whereClauses, "(\n      ("+strings.Join(partitionClauses, ") OR\n      (")+")\n)")
		}
		filterComponents.params = append(filterComponents.params, partitionClausesParams...)
		filterComponents.isEmpty = false
	}

	if includeSort {
		if len(lo.SortList.SortDirectives) > 0 {
			filterComponents.orderByClauses = []string{}
			for _, sortDirective := range lo.SortList.SortDirectives {
				fields := sortDirective.Fields
				if isLabelsFieldList(fields) {
					clause, err := buildSortLabelsClause(fields[2], joinTableIndexByLabelName, sortDirective.Order == sqltypes.ASC, sortDirective.SortAsIP)
					if err != nil {
						return nil, err
					}
					filterComponents.orderByClauses = append(filterComponents.orderByClauses, clause)
				} else {
					fieldEntry, err := l.getValidFieldEntry(mainFieldPrefix, fields, true)
					if err != nil {
						return nil, err
					}
					if sortDirective.SortAsIP {
						fieldEntry = fmt.Sprintf("inet_aton(%s)", fieldEntry)
					}
					direction := "ASC"
					if sortDirective.Order == sqltypes.DESC {
						direction = "DESC"
					}
					filterComponents.orderByClauses = append(filterComponents.orderByClauses, fmt.Sprintf("%s COLLATE NOCASE %s", fieldEntry, direction))
				}
			}
		} else if l.namespaced {
			filterComponents.orderByClauses = append(filterComponents.orderByClauses, fmt.Sprintf("%s.id COLLATE NOCASE ASC", mainFieldPrefix))
		} else {
			filterComponents.orderByClauses = append(filterComponents.orderByClauses, fmt.Sprintf(`%s."metadata.name" COLLATE NOCASE ASC`, mainFieldPrefix))
		}
		filterComponents.isEmpty = false
	}

	// 4- Pagination: LIMIT clause (from lo.Pagination)

	if !isSummaryFilter && len(lo.SummaryFieldList) > 0 && lo.SummaryOnly {
		filterComponents.limitClause = fmt.Sprintf("\n  LIMIT 0")
		filterComponents.limitParam = 0
		filterComponents.isEmpty = false
		return &filterComponents, nil
	}
	limit := lo.Pagination.PageSize
	if limit > 0 {
		filterComponents.limitClause = fmt.Sprintf("\n  LIMIT %d", limit)
		filterComponents.limitParam = limit
		filterComponents.isEmpty = false
	}

	// OFFSET clause (from lo.Pagination)
	offset := 0
	if lo.Pagination.Page >= 1 {
		offset += lo.Pagination.PageSize * (lo.Pagination.Page - 1)
	}
	if offset > 0 {
		filterComponents.offsetClause = fmt.Sprintf("\n  OFFSET %d", offset)
		filterComponents.offsetParam = offset
		filterComponents.isEmpty = false
	}
	return &filterComponents, nil
}

func namesSignatures(names []string) uint64 {
	h := fnv.New64a()
	for _, name := range names {
		io.WriteString(h, name)
		io.WriteString(h, "\x00") // Null separator
	}
	return h.Sum64()
}

func generatePartitionClauses(namespaceFilter string, partitions []partition.Partition, mainFieldPrefix string) (clauses []string, params []any) {
	if len(partitions) == 0 {
		// degenerate case, there will be no results
		return []string{"FALSE"}, nil
	}

	// Map of Signature -> List of Namespaces belonging to this group
	groups := make(map[uint64]struct {
		names         []string
		namespaces    []string
		allNamespaces bool
	})

	singleNamespace := namespaceFilter != "" && namespaceFilter != "*"
	for _, thisPartition := range partitions {
		filterByNamespace := thisPartition.Namespace != "" && thisPartition.Namespace != "*"
		filterByNames := !thisPartition.All

		// Passthrough provides access to everything
		// Same for non-namespaced partitions with All=true
		if thisPartition.Passthrough || (!filterByNamespace && !filterByNames) {
			// nothing to do, no extra filtering to apply by definition
			return nil, nil
		}

		if singleNamespace && filterByNamespace && thisPartition.Namespace != namespaceFilter {
			// Omit not matching partitions, since there is a higher-level clause already
			continue
		}

		var sig uint64 // 0 is a valid signature, representing "All: true"
		var names []string
		if filterByNames {
			// Case B: Restricted Access
			names = sets.List(thisPartition.Names)
			if len(names) == 0 {
				continue // Degenerate case (FALSE)
			}
			sig = namesSignatures(names)
			if sig == 0 { // Safety: In the astronomically unlikely event we hash to 0, bump it so it doesn't grant Full access
				sig = 1
			}
		}

		group, ok := groups[sig]
		if !ok {
			group.names = names
		}
		if !filterByNamespace {
			group.allNamespaces = true
			group.namespaces = nil
		}
		if !group.allNamespaces {
			group.namespaces = append(group.namespaces, thisPartition.Namespace)
		}
		groups[sig] = group

	}
	if len(groups) == 0 {
		// Partitions didn't grant access to any resource
		return []string{"FALSE"}, nil
	}

	if g, ok := groups[0]; ok && (singleNamespace || g.allNamespaces) {
		// special case for the group with no name restrictions:
		// 1. If single namespace, the namespace condition would be redundant
		// 2. All resources in all namespaces means everything, so the `WHERE` will be omitted
		return nil, nil
	}

	for _, sig := range slices.Sorted(maps.Keys(groups)) {
		group := groups[sig]
		slices.Sort(group.namespaces)
		var conditions []string
		switch {
		case sig == 0:
			clauses = append(clauses, mainFieldPrefix+`."metadata.namespace" IN ( ?`+strings.Repeat(", ?", len(group.namespaces)-1)+" )")
			for _, ns := range group.namespaces {
				params = append(params, ns)
			}
		case !singleNamespace && !group.allNamespaces:
			conditions = append(conditions, mainFieldPrefix+`."metadata.namespace" IN ( ?`+strings.Repeat(", ?", len(group.namespaces)-1)+" )")
			for _, ns := range group.namespaces {
				params = append(params, ns)
			}
			fallthrough
		default:
			conditions = append(conditions, mainFieldPrefix+`."metadata.name" IN ( ?`+strings.Repeat(", ?", len(group.names)-1)+" )")
			for _, name := range group.names {
				params = append(params, name)
			}
			clauses = append(clauses, strings.Join(conditions, " AND "))
		}
	}

	return clauses, params
}

func (l *ListOptionIndexer) constructComplexSummaryQueryForField(fieldParts []string, fieldNum int, dbName, columnName, columnNameToDisplay string, filterComponents *filterComponentsT, mainFieldPrefix string, joinTableIndexByLabelName map[string]int, summaryNamespaced bool) (*QueryInfo, error) {
	const nl = "\n"
	isLabelField := isLabelsFieldList(fieldParts)
	withPrefix := fmt.Sprintf("w%d", fieldNum)
	withParts := make([]string, 0)
	// We don't use the main key directly, but we need it for SELECT-DISTINCT with the labels table.
	// Otherwise each instance of <finalField=some-value> will be counted separately for all different keys,
	// which we don't want.
	namespaceField := ""
	if summaryNamespaced {
		namespaceField = ", namespace"
	}
	withParts = append(withParts, fmt.Sprintf("WITH %s(key, finalField%s) AS (\n", withPrefix, namespaceField))
	withParts = append(withParts, "\tSELECT")
	if len(filterComponents.joinParts) > 0 || isLabelField {
		withParts = append(withParts, " DISTINCT")
	}
	withParts = append(withParts, fmt.Sprintf(" %s.key,", mainFieldPrefix))
	targetField := columnNameToDisplay
	if isLabelField {
		labelName := fieldParts[2]
		jtIndex, ok := joinTableIndexByLabelName[labelName]
		if !ok {
			jtIndex = len(joinTableIndexByLabelName) + 1
			jtPrefix := fmt.Sprintf("lt%d", jtIndex)
			joinTableIndexByLabelName[labelName] = jtIndex
			filterComponents.joinParts = append(filterComponents.joinParts,
				joinPart{joinCommand: "LEFT OUTER JOIN",
					tableName:      fmt.Sprintf("%s_labels", dbName),
					tableNameAlias: jtPrefix,
					onPrefix:       "f1",
					onField:        "key",
					otherPrefix:    jtPrefix,
					otherField:     "key"})
			filterComponents.whereClauses = append(filterComponents.whereClauses, fmt.Sprintf("%s.label = ?", jtPrefix))
			filterComponents.params = append(filterComponents.params, labelName)
		}
		targetField = fmt.Sprintf("lt%d.value", jtIndex)
	}
	if summaryNamespaced {
		namespaceField = fmt.Sprintf(`, %s."metadata.namespace"`, mainFieldPrefix)
	}
	withParts = append(withParts, fmt.Sprintf(` %s%s FROM "%s_fields" %s%s`, targetField, namespaceField, dbName, mainFieldPrefix, nl))
	for _, jp := range filterComponents.joinParts {
		withParts = append(withParts, fmt.Sprintf(`  %s "%s" %s ON %s.%s = %s.%s%s`,
			jp.joinCommand, jp.tableName, jp.tableNameAlias, jp.onPrefix, jp.onField, jp.otherPrefix, jp.otherField, nl))
	}
	switch len(filterComponents.whereClauses) {
	case 0: // do nothing
	case 1:
		withParts = append(withParts, fmt.Sprintf("\tWHERE %s\n", filterComponents.whereClauses[0]))
	default:
		withParts = append(withParts, fmt.Sprintf("\tWHERE (%s)\n", strings.Join(filterComponents.whereClauses, ")\n\t\tAND (")))
	}
	if len(filterComponents.orderByClauses) > 0 {
		withParts = append(withParts, "\t"+"ORDER BY "+strings.Join(filterComponents.orderByClauses, ", ")+"\n")
	}
	if filterComponents.limitClause != "" {
		withParts = append(withParts, "\t"+filterComponents.limitClause+"\n")
	}
	if filterComponents.offsetClause != "" {
		withParts = append(withParts, "\t"+filterComponents.offsetClause+"\n")
	}
	withParts = append(withParts, ")\n")
	if summaryNamespaced {
		namespaceField = fmt.Sprintf(", %s.namespace AS ns", withPrefix)
	}
	withParts = append(withParts, fmt.Sprintf("SELECT '%s' AS p, COUNT(*) AS c, %s.finalField AS k%s FROM %s\n", columnName, withPrefix, namespaceField, withPrefix))
	if summaryNamespaced {
		namespaceField = ", ns"
	}
	withParts = append(withParts, fmt.Sprintf("\tWHERE k != \"\"\n\tGROUP BY k%s", namespaceField))
	query := strings.Join(withParts, "")
	queryInfo := &QueryInfo{}
	queryInfo.query = query
	queryInfo.params = filterComponents.params
	logrus.Debugf("Summary prepared statement: %v", queryInfo.query)
	logrus.Debugf("Summary params: %v", queryInfo.params)
	return queryInfo, nil
}

func (l *ListOptionIndexer) constructQuery(lo *sqltypes.ListOptions, partitions []partition.Partition, namespace string, dbName string) (*QueryInfo, error) {
	joinTableIndexByLabelName := make(map[string]int)
	const mainObjectPrefix = "o"
	const mainFieldPrefix = "f"
	const includeSort = true
	const isSummaryFilter = false
	filterComponents, err := l.compileQuery(lo, partitions, namespace, dbName, mainFieldPrefix, joinTableIndexByLabelName, includeSort, isSummaryFilter)
	if err != nil {
		return nil, err
	}
	if err = l.checkRevision(lo); err != nil {
		return nil, err
	}
	return l.generateSQL(filterComponents, dbName, mainObjectPrefix, mainFieldPrefix)
}

func (l *ListOptionIndexer) generateSQL(filterComponents *filterComponentsT, dbName string, mainObjectPrefix string, mainFieldPrefix string) (*QueryInfo, error) {
	query := ""
	params := []any{}
	const nl = "\n"
	comma := ","
	if len(filterComponents.withParts) > 0 {
		// These are for any labels that are being sorted on but don't appear in filters
		query = "WITH "
		for i, wp := range filterComponents.withParts {
			if i == len(filterComponents.withParts)-1 {
				comma = ""
			}
			query += fmt.Sprintf(`%s(key, value) AS (
SELECT key, value FROM "%s_labels"
  WHERE label = ?
)%s
`,
				wp.labelPrefix, dbName, comma)
			params = append(params, wp.labelName)
		}
	}
	params = append(params, filterComponents.params...)

	query += "SELECT "
	if filterComponents.queryUsesLabels {
		query += "DISTINCT "
	}
	query += fmt.Sprintf(`%s.object, %s.objectnonce, %s.dekid FROM "%s" %s%s`,
		mainObjectPrefix,
		mainObjectPrefix,
		mainObjectPrefix,
		dbName,
		mainObjectPrefix,
		nl)
	query += fmt.Sprintf(`  JOIN "%s_fields" %s ON %s.key = %s.key`,
		dbName,
		mainFieldPrefix,
		mainObjectPrefix,
		mainFieldPrefix)
	if len(filterComponents.joinParts) > 0 {
		for _, joinPart := range filterComponents.joinParts {
			tablePart := ""
			// If the join is for a view, it means we're joining on a table defined in an above WITH entry,
			// so there's no actual database table that we're joining on.
			if !joinPart.isView {
				tablePart = fmt.Sprintf(" %q", joinPart.tableName)
			}
			query += fmt.Sprintf(`%s  %s%s %s ON %s.%s = %s.%s`,
				nl,
				joinPart.joinCommand,
				tablePart,
				joinPart.tableNameAlias,
				joinPart.onPrefix,
				joinPart.onField,
				joinPart.otherPrefix,
				joinPart.otherField,
			)
		}
	}
	switch len(filterComponents.whereClauses) {
	case 0: // do nothing
	case 1:
		query += fmt.Sprintf("\n  WHERE\n    %s", filterComponents.whereClauses[0])
	default:
		query += fmt.Sprintf("\n  WHERE\n    (%s)", strings.Join(filterComponents.whereClauses, ") AND\n    ("))
	}

	// before proceeding, save a copy of the query without LIMIT/OFFSET/ORDER info
	// for COUNTing all results later
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM (%s)", query)
	// There's no need for a separate countParams because the arg is always an integer
	// that's vetted by the parser

	if len(filterComponents.orderByClauses) > 0 {
		query += "\n  ORDER BY "
		query += strings.Join(filterComponents.orderByClauses, ", ")
	}

	if filterComponents.limitClause != "" {
		query += "\t" + filterComponents.limitClause + "\n"
	}
	if filterComponents.offsetClause != "" {
		query += "\t" + filterComponents.offsetClause + "\n"
	}
	queryInfo := QueryInfo{query: query, params: params}
	if filterComponents.limitClause != "" || filterComponents.offsetClause != "" {
		queryInfo.countQuery = countQuery
		queryInfo.countParams = params
		queryInfo.limit = filterComponents.limitParam
		queryInfo.offset = filterComponents.offsetParam
	}
	// Otherwise leave these as default values and the executor won't do pagination work

	return &queryInfo, nil
}

// Possible ops from the k8s parser:
// KEY = and == (same) VALUE
// KEY != VALUE
// KEY exists []  # ,KEY, => this filter
// KEY ! []  # ,!KEY, => assert KEY doesn't exist
// KEY in VALUES
// KEY notin VALUES
// KEY contains VALUES
// KEY notcontains VALUES

func (l *ListOptionIndexer) getFieldFilter(filter sqltypes.Filter, prefix string) (string, []any, error) {
	opString := ""
	escapeString := ""
	fieldEntry, err := l.getValidFieldEntry(prefix, filter.Field, false)
	if err != nil {
		return "", nil, err
	}

	switch filter.Op {
	case sqltypes.Eq:
		if filter.Partial {
			opString = "LIKE"
			escapeString = escapeBackslashDirective
		} else {
			opString = "="
		}
		clause := fmt.Sprintf("%s %s ?%s", fieldEntry, opString, escapeString)
		param := formatMatchTarget(filter)
		return clause, []any{param}, nil
	case sqltypes.NotEq:
		if filter.Partial {
			opString = "NOT LIKE"
			escapeString = escapeBackslashDirective
		} else {
			opString = "!="
		}
		clause := fmt.Sprintf("%s %s ?%s", fieldEntry, opString, escapeString)
		param := formatMatchTarget(filter)
		return clause, []any{param}, nil

	case sqltypes.Lt, sqltypes.Gt:
		sym, target, err := prepareComparisonParameters(filter.Op, filter.Matches[0])
		if err != nil {
			return "", nil, err
		}
		clause := fmt.Sprintf("%s %s ?", fieldEntry, sym)
		return clause, []any{target}, nil

	case sqltypes.Exists, sqltypes.NotExists:
		return "", nil, errors.New("NULL and NOT NULL tests aren't supported for non-label queries")

	case sqltypes.In:
		fallthrough
	case sqltypes.NotIn:
		target := "()"
		if len(filter.Matches) > 0 {
			target = fmt.Sprintf("(?%s)", strings.Repeat(", ?", len(filter.Matches)-1))
		}
		opString = "IN"
		if filter.Op == sqltypes.NotIn {
			opString = "NOT IN"
		}
		clause := fmt.Sprintf("%s %s %s", fieldEntry, opString, target)
		matches := make([]any, len(filter.Matches))
		for i, match := range filter.Matches {
			matches[i] = match
		}
		return clause, matches, nil

	case sqltypes.Contains:
		if len(filter.Matches) != 1 {
			return "", nil, fmt.Errorf("array checking works on exactly one field, %d were specified", len(filter.Matches))
		}
		clause := fmt.Sprintf("hasBarredValue(%s, ?)", fieldEntry)
		matches := make([]any, 1)
		matches[0] = filter.Matches[0]
		return clause, matches, nil

	case sqltypes.NotContains:
		if len(filter.Matches) != 1 {
			return "", nil, fmt.Errorf("array checking works on exactly one field, %d were specified", len(filter.Matches))
		}
		clause := fmt.Sprintf("NOT hasBarredValue(%s, ?)", fieldEntry)
		matches := make([]any, 1)
		matches[0] = filter.Matches[0]
		return clause, matches, nil
	}

	return "", nil, fmt.Errorf("unrecognized operator: %s", opString)
}

func (l *ListOptionIndexer) getLabelFilter(index int, filter sqltypes.Filter, mainFieldPrefix string, isSummaryFilter bool, dbName string) (string, []any, error) {
	opString := ""
	escapeString := ""
	matchFmtToUse := strictMatchFmt
	labelName := filter.Field[2]
	switch filter.Op {
	case sqltypes.Eq:
		if filter.Partial {
			opString = "LIKE"
			escapeString = escapeBackslashDirective
			matchFmtToUse = matchFmt
		} else {
			opString = "="
		}
		clause := fmt.Sprintf(`lt%d.label = ? AND lt%d.value %s ?%s`, index, index, opString, escapeString)
		return clause, []any{labelName, formatMatchTargetWithFormatter(filter.Matches[0], matchFmtToUse)}, nil

	case sqltypes.NotEq:
		if filter.Partial {
			opString = "NOT LIKE"
			escapeString = escapeBackslashDirective
			matchFmtToUse = matchFmt
		} else {
			opString = "!="
		}
		subFilter := sqltypes.Filter{
			Field: filter.Field,
			Op:    sqltypes.NotExists,
		}
		existenceClause, subParams, err := l.getLabelFilter(index, subFilter, mainFieldPrefix, isSummaryFilter, dbName)
		if err != nil {
			return "", nil, err
		}
		clause := fmt.Sprintf(`(%s) OR (lt%d.label = ? AND lt%d.value %s ?%s)`, existenceClause, index, index, opString, escapeString)
		params := append(subParams, labelName, formatMatchTargetWithFormatter(filter.Matches[0], matchFmtToUse))
		return clause, params, nil

	case sqltypes.Lt, sqltypes.Gt:
		sym, target, err := prepareComparisonParameters(filter.Op, filter.Matches[0])
		if err != nil {
			return "", nil, err
		}
		clause := fmt.Sprintf(`lt%d.label = ? AND lt%d.value %s ?`, index, index, sym)
		return clause, []any{labelName, target}, nil

	case sqltypes.Exists:
		clause := fmt.Sprintf(`lt%d.label = ?`, index)
		return clause, []any{labelName}, nil

	case sqltypes.NotExists:
		clause := ""
		if isSummaryFilter {
			mainFieldPrefix := "f1"
			newMainFieldPrefix := mainFieldPrefix + "1"
			clause = fmt.Sprintf(`%s.key NOT IN (SELECT %s.key FROM "%s_fields" %s
		LEFT OUTER JOIN "%s_labels" lt%di1 ON %s.key = lt%di1.key
		WHERE lt%di1.label = ?)`,
				mainFieldPrefix, newMainFieldPrefix, dbName, newMainFieldPrefix,
				dbName, index, newMainFieldPrefix, index,
				index)
		} else {
			clause = fmt.Sprintf(`o.key NOT IN (SELECT f1.key FROM "%s_fields" f1
		LEFT OUTER JOIN "%s_labels" lt%di1 ON f1.key = lt%di1.key
		WHERE lt%di1.label = ?)`, dbName, dbName, index, index, index)
		}
		return clause, []any{labelName}, nil

	case sqltypes.In:
		target := "(?"
		if len(filter.Matches) > 0 {
			target += strings.Repeat(", ?", len(filter.Matches)-1)
		}
		target += ")"
		clause := fmt.Sprintf(`lt%d.label = ? AND lt%d.value IN %s`, index, index, target)
		matches := make([]any, len(filter.Matches)+1)
		matches[0] = labelName
		for i, match := range filter.Matches {
			matches[i+1] = match
		}
		return clause, matches, nil

	case sqltypes.NotIn:
		target := "(?"
		if len(filter.Matches) > 0 {
			target += strings.Repeat(", ?", len(filter.Matches)-1)
		}
		target += ")"
		subFilter := sqltypes.Filter{
			Field: filter.Field,
			Op:    sqltypes.NotExists,
		}
		existenceClause, subParams, err := l.getLabelFilter(index, subFilter, mainFieldPrefix, isSummaryFilter, dbName)
		if err != nil {
			return "", nil, err
		}
		clause := fmt.Sprintf(`(%s) OR (lt%d.label = ? AND lt%d.value NOT IN %s)`, existenceClause, index, index, target)
		matches := append(subParams, labelName)
		for _, match := range filter.Matches {
			matches = append(matches, match)
		}
		return clause, matches, nil

	case sqltypes.Contains:
		if len(filter.Matches) != 1 {
			return "", nil, fmt.Errorf("array checking works on exactly one field, %d were specified", len(filter.Matches))
		}
		// Labels can't have | characters so they're implemented like '='
		filter.Op = sqltypes.Eq
		return l.getLabelFilter(index, filter, mainFieldPrefix, isSummaryFilter, dbName)

	case sqltypes.NotContains:
		if len(filter.Matches) != 1 {
			return "", nil, fmt.Errorf("array checking works on exactly one field, %d were specified", len(filter.Matches))
		}
		// Labels can't have | characters so they're implemented like '='
		filter.Op = sqltypes.NotEq
		return l.getLabelFilter(index, filter, mainFieldPrefix, isSummaryFilter, dbName)
	}
	return "", nil, fmt.Errorf("unrecognized operator: %s", opString)
}

func (l *ListOptionIndexer) getProjectsOrNamespacesFieldFilter(filter sqltypes.Filter) (string, []any, error) {
	opString := ""
	fieldEntry, err := l.getValidFieldEntry("nsf", filter.Field, false)
	if err != nil {
		return "", nil, err
	}
	switch filter.Op {
	case sqltypes.In:
		fallthrough
	case sqltypes.NotIn:
		target := "()"
		if len(filter.Matches) > 0 {
			target = fmt.Sprintf("(?%s)", strings.Repeat(", ?", len(filter.Matches)-1))
		}
		opString = "IN"
		if filter.Op == sqltypes.NotIn {
			opString = "NOT IN"
		}
		clause := fmt.Sprintf("%s %s %s", fieldEntry, opString, target)
		matches := make([]any, len(filter.Matches))
		for i, match := range filter.Matches {
			matches[i] = match
		}
		return clause, matches, nil
	}

	return "", nil, fmt.Errorf("unrecognized operator: %s", opString)
}

func (l *ListOptionIndexer) getProjectsOrNamespacesLabelFilter(index int, filter sqltypes.Filter, dbName string) (string, []any, error) {
	opString := ""
	labelName := filter.Field[2]
	target := "()"
	if len(filter.Matches) > 0 {
		target = fmt.Sprintf("(?%s)", strings.Repeat(", ?", len(filter.Matches)-1))
	}
	matches := make([]any, len(filter.Matches)+1)
	matches[0] = labelName
	for i, match := range filter.Matches {
		matches[i+1] = match
	}
	switch filter.Op {
	case sqltypes.In:
		clause := fmt.Sprintf(`lt%d.label = ? AND lt%d.value IN %s`, index, index, target)
		return clause, matches, nil
	case sqltypes.NotIn:
		clause1 := fmt.Sprintf(`(lt%d.label = ? AND lt%d.value NOT IN %s)`, index, index, target)
		clause2 := fmt.Sprintf(`(o.key NOT IN (SELECT f1.key FROM "%s_fields" f1
		LEFT OUTER JOIN "_v1_Namespace_fields" nsf1 ON f1."metadata.namespace" = nsf1."metadata.name"
		LEFT OUTER JOIN "_v1_Namespace_labels" lt%di1 ON nsf1.key = lt%di1.key
		WHERE lt%di1.label = ?))`, dbName, index, index, index)
		matches = append(matches, labelName)
		clause := fmt.Sprintf("%s OR %s", clause1, clause2)
		return clause, matches, nil
	}
	return "", nil, fmt.Errorf("unrecognized operator: %s", opString)
}

func (l *ListOptionIndexer) getStandardColumnNameToDisplay(fieldParts []string, mainFieldPrefix string) (string, error) {
	fieldID := smartJoin(fieldParts)
	var columnValueName string

	// Try direct field lookup first
	if field, ok := l.indexedFields[fieldID]; ok {
		columnName := field.ColumnName()
		if mainFieldPrefix == "" {
			columnValueName = fmt.Sprintf("%q", columnName)
		} else {
			columnValueName = fmt.Sprintf("%s.%q", mainFieldPrefix, columnName)
		}
		return columnValueName, nil
	}

	// Fallback for numeric-indexed field expressions like spec.containers.image[2]
	if len(fieldParts) == 1 || containsNonNumericRegex.MatchString(fieldParts[len(fieldParts)-1]) {
		return "", fmt.Errorf("column is invalid [%s]: %w", fieldID, ErrInvalidColumn)
	}

	// Check if base field (without numeric index) exists
	baseFieldID := smartJoin(fieldParts[:len(fieldParts)-1])
	if field, ok := l.indexedFields[baseFieldID]; ok {
		index, err := strconv.Atoi(fieldParts[len(fieldParts)-1])
		if err != nil {
			return "", fmt.Errorf("column is invalid [%s]: %w", fieldID, ErrInvalidColumn)
		}
		columnName := field.ColumnName()
		if mainFieldPrefix == "" {
			columnValueName = fmt.Sprintf(`extractBarredValue(%q, %d)`, columnName, index)
		} else {
			columnValueName = fmt.Sprintf(`extractBarredValue(%s.%q, %d)`, mainFieldPrefix, columnName, index)
		}
		return columnValueName, nil
	}

	return "", fmt.Errorf("column is invalid [%s]: %w", fieldID, ErrInvalidColumn)
}

// Suppose the query access something like 'spec.containers[3].image' but only
// spec.containers.image is specified in the index.  If `spec.containers` is
// an array, then spec.containers.image is a pseudo-array of |-separated strings,
// and we can use our custom registered extractBarredValue function to extract the
// desired substring.
//
// The index can appear anywhere in the list of fields after the first entry,
// but we always end up with a |-separated list of substrings. Most of the time
// the index will be the second-last entry, but we lose nothing allowing for any
// position.
// Indices are 0-based.

func (l *ListOptionIndexer) getValidFieldEntry(prefix string, fields []string, inSort bool) (string, error) {
	fieldID := smartJoin(fields)

	// Try direct field lookup first
	if field, ok := l.indexedFields[fieldID]; ok {
		if inSort {
			computedField, ok := field.(*ComputedField)
			if ok && computedField.IsTimestamp {
				return fmt.Sprintf(`adjustTimestampForSorting(%s."%s")`, prefix, computedField.Name), nil
			}
		}
		return fmt.Sprintf(`%s."%s"`, prefix, field.ColumnName()), nil
	}

	// Fallback: handle numeric indices in the path (e.g., spec.containers.3.image)
	// Look for a numeric part anywhere in the path and try to find the base field without it
	if len(fields) <= 2 {
		return "", fmt.Errorf("column is invalid [%s]: %w", fieldID, ErrInvalidColumn)
	}

	// Find the last numeric index in the field parts
	idx := -1
	for i := len(fields) - 1; i > 0; i-- {
		if !containsNonNumericRegex.MatchString(fields[i]) {
			idx = i
			break
		}
	}

	if idx == -1 {
		return "", fmt.Errorf("column is invalid [%s]: %w", fieldID, ErrInvalidColumn)
	}

	// Build the base field without the numeric index
	indexField := fields[idx]
	otherFields := append(fields[0:idx], fields[idx+1:]...)
	baseFieldID := smartJoin(otherFields)

	// Check if the base field exists
	if field, ok := l.indexedFields[baseFieldID]; ok {
		return fmt.Sprintf(`extractBarredValue(%s."%s", "%s")`, prefix, field.ColumnName(), indexField), nil
	}

	return "", fmt.Errorf("column is invalid [%s]: %w", fieldID, ErrInvalidColumn)
}

// isIntegerField checks if a field is stored as INTEGER type.
func (l *ListOptionIndexer) isIntegerField(fieldID string) bool {
	if f, ok := l.indexedFields[fieldID]; ok {
		return f.ColumnType() == "INTEGER"
	}
	return false
}

func (f filterComponentsT) copy() filterComponentsT {
	return filterComponentsT{
		withParts:       append([]withPart{}, f.withParts...),
		joinParts:       append([]joinPart{}, f.joinParts...),
		whereClauses:    append([]string{}, f.whereClauses...),
		orderByClauses:  append([]string{}, f.orderByClauses...),
		limitClause:     f.limitClause,
		limitParam:      f.limitParam,
		offsetClause:    f.offsetClause,
		offsetParam:     f.offsetParam,
		params:          append([]any{}, f.params...),
		queryUsesLabels: f.queryUsesLabels,
		isEmpty:         f.isEmpty,
	}
}

// Helper functions for the ListOptionIndexer sql-gen methods in alphabetical order3

func buildSortLabelsClause(labelName string, joinTableIndexByLabelName map[string]int, isAsc bool, sortAsIP bool) (string, error) {
	ltIndex, err := internLabel(labelName, joinTableIndexByLabelName, -1)
	fieldEntry := fmt.Sprintf("lt%d.value", ltIndex)
	if sortAsIP {
		fieldEntry = fmt.Sprintf("inet_aton(%s)", fieldEntry)
	}
	if err != nil {
		return "", err
	}
	dir := "ASC"
	nullsPosition := "LAST"
	if !isAsc {
		dir = "DESC"
		nullsPosition = "FIRST"
	}
	return fmt.Sprintf("%s COLLATE NOCASE %s NULLS %s", fieldEntry, dir, nullsPosition), nil
}

func extractSubFields(fields string) []string {
	subfields := make([]string, 0)
	for _, subField := range subfieldRegex.FindAllString(fields, -1) {
		subfields = append(subfields, strings.TrimSuffix(subField, "."))
	}
	return subfields
}

func formatMatchTarget(filter sqltypes.Filter) string {
	format := strictMatchFmt
	if filter.Partial {
		format = matchFmt
	}
	return formatMatchTargetWithFormatter(filter.Matches[0], format)
}

func formatMatchTargetWithFormatter(match string, format string) string {
	// To allow matches on the backslash itself, the character needs to be replaced first.
	// Otherwise, it will undo the following replacements.
	match = strings.ReplaceAll(match, `\`, `\\`)
	match = strings.ReplaceAll(match, `_`, `\_`)
	match = strings.ReplaceAll(match, `%`, `\%`)
	return fmt.Sprintf(format, match)
}

// getField extracts the value of a field expressed as a string path from an unstructured object
func getField(a any, field string) (any, error) {
	subFields := extractSubFields(field)
	o, ok := a.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("unexpected object type, expected unstructured.Unstructured: %v", a)
	}

	var obj interface{}
	var found bool
	var err error
	obj = o.Object
	for i, subField := range subFields {
		switch t := obj.(type) {
		case map[string]interface{}:
			subField = strings.TrimSuffix(strings.TrimPrefix(subField, "["), "]")
			obj, found, err = unstructured.NestedFieldNoCopy(t, subField)
			if err != nil {
				return nil, err
			}
			if !found {
				// Particularly with labels/annotation indexes, it is totally possible that some objects won't have these.
				// So either this is not an error state, or it could be an error state with a type that callers
				// will need to deal with somehow.
				return nil, nil
			}
		case []interface{}:
			if strings.HasPrefix(subField, "[") && strings.HasSuffix(subField, "]") {
				key, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(subField, "["), "]"))
				if err != nil {
					return nil, fmt.Errorf("[listoption indexer] failed to convert subfield [%s] to int in listoption index: %w", subField, err)
				}
				if key >= len(t) {
					return nil, fmt.Errorf("[listoption indexer] given index is too large for slice of len %d", len(t))
				}
				// Preserve the type of array elements instead of stringifying
				obj = t[key]
			} else if i == len(subFields)-1 {
				// If the last layer is an array, return array.map(a => a[subfield])
				result := make([]string, len(t))
				for index, v := range t {
					itemVal, ok := v.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf(failedToGetFromSliceFmt, subField)
					}

					_, found := itemVal[subField]
					if found {
						itemStr, ok := itemVal[subField].(string)
						if !ok {
							return nil, fmt.Errorf(failedToGetFromSliceFmt, subField)
						}
						result[index] = itemStr
					} else {
						result[index] = ""
					}
				}
				return result, nil
			}
		default:
			return nil, fmt.Errorf("[listoption indexer] failed to parse subfields: %v", subFields)
		}
	}
	return obj, nil
}

func getLabelColumnNameToDisplay(fieldParts []string) (string, error) {
	lastPart := fieldParts[2]
	columnNameToDisplay := ""
	const nameLimit = 63
	if len(lastPart) > nameLimit {
		return "", fmt.Errorf("label value %s..%s (%d chars, max %d) is too long", lastPart[0:10], lastPart[len(lastPart)-10:], len(lastPart), nameLimit)
	}
	simpleName := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if simpleName.MatchString(lastPart) {
		columnNameToDisplay = strings.Join(fieldParts, ".")
	} else {
		compoundName := regexp.MustCompile(`[^a-zA-Z0-9_\-./]`)
		if compoundName.MatchString(lastPart) {
			return "", fmt.Errorf("invalid label name: %s", lastPart)
		}
		columnNameToDisplay = fmt.Sprintf("metadata.labels[%s]", lastPart)
	}
	return columnNameToDisplay, nil
}

func getUnboundSortLabels(lo *sqltypes.ListOptions) []string {
	numSortDirectives := len(lo.SortList.SortDirectives)
	if numSortDirectives == 0 {
		return make([]string, 0)
	}
	unboundSortLabels := make(map[string]bool)
	for _, sortDirective := range lo.SortList.SortDirectives {
		fields := sortDirective.Fields
		if isLabelsFieldList(fields) {
			unboundSortLabels[fields[2]] = true
		}
	}
	if lo.Filters != nil {
		for _, andFilter := range lo.Filters {
			for _, orFilter := range andFilter.Filters {
				if isLabelFilter(&orFilter) {
					switch orFilter.Op {
					case sqltypes.In, sqltypes.Eq, sqltypes.Gt, sqltypes.Lt, sqltypes.Exists:
						delete(unboundSortLabels, orFilter.Field[2])
						// other ops don't necessarily select a label
					}
				}
			}
		}
	}
	return slices.Collect(maps.Keys(unboundSortLabels))
}

func getWithParts(unboundSortLabels []string, joinTableIndexByLabelName map[string]int, dbName string, mainFieldPrefix string) ([]string, []any, []string, []string, error) {
	numLabels := len(unboundSortLabels)
	parts := make([]string, numLabels)
	params := make([]any, numLabels)
	withNames := make([]string, numLabels)
	joinParts := make([]string, numLabels)
	for i, label := range unboundSortLabels {
		i1 := i + 1
		idx, err := internLabel(label, joinTableIndexByLabelName, i1)
		if err != nil {
			return parts, params, withNames, joinParts, err
		}
		parts[i] = fmt.Sprintf(`lt%d(key, value) AS (
SELECT key, value FROM "%s_labels"
  WHERE label = ?
)`, idx, dbName)
		params[i] = label
		withNames[i] = fmt.Sprintf("lt%d", idx)
		joinParts[i] = fmt.Sprintf(`LEFT OUTER JOIN "%s_labels" lt%d ON %s.key = lt%d.key`, dbName, idx, mainFieldPrefix, idx)
	}

	return parts, params, withNames, joinParts, nil
}

func getWithPartsForCompiling(unboundSortLabels []string, joinTableIndexByLabelName map[string]int, mainFieldPrefix string) ([]withPart, error) {
	numLabels := len(unboundSortLabels)
	withParts := make([]withPart, numLabels)
	for i, label := range unboundSortLabels {
		i1 := i + 1
		idx, err := internLabel(label, joinTableIndexByLabelName, i1)
		if err != nil {
			return nil, err
		}
		withParts[i] = withPart{
			labelName:       label,
			mainFieldPrefix: mainFieldPrefix,
			labelIndex:      idx,
			labelPrefix:     fmt.Sprintf("lt%d", idx),
		}
	}
	return withParts, nil
}

func hasLabelFilter(filters []sqltypes.OrFilter) bool {
	for _, outerFilter := range filters {
		for _, filter := range outerFilter.Filters {
			if isLabelFilter(&filter) {
				return true
			}
		}
	}
	return false
}

// if nextNum <= 0 return an error message
func internLabel(labelName string, joinTableIndexByLabelName map[string]int, nextNum int) (int, error) {
	i, ok := joinTableIndexByLabelName[labelName]
	if ok {
		return i, nil
	}
	if nextNum <= 0 {
		return -1, fmt.Errorf("internal error: no join-table index given for label \"%s\"", labelName)
	}
	joinTableIndexByLabelName[labelName] = nextNum
	return nextNum, nil
}

func isLabelFilter(f *sqltypes.Filter) bool {
	return len(f.Field) >= 2 && f.Field[0] == "metadata" && f.Field[1] == "labels"
}

func isLabelsFieldList(fields []string) bool {
	return len(fields) == 3 && fields[0] == "metadata" && fields[1] == "labels"
}

func prepareComparisonParameters(op sqltypes.Op, target string) (string, float64, error) {
	num, err := strconv.ParseFloat(target, 32)
	if err != nil {
		return "", 0, err
	}
	switch op {
	case sqltypes.Lt:
		return "<", num, nil
	case sqltypes.Gt:
		return ">", num, nil
	}
	return "", 0, fmt.Errorf("unrecognized operator when expecting '<' or '>': '%s'", op)
}

// There are two kinds of string arrays to turn into a string, based on the last value in the array
// simple: ["a", "b", "conformsToIdentifier"] => "a.b.conformsToIdentifier"
// complex: ["a", "b", "foo.io/stuff"] => "a.b[foo.io/stuff]"

func smartJoin(s []string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) == 1 {
		return s[0]
	}
	lastBit := s[len(s)-1]
	simpleName := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if simpleName.MatchString(lastBit) {
		return strings.Join(s, ".")
	}
	return fmt.Sprintf("%s[%s]", strings.Join(s[0:len(s)-1], "."), lastBit)
}

// toColumnName returns the column name corresponding to a field expressed as string slice
func toColumnName(s []string) string {
	return db.Sanitize(smartJoin(s))
}
