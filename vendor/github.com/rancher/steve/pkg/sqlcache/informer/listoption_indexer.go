package informer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rancher/steve/pkg/sqlcache/db/transaction"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/partition"
)

// ListOptionIndexer extends Indexer by allowing queries based on ListOption
type ListOptionIndexer struct {
	*Indexer

	namespaced    bool
	indexedFields []string

	// maximumEventsCount is how many events to keep. 0 means keep all events.
	maximumEventsCount int

	latestRVLock sync.RWMutex
	latestRV     string

	watchersLock sync.RWMutex
	watchers     map[*watchKey]*watcher

	upsertEventsQuery        string
	findEventsRowByRVQuery   string
	listEventsAfterQuery     string
	deleteEventsByCountQuery string
	addFieldsQuery           string
	deleteFieldsByKeyQuery   string
	deleteFieldsQuery        string
	upsertLabelsQuery        string
	deleteLabelsByKeyQuery   string
	deleteLabelsQuery        string

	upsertEventsStmt        *sql.Stmt
	findEventsRowByRVStmt   *sql.Stmt
	listEventsAfterStmt     *sql.Stmt
	deleteEventsByCountStmt *sql.Stmt
	addFieldsStmt           *sql.Stmt
	deleteFieldsByKeyStmt   *sql.Stmt
	deleteFieldsStmt        *sql.Stmt
	upsertLabelsStmt        *sql.Stmt
	deleteLabelsByKeyStmt   *sql.Stmt
	deleteLabelsStmt        *sql.Stmt
}

var (
	defaultIndexedFields    = []string{"metadata.name", "metadata.creationTimestamp"}
	defaultIndexNamespaced  = "metadata.namespace"
	subfieldRegex           = regexp.MustCompile(`([a-zA-Z]+)|(\[[-a-zA-Z./]+])|(\[[0-9]+])`)
	containsNonNumericRegex = regexp.MustCompile(`\D`)

	ErrInvalidColumn = errors.New("supplied column is invalid")
	ErrTooOld        = errors.New("resourceversion too old")
)

const (
	matchFmt                 = `%%%s%%`
	strictMatchFmt           = `%s`
	escapeBackslashDirective = ` ESCAPE '\'` // The leading space is crucial for unit tests only '

	// RV stands for ResourceVersion
	createEventsTableFmt = `CREATE TABLE "%s_events" (
                       rv TEXT NOT NULL,
                       type TEXT NOT NULL,
                       event BLOB NOT NULL,
                       PRIMARY KEY (type, rv)
          )`
	listEventsAfterFmt = `SELECT type, rv, event
	       FROM "%s_events"
	       WHERE rowid > ?
       `
	findEventsRowByRVFmt = `SELECT rowid
               FROM "%s_events"
               WHERE rv = ?
       `
	deleteEventsByCountFmt = `DELETE FROM "%s_events"
	WHERE rowid
	NOT IN (
		SELECT rowid FROM "%s_events" ORDER BY rowid DESC LIMIT ?
	)`

	createFieldsTableFmt = `CREATE TABLE "%s_fields" (
			key TEXT NOT NULL PRIMARY KEY,
            %s
	   )`
	createFieldsIndexFmt = `CREATE INDEX "%s_%s_index" ON "%s_fields"("%s")`
	deleteFieldsFmt      = `DELETE FROM "%s_fields"`

	failedToGetFromSliceFmt = "[listoption indexer] failed to get subfield [%s] from slice items"

	createLabelsTableFmt = `CREATE TABLE IF NOT EXISTS "%s_labels" (
		key TEXT NOT NULL REFERENCES "%s"(key) ON DELETE CASCADE,
		label TEXT NOT NULL,
		value TEXT NOT NULL,
		PRIMARY KEY (key, label)
	)`
	createLabelsTableIndexFmt = `CREATE INDEX IF NOT EXISTS "%s_labels_index" ON "%s_labels"(label, value)`

	upsertLabelsStmtFmt      = `REPLACE INTO "%s_labels"(key, label, value) VALUES (?, ?, ?)`
	deleteLabelsByKeyStmtFmt = `DELETE FROM "%s_labels" WHERE KEY = ?`
	deleteLabelsStmtFmt      = `DELETE FROM "%s_labels"`
)

type ListOptionIndexerOptions struct {
	// Fields is a list of fields within the object that we want indexed for
	// filtering & sorting. Each field is specified as a slice.
	//
	// For example, .metadata.resourceVersion should be specified as []string{"metadata", "resourceVersion"}
	Fields [][]string
	// IsNamespaced determines whether the GVK for this ListOptionIndexer is
	// namespaced
	IsNamespaced bool
	// MaximumEventsCount is the maximum number of events we want to keep
	// in the _events table.
	//
	// Zero means never delete events.
	MaximumEventsCount int
}

// NewListOptionIndexer returns a SQLite-backed cache.Indexer of unstructured.Unstructured Kubernetes resources of a certain GVK
// ListOptionIndexer is also able to satisfy ListOption queries on indexed (sub)fields.
func NewListOptionIndexer(ctx context.Context, s Store, opts ListOptionIndexerOptions) (*ListOptionIndexer, error) {
	// necessary in order to gob/ungob unstructured.Unstructured objects
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})

	i, err := NewIndexer(ctx, cache.Indexers{}, s)
	if err != nil {
		return nil, err
	}

	var indexedFields []string
	for _, f := range defaultIndexedFields {
		indexedFields = append(indexedFields, f)
	}
	if opts.IsNamespaced {
		indexedFields = append(indexedFields, defaultIndexNamespaced)
	}
	for _, f := range opts.Fields {
		indexedFields = append(indexedFields, toColumnName(f))
	}

	l := &ListOptionIndexer{
		Indexer:            i,
		namespaced:         opts.IsNamespaced,
		indexedFields:      indexedFields,
		maximumEventsCount: opts.MaximumEventsCount,
		watchers:           make(map[*watchKey]*watcher),
	}
	l.RegisterAfterAdd(l.addIndexFields)
	l.RegisterAfterAdd(l.addLabels)
	l.RegisterAfterAdd(l.notifyEventAdded)
	l.RegisterAfterAdd(l.deleteOldEvents)
	l.RegisterAfterUpdate(l.addIndexFields)
	l.RegisterAfterUpdate(l.addLabels)
	l.RegisterAfterUpdate(l.notifyEventModified)
	l.RegisterAfterUpdate(l.deleteOldEvents)
	l.RegisterAfterDelete(l.deleteFieldsByKey)
	l.RegisterAfterDelete(l.deleteLabelsByKey)
	l.RegisterAfterDelete(l.notifyEventDeleted)
	l.RegisterAfterDelete(l.deleteOldEvents)
	l.RegisterAfterDeleteAll(l.deleteFields)
	l.RegisterAfterDeleteAll(l.deleteLabels)
	columnDefs := make([]string, len(indexedFields))
	for index, field := range indexedFields {
		column := fmt.Sprintf(`"%s" TEXT`, field)
		columnDefs[index] = column
	}

	dbName := db.Sanitize(i.GetName())
	columns := make([]string, len(indexedFields))
	qmarks := make([]string, len(indexedFields))
	setStatements := make([]string, len(indexedFields))

	err = l.WithTransaction(ctx, true, func(tx transaction.Client) error {
		createEventsTableQuery := fmt.Sprintf(createEventsTableFmt, dbName)
		_, err = tx.Exec(createEventsTableQuery)
		if err != nil {
			return &db.QueryError{QueryString: createEventsTableFmt, Err: err}
		}

		_, err = tx.Exec(fmt.Sprintf(createFieldsTableFmt, dbName, strings.Join(columnDefs, ", ")))
		if err != nil {
			return err
		}

		for index, field := range indexedFields {
			// create index for field
			_, err = tx.Exec(fmt.Sprintf(createFieldsIndexFmt, dbName, field, dbName, field))
			if err != nil {
				return err
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
		createLabelsTableQuery := fmt.Sprintf(createLabelsTableFmt, dbName, dbName)
		_, err = tx.Exec(createLabelsTableQuery)
		if err != nil {
			return &db.QueryError{QueryString: createLabelsTableQuery, Err: err}
		}

		createLabelsTableIndexQuery := fmt.Sprintf(createLabelsTableIndexFmt, dbName, dbName)
		_, err = tx.Exec(createLabelsTableIndexQuery)
		if err != nil {
			return &db.QueryError{QueryString: createLabelsTableIndexQuery, Err: err}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	l.upsertEventsQuery = fmt.Sprintf(
		`REPLACE INTO "%s_events"(rv, type, event) VALUES (?, ?, ?)`,
		dbName,
	)
	l.upsertEventsStmt = l.Prepare(l.upsertEventsQuery)

	l.listEventsAfterQuery = fmt.Sprintf(listEventsAfterFmt, dbName)
	l.listEventsAfterStmt = l.Prepare(l.listEventsAfterQuery)

	l.findEventsRowByRVQuery = fmt.Sprintf(findEventsRowByRVFmt, dbName)
	l.findEventsRowByRVStmt = l.Prepare(l.findEventsRowByRVQuery)

	l.deleteEventsByCountQuery = fmt.Sprintf(deleteEventsByCountFmt, dbName, dbName)
	l.deleteEventsByCountStmt = l.Prepare(l.deleteEventsByCountQuery)

	l.addFieldsQuery = fmt.Sprintf(
		`INSERT INTO "%s_fields"(key, %s) VALUES (?, %s) ON CONFLICT DO UPDATE SET %s`,
		dbName,
		strings.Join(columns, ", "),
		strings.Join(qmarks, ", "),
		strings.Join(setStatements, ", "),
	)
	l.deleteFieldsByKeyQuery = fmt.Sprintf(`DELETE FROM "%s_fields" WHERE key = ?`, dbName)
	l.deleteFieldsQuery = fmt.Sprintf(deleteFieldsFmt, dbName)

	l.addFieldsStmt = l.Prepare(l.addFieldsQuery)
	l.deleteFieldsByKeyStmt = l.Prepare(l.deleteFieldsByKeyQuery)
	l.deleteFieldsStmt = l.Prepare(l.deleteFieldsQuery)

	l.upsertLabelsQuery = fmt.Sprintf(upsertLabelsStmtFmt, dbName)
	l.deleteLabelsByKeyQuery = fmt.Sprintf(deleteLabelsByKeyStmtFmt, dbName)
	l.deleteLabelsQuery = fmt.Sprintf(deleteLabelsStmtFmt, dbName)
	l.upsertLabelsStmt = l.Prepare(l.upsertLabelsQuery)
	l.deleteLabelsByKeyStmt = l.Prepare(l.deleteLabelsByKeyQuery)
	l.deleteLabelsStmt = l.Prepare(l.deleteLabelsQuery)

	return l, nil
}

func (l *ListOptionIndexer) Watch(ctx context.Context, opts WatchOptions, eventsCh chan<- watch.Event) error {
	l.latestRVLock.RLock()
	latestRV := l.latestRV
	l.latestRVLock.RUnlock()

	targetRV := opts.ResourceVersion
	if opts.ResourceVersion == "" {
		targetRV = latestRV
	}

	var events []watch.Event
	var key *watchKey
	// Even though we're not writing in this transaction, we prevent other writes to SQL
	// because we don't want to add more events while we're backfilling events, so we don't miss events
	err := l.WithTransaction(ctx, true, func(tx transaction.Client) error {
		rowIDRow := tx.Stmt(l.findEventsRowByRVStmt).QueryRowContext(ctx, targetRV)
		if err := rowIDRow.Err(); err != nil {
			return &db.QueryError{QueryString: l.findEventsRowByRVQuery, Err: err}
		}

		var rowID int
		err := rowIDRow.Scan(&rowID)
		if errors.Is(err, sql.ErrNoRows) {
			if targetRV != latestRV {
				return ErrTooOld
			}
		} else if err != nil {
			return fmt.Errorf("failed scan rowid: %w", err)
		}

		// Backfilling previous events from resourceVersion
		rows, err := tx.Stmt(l.listEventsAfterStmt).QueryContext(ctx, rowID)
		if err != nil {
			return &db.QueryError{QueryString: l.listEventsAfterQuery, Err: err}
		}
		defer rows.Close()

		for rows.Next() {
			var typ, rv string
			var buf sql.RawBytes
			err := rows.Scan(&typ, &rv, &buf)
			if err != nil {
				return fmt.Errorf("scanning event row: %w", err)
			}

			example := &unstructured.Unstructured{}
			val, err := fromBytes(buf, reflect.TypeOf(example))
			if err != nil {
				return fmt.Errorf("decoding event object: %w", err)
			}

			obj, ok := val.Elem().Interface().(runtime.Object)
			if !ok {
				continue
			}

			filter := opts.Filter
			if !matchFilter(filter.ID, filter.Namespace, filter.Selector, obj) {
				continue
			}

			events = append(events, watch.Event{
				Type:   watch.EventType(typ),
				Object: val.Elem().Interface().(runtime.Object),
			})
		}

		if err := rows.Err(); err != nil {
			return err
		}

		for _, event := range events {
			eventsCh <- event
		}

		key = l.addWatcher(eventsCh, opts.Filter)
		return nil
	})
	if err != nil {
		return err
	}

	<-ctx.Done()
	l.removeWatcher(key)
	return nil
}

func toBytes(obj any) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(obj)
	if err != nil {
		panic(fmt.Errorf("error while gobbing object: %w", err))
	}
	bb := buf.Bytes()
	return bb
}

func fromBytes(buf sql.RawBytes, typ reflect.Type) (reflect.Value, error) {
	dec := gob.NewDecoder(bytes.NewReader(buf))
	singleResult := reflect.New(typ)
	err := dec.DecodeValue(singleResult)
	return singleResult, err
}

type watchKey struct {
	_ bool // ensure watchKey is NOT zero-sized to get unique pointers
}

type watcher struct {
	ch     chan<- watch.Event
	filter WatchFilter
}

func (l *ListOptionIndexer) addWatcher(eventCh chan<- watch.Event, filter WatchFilter) *watchKey {
	key := new(watchKey)
	l.watchersLock.Lock()
	l.watchers[key] = &watcher{
		ch:     eventCh,
		filter: filter,
	}
	l.watchersLock.Unlock()
	return key
}

func (l *ListOptionIndexer) removeWatcher(key *watchKey) {
	l.watchersLock.Lock()
	delete(l.watchers, key)
	l.watchersLock.Unlock()
}

/* Core methods */

func (l *ListOptionIndexer) notifyEventAdded(key string, obj any, tx transaction.Client) error {
	return l.notifyEvent(watch.Added, nil, obj, tx)
}

func (l *ListOptionIndexer) notifyEventModified(key string, obj any, tx transaction.Client) error {
	oldObj, exists, err := l.GetByKey(key)
	if err != nil {
		return fmt.Errorf("error getting old object: %w", err)
	}

	if !exists {
		return fmt.Errorf("old object %q should be in store but was not", key)
	}

	return l.notifyEvent(watch.Modified, oldObj, obj, tx)
}

func (l *ListOptionIndexer) notifyEventDeleted(key string, obj any, tx transaction.Client) error {
	oldObj, exists, err := l.GetByKey(key)
	if err != nil {
		return fmt.Errorf("error getting old object: %w", err)
	}

	if !exists {
		return fmt.Errorf("old object %q should be in store but was not", key)
	}
	return l.notifyEvent(watch.Deleted, oldObj, obj, tx)
}

func (l *ListOptionIndexer) notifyEvent(eventType watch.EventType, oldObj any, obj any, tx transaction.Client) error {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	latestRV := acc.GetResourceVersion()
	_, err = tx.Stmt(l.upsertEventsStmt).Exec(latestRV, eventType, toBytes(obj))
	if err != nil {
		return &db.QueryError{QueryString: l.upsertEventsQuery, Err: err}
	}

	l.watchersLock.RLock()
	for _, watcher := range l.watchers {
		if !matchWatch(watcher.filter.ID, watcher.filter.Namespace, watcher.filter.Selector, oldObj, obj) {
			continue
		}

		watcher.ch <- watch.Event{
			Type:   eventType,
			Object: obj.(runtime.Object).DeepCopyObject(),
		}
	}
	l.watchersLock.RUnlock()

	l.latestRVLock.Lock()
	defer l.latestRVLock.Unlock()
	l.latestRV = latestRV
	return nil
}

func (l *ListOptionIndexer) deleteOldEvents(key string, obj any, tx transaction.Client) error {
	if l.maximumEventsCount == 0 {
		return nil
	}

	_, err := tx.Stmt(l.deleteEventsByCountStmt).Exec(l.maximumEventsCount)
	if err != nil {
		return &db.QueryError{QueryString: l.deleteEventsByCountQuery, Err: err}
	}
	return nil
}

// addIndexFields saves sortable/filterable fields into tables
func (l *ListOptionIndexer) addIndexFields(key string, obj any, tx transaction.Client) error {
	args := []any{key}
	for _, field := range l.indexedFields {
		value, err := getField(obj, field)
		if err != nil {
			logrus.Errorf("cannot index object of type [%s] with key [%s] for indexer [%s]: %v", l.GetType().String(), key, l.GetName(), err)
			return err
		}
		switch typedValue := value.(type) {
		case nil:
			args = append(args, "")
		case int, bool, string, int64, float64:
			args = append(args, fmt.Sprint(typedValue))
		case []string:
			args = append(args, strings.Join(typedValue, "|"))
		default:
			err2 := fmt.Errorf("field %v has a non-supported type value: %v", field, value)
			return err2
		}
	}

	_, err := tx.Stmt(l.addFieldsStmt).Exec(args...)
	if err != nil {
		return &db.QueryError{QueryString: l.addFieldsQuery, Err: err}
	}
	return nil
}

// labels are stored in tables that shadow the underlying object table for each GVK
func (l *ListOptionIndexer) addLabels(key string, obj any, tx transaction.Client) error {
	k8sObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("addLabels: unexpected object type, expected unstructured.Unstructured: %v", obj)
	}
	incomingLabels := k8sObj.GetLabels()
	for k, v := range incomingLabels {
		_, err := tx.Stmt(l.upsertLabelsStmt).Exec(key, k, v)
		if err != nil {
			return &db.QueryError{QueryString: l.upsertLabelsQuery, Err: err}
		}
	}
	return nil
}

func (l *ListOptionIndexer) deleteFieldsByKey(key string, _ any, tx transaction.Client) error {
	args := []any{key}

	_, err := tx.Stmt(l.deleteFieldsByKeyStmt).Exec(args...)
	if err != nil {
		return &db.QueryError{QueryString: l.deleteFieldsByKeyQuery, Err: err}
	}
	return nil
}

func (l *ListOptionIndexer) deleteFields(tx transaction.Client) error {
	_, err := tx.Stmt(l.deleteFieldsStmt).Exec()
	if err != nil {
		return &db.QueryError{QueryString: l.deleteFieldsQuery, Err: err}
	}
	return nil
}

func (l *ListOptionIndexer) deleteLabelsByKey(key string, _ any, tx transaction.Client) error {
	_, err := tx.Stmt(l.deleteLabelsByKeyStmt).Exec(key)
	if err != nil {
		return &db.QueryError{QueryString: l.deleteLabelsByKeyQuery, Err: err}
	}
	return nil
}

func (l *ListOptionIndexer) deleteLabels(tx transaction.Client) error {
	_, err := tx.Stmt(l.deleteLabelsStmt).Exec()
	if err != nil {
		return &db.QueryError{QueryString: l.deleteLabelsQuery, Err: err}
	}
	return nil
}

// ListByOptions returns objects according to the specified list options and partitions.
// Specifically:
//   - an unstructured list of resources belonging to any of the specified partitions
//   - the total number of resources (returned list might be a subset depending on pagination options in lo)
//   - a continue token, if there are more pages after the returned one
//   - an error instead of all of the above if anything went wrong
func (l *ListOptionIndexer) ListByOptions(ctx context.Context, lo *sqltypes.ListOptions, partitions []partition.Partition, namespace string) (*unstructured.UnstructuredList, int, string, error) {
	queryInfo, err := l.constructQuery(lo, partitions, namespace, db.Sanitize(l.GetName()))
	if err != nil {
		return nil, 0, "", err
	}
	return l.executeQuery(ctx, queryInfo)
}

// QueryInfo is a helper-struct that is used to represent the core query and parameters when converting
// a filter from the UI into a sql query
type QueryInfo struct {
	query       string
	params      []any
	countQuery  string
	countParams []any
	limit       int
	offset      int
}

func (l *ListOptionIndexer) constructQuery(lo *sqltypes.ListOptions, partitions []partition.Partition, namespace string, dbName string) (*QueryInfo, error) {
	ensureSortLabelsAreSelected(lo)
	queryInfo := &QueryInfo{}
	queryUsesLabels := hasLabelFilter(lo.Filters)
	joinTableIndexByLabelName := make(map[string]int)

	// First, what kind of filtering will we be doing?
	// 1- Intro: SELECT and JOIN clauses
	// There's a 1:1 correspondence between a base table and its _Fields table
	// but it's possible that a key has no associated labels, so if we're doing a
	// non-existence test on labels we need to do a LEFT OUTER JOIN
	distinctModifier := ""
	if queryUsesLabels {
		distinctModifier = " DISTINCT"
	}
	query := fmt.Sprintf(`SELECT%s o.object, o.objectnonce, o.dekid FROM "%s" o`, distinctModifier, dbName)
	query += "\n  "
	query += fmt.Sprintf(`JOIN "%s_fields" f ON o.key = f.key`, dbName)
	if queryUsesLabels {
		for i, orFilter := range lo.Filters {
			for j, filter := range orFilter.Filters {
				if isLabelFilter(&filter) {
					labelName := filter.Field[2]
					_, ok := joinTableIndexByLabelName[labelName]
					if !ok {
						// Make the lt index 1-based for readability
						jtIndex := i + j + 1
						joinTableIndexByLabelName[labelName] = jtIndex
						query += "\n  "
						query += fmt.Sprintf(`LEFT OUTER JOIN "%s_labels" lt%d ON o.key = lt%d.key`, dbName, jtIndex, jtIndex)
					}
				}
			}
		}
	}
	params := []any{}

	// 2- Filtering: WHERE clauses (from lo.Filters)
	whereClauses := []string{}
	for _, orFilters := range lo.Filters {
		orClause, orParams, err := l.buildORClauseFromFilters(orFilters, dbName, joinTableIndexByLabelName)
		if err != nil {
			return queryInfo, err
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
	for _, thisPartition := range partitions {
		if thisPartition.Passthrough {
			// nothing to do, no extra filtering to apply by definition
		} else {
			singlePartitionClauses := []string{}

			// filter by namespace
			if thisPartition.Namespace != "" && thisPartition.Namespace != "*" {
				singlePartitionClauses = append(singlePartitionClauses, fmt.Sprintf(`f."metadata.namespace" = ?`))
				params = append(params, thisPartition.Namespace)
			}

			// optionally filter by names
			if !thisPartition.All {
				names := thisPartition.Names

				if len(names) == 0 {
					// degenerate case, there will be no results
					singlePartitionClauses = append(singlePartitionClauses, "FALSE")
				} else {
					singlePartitionClauses = append(singlePartitionClauses, fmt.Sprintf(`f."metadata.name" IN (?%s)`, strings.Repeat(", ?", len(thisPartition.Names)-1)))
					// sort for reproducibility
					sortedNames := thisPartition.Names.UnsortedList()
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

	// before proceeding, save a copy of the query and params without LIMIT/OFFSET/ORDER info
	// for COUNTing all results later
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM (%s)", query)
	countParams := params[:]

	// 3- Sorting: ORDER BY clauses (from lo.Sort)
	if len(lo.SortList.SortDirectives) > 0 {
		orderByClauses := []string{}
		for _, sortDirective := range lo.SortList.SortDirectives {
			fields := sortDirective.Fields
			if isLabelsFieldList(fields) {
				clause, sortParam, err := buildSortLabelsClause(fields[2], joinTableIndexByLabelName, sortDirective.Order == sqltypes.ASC)
				if err != nil {
					return nil, err
				}
				orderByClauses = append(orderByClauses, clause)
				params = append(params, sortParam)
			} else {
				fieldEntry, err := l.getValidFieldEntry("f", fields)
				if err != nil {
					return queryInfo, err
				}
				direction := "ASC"
				if sortDirective.Order == sqltypes.DESC {
					direction = "DESC"
				}
				orderByClauses = append(orderByClauses, fmt.Sprintf("%s %s", fieldEntry, direction))
			}
		}
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

	// 4- Pagination: LIMIT clause (from lo.Pagination)

	limitClause := ""
	limit := lo.Pagination.PageSize
	if limit > 0 {
		limitClause = "\n  LIMIT ?"
		params = append(params, limit)
	}

	// OFFSET clause (from lo.Pagination)
	offsetClause := ""
	offset := 0
	if lo.Pagination.Page >= 1 {
		offset += lo.Pagination.PageSize * (lo.Pagination.Page - 1)
	}
	if offset > 0 {
		offsetClause = "\n  OFFSET ?"
		params = append(params, offset)
	}
	if limit > 0 || offset > 0 {
		query += limitClause
		query += offsetClause
		queryInfo.countQuery = countQuery
		queryInfo.countParams = countParams
		queryInfo.limit = limit
		queryInfo.offset = offset
	}
	// Otherwise leave these as default values and the executor won't do pagination work

	logrus.Debugf("ListOptionIndexer prepared statement: %v", query)
	logrus.Debugf("Params: %v", params)
	queryInfo.query = query
	queryInfo.params = params

	return queryInfo, nil
}

func (l *ListOptionIndexer) executeQuery(ctx context.Context, queryInfo *QueryInfo) (result *unstructured.UnstructuredList, total int, token string, err error) {
	stmt := l.Prepare(queryInfo.query)
	defer func() {
		cerr := l.CloseStmt(stmt)
		if cerr != nil {
			err = errors.Join(err, &db.QueryError{QueryString: queryInfo.query, Err: cerr})
		}
	}()

	var items []any
	err = l.WithTransaction(ctx, false, func(tx transaction.Client) error {
		txStmt := tx.Stmt(stmt)
		rows, err := txStmt.QueryContext(ctx, queryInfo.params...)
		if err != nil {
			return &db.QueryError{QueryString: queryInfo.query, Err: err}
		}
		items, err = l.ReadObjects(rows, l.GetType(), l.GetShouldEncrypt())
		if err != nil {
			return err
		}

		total = len(items)
		if queryInfo.countQuery != "" {
			countStmt := l.Prepare(queryInfo.countQuery)
			defer func() {
				cerr := l.CloseStmt(countStmt)
				if cerr != nil {
					err = errors.Join(err, &db.QueryError{QueryString: queryInfo.countQuery, Err: cerr})
				}
			}()
			txStmt := tx.Stmt(countStmt)
			rows, err := txStmt.QueryContext(ctx, queryInfo.countParams...)
			if err != nil {
				return &db.QueryError{QueryString: queryInfo.countQuery, Err: err}
			}
			total, err = l.ReadInt(rows)
			if err != nil {
				return fmt.Errorf("error reading query results: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, 0, "", err
	}

	continueToken := ""
	limit := queryInfo.limit
	offset := queryInfo.offset
	if limit > 0 && offset+len(items) < total {
		continueToken = fmt.Sprintf("%d", offset+limit)
	}

	l.latestRVLock.RLock()
	latestRV := l.latestRV
	l.latestRVLock.RUnlock()

	return toUnstructuredList(items, latestRV), total, continueToken, nil
}

func (l *ListOptionIndexer) validateColumn(column string) error {
	for _, v := range l.indexedFields {
		if v == column {
			return nil
		}
	}
	return fmt.Errorf("column is invalid [%s]: %w", column, ErrInvalidColumn)
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

func (l *ListOptionIndexer) getValidFieldEntry(prefix string, fields []string) (string, error) {
	columnName := toColumnName(fields)
	err := l.validateColumn(columnName)
	if err == nil {
		return fmt.Sprintf(`%s."%s"`, prefix, columnName), nil
	}
	if len(fields) <= 2 {
		return "", err
	}
	idx := -1
	for i := len(fields) - 1; i > 0; i-- {
		if !containsNonNumericRegex.MatchString(fields[i]) {
			idx = i
			break
		}
	}
	if idx == -1 {
		// We don't have an index onto a valid field
		return "", err
	}
	indexField := fields[idx]
	// fields[len(fields):] gives empty array
	otherFields := append(fields[0:idx], fields[idx+1:]...)
	leadingColumnName := toColumnName(otherFields)
	if l.validateColumn(leadingColumnName) != nil {
		// We have an index, but not onto a valid field
		return "", err
	}
	return fmt.Sprintf(`extractBarredValue(%s."%s", "%s")`, prefix, leadingColumnName, indexField), nil
}

// buildORClause creates an SQLite compatible query that ORs conditions built from passed filters
func (l *ListOptionIndexer) buildORClauseFromFilters(orFilters sqltypes.OrFilter, dbName string, joinTableIndexByLabelName map[string]int) (string, []any, error) {
	var params []any
	clauses := make([]string, 0, len(orFilters.Filters))
	var newParams []any
	var newClause string
	var err error

	for _, filter := range orFilters.Filters {
		if isLabelFilter(&filter) {
			index, ok := joinTableIndexByLabelName[filter.Field[2]]
			if !ok {
				return "", nil, fmt.Errorf("internal error: no index for label name %s", filter.Field[2])
			}
			newClause, newParams, err = l.getLabelFilter(index, filter, dbName)
		} else {
			newClause, newParams, err = l.getFieldFilter(filter)
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

func buildSortLabelsClause(labelName string, joinTableIndexByLabelName map[string]int, isAsc bool) (string, string, error) {
	ltIndex, ok := joinTableIndexByLabelName[labelName]
	if !ok {
		return "", "", fmt.Errorf(`internal error: no join-table index given for labelName "%s"`, labelName)
	}
	stmt := fmt.Sprintf(`CASE lt%d.label WHEN ? THEN lt%d.value ELSE NULL END`, ltIndex, ltIndex)
	dir := "ASC"
	nullsPosition := "LAST"
	if !isAsc {
		dir = "DESC"
		nullsPosition = "FIRST"
	}
	return fmt.Sprintf("(%s) %s NULLS %s", stmt, dir, nullsPosition), labelName, nil
}

// If the user tries to sort on a particular label without mentioning it in a query,
// it turns out that the sort-directive is ignored. It could be that the sqlite engine
// is doing some kind of optimization on the `select distinct`, but verifying an otherwise
// unreferenced label exists solves this problem.
// And it's better to do this by modifying the ListOptions object.
// There are no thread-safety issues in doing this because the ListOptions object is
// created in Store.ListByPartitions, and that ends up calling ListOptionIndexer.ConstructQuery.
// No other goroutines access this object.
func ensureSortLabelsAreSelected(lo *sqltypes.ListOptions) {
	if len(lo.SortList.SortDirectives) == 0 {
		return
	}
	unboundSortLabels := make(map[string]bool)
	for _, sortDirective := range lo.SortList.SortDirectives {
		fields := sortDirective.Fields
		if isLabelsFieldList(fields) {
			unboundSortLabels[fields[2]] = true
		}
	}
	if len(unboundSortLabels) == 0 {
		return
	}
	// If we have sort directives but no filters, add an exists-filter for each label.
	if lo.Filters == nil || len(lo.Filters) == 0 {
		lo.Filters = make([]sqltypes.OrFilter, 1)
		lo.Filters[0].Filters = make([]sqltypes.Filter, len(unboundSortLabels))
		i := 0
		for labelName := range unboundSortLabels {
			lo.Filters[0].Filters[i] = sqltypes.Filter{
				Field: []string{"metadata", "labels", labelName},
				Op:    sqltypes.Exists,
			}
			i++
		}
		return
	}
	// The gotcha is we have to bind the labels for each set of orFilters, so copy them each time
	for i, orFilters := range lo.Filters {
		copyUnboundSortLabels := make(map[string]bool, len(unboundSortLabels))
		for k, v := range unboundSortLabels {
			copyUnboundSortLabels[k] = v
		}
		for _, filter := range orFilters.Filters {
			if isLabelFilter(&filter) {
				copyUnboundSortLabels[filter.Field[2]] = false
			}
		}
		// Now for any labels that are still true, add another where clause
		for labelName, needsBinding := range copyUnboundSortLabels {
			if needsBinding {
				// `orFilters` is a copy of lo.Filters[i], so reference the original.
				lo.Filters[i].Filters = append(lo.Filters[i].Filters, sqltypes.Filter{
					Field: []string{"metadata", "labels", labelName},
					Op:    sqltypes.Exists,
				})
			}
		}
	}
}

// Possible ops from the k8s parser:
// KEY = and == (same) VALUE
// KEY != VALUE
// KEY exists []  # ,KEY, => this filter
// KEY ! []  # ,!KEY, => assert KEY doesn't exist
// KEY in VALUES
// KEY notin VALUES

func (l *ListOptionIndexer) getFieldFilter(filter sqltypes.Filter) (string, []any, error) {
	opString := ""
	escapeString := ""
	fieldEntry, err := l.getValidFieldEntry("f", filter.Field)
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
		return clause, []any{formatMatchTarget(filter)}, nil
	case sqltypes.NotEq:
		if filter.Partial {
			opString = "NOT LIKE"
			escapeString = escapeBackslashDirective
		} else {
			opString = "!="
		}
		clause := fmt.Sprintf("%s %s ?%s", fieldEntry, opString, escapeString)
		return clause, []any{formatMatchTarget(filter)}, nil

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
	}

	return "", nil, fmt.Errorf("unrecognized operator: %s", opString)
}

func (l *ListOptionIndexer) getLabelFilter(index int, filter sqltypes.Filter, dbName string) (string, []any, error) {
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
		existenceClause, subParams, err := l.getLabelFilter(index, subFilter, dbName)
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
		clause := fmt.Sprintf(`o.key NOT IN (SELECT o1.key FROM "%s" o1
		JOIN "%s_fields" f1 ON o1.key = f1.key
		LEFT OUTER JOIN "%s_labels" lt%di1 ON o1.key = lt%di1.key
		WHERE lt%di1.label = ?)`, dbName, dbName, dbName, index, index, index)
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
		existenceClause, subParams, err := l.getLabelFilter(index, subFilter, dbName)
		if err != nil {
			return "", nil, err
		}
		clause := fmt.Sprintf(`(%s) OR (lt%d.label = ? AND lt%d.value NOT IN %s)`, existenceClause, index, index, target)
		matches := append(subParams, labelName)
		for _, match := range filter.Matches {
			matches = append(matches, match)
		}
		return clause, matches, nil
	}
	return "", nil, fmt.Errorf("unrecognized operator: %s", opString)
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

func extractSubFields(fields string) []string {
	subfields := make([]string, 0)
	for _, subField := range subfieldRegex.FindAllString(fields, -1) {
		subfields = append(subfields, strings.TrimSuffix(subField, "."))
	}
	return subfields
}

func isLabelFilter(f *sqltypes.Filter) bool {
	return len(f.Field) >= 2 && f.Field[0] == "metadata" && f.Field[1] == "labels"
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

func isLabelsFieldList(fields []string) bool {
	return len(fields) == 3 && fields[0] == "metadata" && fields[1] == "labels"
}

// toUnstructuredList turns a slice of unstructured objects into an unstructured.UnstructuredList
func toUnstructuredList(items []any, resourceVersion string) *unstructured.UnstructuredList {
	objectItems := make([]any, len(items))
	result := &unstructured.UnstructuredList{
		Items:  make([]unstructured.Unstructured, len(items)),
		Object: map[string]interface{}{"items": objectItems},
	}
	if resourceVersion != "" {
		result.SetResourceVersion(resourceVersion)
	}
	for i, item := range items {
		result.Items[i] = *item.(*unstructured.Unstructured)
		objectItems[i] = item.(*unstructured.Unstructured).Object
	}
	return result
}

func matchWatch(filterName string, filterNamespace string, filterSelector labels.Selector, oldObj any, obj any) bool {
	matchOld := false
	if oldObj != nil {
		matchOld = matchFilter(filterName, filterNamespace, filterSelector, oldObj)
	}
	return matchOld || matchFilter(filterName, filterNamespace, filterSelector, obj)
}

func matchFilter(filterName string, filterNamespace string, filterSelector labels.Selector, obj any) bool {
	if obj == nil {
		return false
	}
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return false
	}
	if filterName != "" && filterName != metadata.GetName() {
		return false
	}
	if filterNamespace != "" && filterNamespace != metadata.GetNamespace() {
		return false
	}
	if filterSelector != nil {
		if !filterSelector.Matches(labels.Set(metadata.GetLabels())) {
			return false
		}
	}
	return true
}
