package informer

import (
	"context"
	"database/sql"
	"encoding/gob"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rancher/lasso/pkg/cache/sql/db"
	"github.com/rancher/lasso/pkg/cache/sql/partition"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

// ListOptionIndexer extends Indexer by allowing queries based on ListOption
type ListOptionIndexer struct {
	*Indexer

	namespaced    bool
	indexedFields []string

	addFieldQuery    string
	deleteFieldQuery string

	addFieldStmt    *sql.Stmt
	deleteFieldStmt *sql.Stmt
}

var (
	defaultIndexedFields   = []string{"metadata.name", "metadata.creationTimestamp"}
	defaultIndexNamespaced = "metadata.namespace"
	subfieldRegex          = regexp.MustCompile(`([a-zA-Z]+)|(\[[a-zA-Z./]+])|(\[[0-9]+])`)

	InvalidColumnErr = errors.New("supplied column is invalid")
)

const (
	matchFmt             = `%%%s%%`
	strictMatchFmt       = `%s`
	createFieldsTableFmt = `CREATE TABLE db2."%s_fields" (
			key TEXT NOT NULL PRIMARY KEY,
            %s
	   )`
	createFieldsIndexFmt = `CREATE INDEX db2."%s_%s_index" ON "%s_fields"("%s")`

	failedToGetFromSliceFmt = "[listoption indexer] failed to get subfield [%s] from slice items: %w"
)

// NewListOptionIndexer returns a SQLite-backed cache.Indexer of unstructured.Unstructured Kubernetes resources of a certain GVK
// ListOptionIndexer is also able to satisfy ListOption queries on indexed (sub)fields
// Fields are specified as slices (eg. "metadata.resourceVersion" is ["metadata", "resourceVersion"])
func NewListOptionIndexer(fields [][]string, s Store, namespaced bool) (*ListOptionIndexer, error) {
	// necessary in order to gob/ungob unstructured.Unstructured objects
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})

	i, err := NewIndexer(cache.Indexers{}, s)
	if err != nil {
		return nil, err
	}

	var indexedFields []string
	for _, f := range defaultIndexedFields {
		indexedFields = append(indexedFields, f)
	}
	if namespaced {
		indexedFields = append(indexedFields, defaultIndexNamespaced)
	}
	for _, f := range fields {
		indexedFields = append(indexedFields, toColumnName(f))
	}

	l := &ListOptionIndexer{
		Indexer:       i,
		namespaced:    namespaced,
		indexedFields: indexedFields,
	}
	l.RegisterAfterUpsert(l.afterUpsert)
	l.RegisterAfterDelete(l.afterDelete)
	columnDefs := make([]string, len(indexedFields))
	for index, field := range indexedFields {
		column := fmt.Sprintf(`"%s" TEXT`, field)
		columnDefs[index] = column
	}

	tx, err := l.Begin()
	if err != nil {
		return nil, err
	}
	err = tx.Exec(fmt.Sprintf(createFieldsTableFmt, i.GetName(), strings.Join(columnDefs, ", ")))
	if err != nil {
		return nil, err
	}

	columns := make([]string, len(indexedFields))
	qmarks := make([]string, len(indexedFields))
	setStatements := make([]string, len(indexedFields))

	for index, field := range indexedFields {
		// create index for field
		err = tx.Exec(fmt.Sprintf(createFieldsIndexFmt, i.GetName(), field, i.GetName(), field))
		if err != nil {
			return nil, err
		}

		// format field into column for prepared statement
		column := fmt.Sprintf(`"%s"`, field)
		columns[index] = column

		// add placeholder for column's value in prepared statement
		qmarks[index] = "?"

		// add formatted set statement for prepared statement
		setStatement := fmt.Sprintf(`"%s" = excluded."%s"`, field, field)
		setStatements[index] = setStatement
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	l.addFieldQuery = fmt.Sprintf(
		`INSERT INTO db2."%s_fields"(key, %s) VALUES (?, %s) ON CONFLICT DO UPDATE SET %s`,
		i.GetName(),
		strings.Join(columns, ", "),
		strings.Join(qmarks, ", "),
		strings.Join(setStatements, ", "),
	)
	l.deleteFieldQuery = fmt.Sprintf(`DELETE FROM db2."%s_fields" WHERE key = ?`, s.GetName())

	l.addFieldStmt = l.Prepare(l.addFieldQuery)
	l.deleteFieldStmt = l.Prepare(l.deleteFieldQuery)

	return l, nil
}

/* Core methods */

// afterUpsert saves sortable/filterable fields into tables
func (l *ListOptionIndexer) afterUpsert(key string, obj any, tx db.TXClient) error {
	args := []any{key}
	for _, field := range l.indexedFields {
		value, err := getField(obj, field)
		if err != nil {
			logrus.Errorf("cannot index object of type [%s] with key [%s] for indexer [%s]: %v", l.GetType().String(), key, l.GetName(), err)
			cErr := tx.Cancel()
			if cErr != nil {
				return fmt.Errorf("could not cancel transaction: %s while recovering from error: %w", cErr.Error(), err)
			}
			return err
		}
		switch typedValue := value.(type) {
		case nil:
			args = append(args, "")
		case int, bool, string:
			args = append(args, fmt.Sprint(typedValue))
		case []string:
			args = append(args, strings.Join(typedValue, "|"))
		default:
			return errors.Errorf("%v has a non-supported type value: %v", field, value)
		}
	}

	err := tx.StmtExec(tx.Stmt(l.addFieldStmt), args...)
	if err != nil {
		return fmt.Errorf("while executing query: %s got error: %w", l.addFieldQuery, err)
	}
	return nil
}

func (l *ListOptionIndexer) afterDelete(key string, tx db.TXClient) error {
	args := []any{key}

	err := tx.StmtExec(tx.Stmt(l.deleteFieldStmt), args...)
	if err != nil {
		return fmt.Errorf("while executing query: %s got error: %w", l.deleteFieldQuery, err)
	}
	return nil
}

// ListByOptions returns objects according to the specified list options and partitions.
// Specifically:
//   - an unstructured list of resources belonging to any of the specified partitions
//   - the total number of resources (returned list might be a subset depending on pagination options in lo)
//   - a continue token, if there are more pages after the returned one
//   - an error instead of all of the above if anything went wrong
func (l *ListOptionIndexer) ListByOptions(ctx context.Context, lo ListOptions, partitions []partition.Partition, namespace string) (*unstructured.UnstructuredList, int, string, error) {
	// 1- Intro: SELECT and JOIN clauses
	query := fmt.Sprintf(`SELECT o.object, o.objectnonce, o.dekid FROM "%s" o`, l.GetName())
	query += "\n  "
	query += fmt.Sprintf(`JOIN db2."%s_fields" f ON o.key = f.key`, l.GetName())
	params := []any{}

	// 2- Filtering: WHERE clauses (from lo.Filters)
	whereClauses := []string{}
	for _, orFilters := range lo.Filters {
		orClause, orParams, err := l.buildORClauseFromFilters(orFilters)
		if err != nil {
			return nil, 0, "", err
		}
		if orClause == "" {
			continue
		}
		whereClauses = append(whereClauses, orClause)
		params = append(params, orParams...)
	}

	// WHERE clauses (from namespace)
	if namespace != "" && namespace != "*" {
		whereClauses = append(whereClauses, fmt.Sprintf(`f."metadata.namespace" = ?`))
		params = append(params, namespace)
	}

	// WHERE clauses (from partitions and their corresponding parameters)
	partitionClauses := []string{}
	for _, partition := range partitions {
		if partition.Passthrough {
			// nothing to do, no extra filtering to apply by definition
		} else {
			singlePartitionClauses := []string{}

			// filter by namespace
			if partition.Namespace != "" && partition.Namespace != "*" {
				singlePartitionClauses = append(singlePartitionClauses, fmt.Sprintf(`f."metadata.namespace" = ?`))
				params = append(params, partition.Namespace)
			}

			// optionally filter by names
			if !partition.All {
				names := partition.Names

				if len(names) == 0 {
					// degenerate case, there will be no results
					singlePartitionClauses = append(singlePartitionClauses, "FALSE")
				} else {
					singlePartitionClauses = append(singlePartitionClauses, fmt.Sprintf(`f."metadata.name" IN (?%s)`, strings.Repeat(", ?", len(partition.Names)-1)))
					// sort for reproducibility
					sortedNames := partition.Names.UnsortedList()
					sort.Strings(sortedNames)
					for _, name := range sortedNames {
						params = append(params, name)
					}
				}
			}

			if len(singlePartitionClauses) > 0 {
				partitionClauses = append(partitionClauses, strings.Join(singlePartitionClauses, " AND "))
			}
		}
	}
	if len(partitions) == 0 {
		// degenerate case, there will be no results
		whereClauses = append(whereClauses, "FALSE")
	}
	if len(partitionClauses) == 1 {
		whereClauses = append(whereClauses, partitionClauses[0])
	}
	if len(partitionClauses) > 1 {
		whereClauses = append(whereClauses, "(\n      ("+strings.Join(partitionClauses, ") OR\n      (")+")\n)")
	}

	if len(whereClauses) > 0 {
		query += "\n  WHERE\n    "
		for index, clause := range whereClauses {
			query += fmt.Sprintf("(%s)", clause)
			if index == len(whereClauses)-1 {
				break
			}
			query += " AND\n    "
		}
	}

	// 2- Sorting: ORDER BY clauses (from lo.Sort)
	orderByClauses := []string{}
	if len(lo.Sort.PrimaryField) > 0 {
		columnName := toColumnName(lo.Sort.PrimaryField)
		if err := l.validateColumn(columnName); err != nil {
			return nil, 0, "", err
		}

		direction := "ASC"
		if lo.Sort.PrimaryOrder == DESC {
			direction = "DESC"
		}
		orderByClauses = append(orderByClauses, fmt.Sprintf(`f."%s" %s`, columnName, direction))
	}
	if len(lo.Sort.SecondaryField) > 0 {
		columnName := toColumnName(lo.Sort.SecondaryField)
		if err := l.validateColumn(columnName); err != nil {
			return nil, 0, "", err
		}

		direction := "ASC"
		if lo.Sort.SecondaryOrder == DESC {
			direction = "DESC"
		}
		orderByClauses = append(orderByClauses, fmt.Sprintf(`f."%s" %s`, columnName, direction))
	}

	if len(orderByClauses) > 0 {
		query += "\n  ORDER BY "
		query += strings.Join(orderByClauses, ", ")
	} else {
		// make sure one default order is always picked
		if l.namespaced {
			query += "\n  ORDER BY f.\"metadata.namespace\" ASC, f.\"metadata.name\" ASC "
		} else {
			query += "\n  ORDER BY f.\"metadata.name\" ASC "
		}
	}

	// 4- Pagination: LIMIT clause (from lo.Pagination and/or lo.ChunkSize/lo.Resume)

	// before proceeding, save a copy of the query and params without LIMIT/OFFSET
	// for COUNTing all results later
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM (%s)", query)
	countParams := params[:]

	limitClause := ""
	// take the smallest limit between lo.Pagination and lo.ChunkSize
	limit := lo.Pagination.PageSize
	if limit == 0 || (lo.ChunkSize > 0 && lo.ChunkSize < limit) {
		limit = lo.ChunkSize
	}
	if limit > 0 {
		limitClause = "\n  LIMIT ?"
		params = append(params, limit)
	}

	// OFFSET clause (from lo.Pagination and/or lo.Resume)
	offsetClause := ""
	offset := 0
	if lo.Resume != "" {
		offsetInt, err := strconv.Atoi(lo.Resume)
		if err != nil {
			return nil, 0, "", err
		}
		offset = offsetInt
	}
	if lo.Pagination.Page >= 1 {
		offset += lo.Pagination.PageSize * (lo.Pagination.Page - 1)
	}
	if offset > 0 {
		offsetClause = "\n  OFFSET ?"
		params = append(params, offset)
	}

	// assemble and log the final query
	query += limitClause
	query += offsetClause
	logrus.Debugf("ListOptionIndexer prepared statement: %v", query)
	logrus.Debugf("Params: %v", params...)

	// execute
	stmt := l.Prepare(query)
	defer l.CloseStmt(stmt)
	rows, err := l.QueryForRows(ctx, stmt, params...)
	if err != nil {
		return nil, 0, "", fmt.Errorf("while executing query: %s got error: %w", query, err)
	}
	items, err := l.ReadObjects(rows, l.GetType(), l.GetShouldEncrypt())
	if err != nil {
		return nil, 0, "", err
	}

	total := len(items)
	// if limit or offset were set, execute counting of all rows
	if limit > 0 || offset > 0 {
		countStmt := l.Prepare(countQuery)
		defer l.CloseStmt(countStmt)
		rows, err = l.QueryForRows(ctx, countStmt, countParams...)
		if err != nil {
			return nil, 0, "", errors.Wrapf(err, "Error while executing query:\n %s", countQuery)
		}
		total, err = l.ReadInt(rows)
		if err != nil {
			return nil, 0, "", err
		}
	}

	continueToken := ""
	if limit > 0 && offset+len(items) < total {
		continueToken = fmt.Sprintf("%d", offset+limit)
	}

	return toUnstructuredList(items), total, continueToken, nil
}

func (l *ListOptionIndexer) validateColumn(column string) error {
	for _, v := range l.indexedFields {
		if v == column {
			return nil
		}
	}
	return fmt.Errorf("column is invalid [%s]: %w", column, InvalidColumnErr)
}

// buildORClause creates an SQLite compatible query that ORs conditions built from passed filters
func (l *ListOptionIndexer) buildORClauseFromFilters(orFilters OrFilter) (string, []any, error) {
	var orWhereClause string
	var params []any

	for index, filter := range orFilters.Filters {
		opString := "LIKE"
		if filter.Op == NotEq {
			opString = "NOT LIKE"
		}
		columnName := toColumnName(filter.Field)
		if err := l.validateColumn(columnName); err != nil {
			return "", nil, err
		}

		orWhereClause += fmt.Sprintf(`f."%s" %s ?`, columnName, opString)
		format := strictMatchFmt
		if filter.Partial {
			format = matchFmt
		}
		params = append(params, fmt.Sprintf(format, filter.Match))
		if index == len(orFilters.Filters)-1 {
			continue
		}
		orWhereClause += " OR "
	}
	return orWhereClause, params, nil
}

// toColumnName returns the column name corresponding to a field expressed as string slice
func toColumnName(s []string) string {
	return strings.Join(s, ".")
}

// getField extracts the value of a field expressed as a string path from an unstructured object
func getField(a any, field string) (any, error) {
	subFields := extractSubFields(field)
	o, ok := a.(*unstructured.Unstructured)
	if !ok {
		return nil, errors.Errorf("Unexpected object type, expected unstructured.Unstructured: %v", a)
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
				// particularly with labels/annotation indexes, it is totally possible that some objects won't have these,
				// so either we this is not an error state or it could be an error state with a type that callers can check for
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
				obj = fmt.Sprintf("%v", t[key])
			} else if i == len(subFields)-1 {
				result := make([]string, len(t))
				for index, v := range t {
					itemVal, ok := v.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf(failedToGetFromSliceFmt, subField, err)
					}
					itemStr, ok := itemVal[subField].(string)
					if !ok {
						return nil, fmt.Errorf(failedToGetFromSliceFmt, subField, err)
					}
					result[index] = itemStr
				}
				return result, nil
			}
		default:
			return nil, fmt.Errorf("[listoption indexer] failed to parse subfields: %v", subFields)
		}
	}
	return obj, nil
}

func extractSubFields(fields string) []string {
	subfields := make([]string, 0)
	for _, subField := range subfieldRegex.FindAllString(fields, -1) {
		subfields = append(subfields, strings.TrimSuffix(subField, "."))
	}
	return subfields
}

// toUnstructuredList turns a slice of unstructured objects into an unstructured.UnstructuredList
func toUnstructuredList(items []any) *unstructured.UnstructuredList {
	objectItems := make([]map[string]any, len(items))
	result := &unstructured.UnstructuredList{
		Items:  make([]unstructured.Unstructured, len(items)),
		Object: map[string]interface{}{"items": objectItems},
	}
	for i, item := range items {
		result.Items[i] = *item.(*unstructured.Unstructured)
		objectItems[i] = item.(*unstructured.Unstructured).Object
	}
	return result
}
