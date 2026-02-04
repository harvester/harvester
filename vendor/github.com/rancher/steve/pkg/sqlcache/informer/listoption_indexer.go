package informer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/partition"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

// ListOptionIndexer extends Indexer by allowing queries based on ListOption
type ListOptionIndexer struct {
	*Indexer

	namespaced    bool
	indexedFields []string

	// lock protects both latestRV and watchers
	lock     sync.RWMutex
	latestRV string
	watchers map[*watchKey]*watcher

	// gcInterval is how often to run the garbage collection
	gcInterval time.Duration
	// gcKeepCount is how many events to keep in _events table when gc runs
	gcKeepCount int

	upsertEventsStmt        db.Stmt
	findEventsRowByRVStmt   db.Stmt
	listEventsAfterStmt     db.Stmt
	deleteEventsByCountStmt db.Stmt
	dropEventsStmt          db.Stmt
	addFieldsStmt           db.Stmt
	deleteFieldsStmt        db.Stmt
	dropFieldsStmt          db.Stmt
	upsertLabelsStmt        db.Stmt
	deleteLabelsStmt        db.Stmt
	dropLabelsStmt          db.Stmt
}

var (
	defaultIndexedFields   = []string{"metadata.name", "metadata.creationTimestamp"}
	defaultIndexNamespaced = "metadata.namespace"
	immutableFields        = sets.New(
		"metadata.creationTimestamp",
		"metadata.namespace",
		"metadata.name",
		"id",
	)

	ErrTooOld = errors.New("resourceversion too old")
)

const (

	// RV stands for ResourceVersion
	createEventsTableFmt = `CREATE TABLE "%s_events" (
                       rv TEXT NOT NULL,
                       type TEXT NOT NULL,
                       event BLOB NOT NULL,
                       eventnonce BLOB,
	               dekid BLOB,
                       PRIMARY KEY (rv, type)
          )`
	listEventsAfterFmt = `SELECT type, rv, event, eventnonce, dekid
	       FROM "%s_events"
	       WHERE rowid > ?
       `
	findEventsRowByRVFmt = `SELECT rowid
               FROM "%s_events"
               WHERE rv = ?
       `
	upsertEventsStmtFmt = `
INSERT INTO "%s_events" (rv, type, event, eventnonce, dekid)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(type, rv) DO UPDATE SET
  event = excluded.event,
  eventnonce = excluded.eventnonce,
  dekid = excluded.dekid`
	deleteEventsByCountFmt = `DELETE FROM "%s_events"
	WHERE rowid < (
	    SELECT MIN(rowid) FROM (
	        SELECT rowid FROM "%s_events" ORDER BY rowid DESC LIMIT ?
	    ) q
	)`
	dropEventsFmt = `DROP TABLE IF EXISTS "%s_events"`

	createFieldsTableFmt = `CREATE TABLE "%s_fields" (
		key TEXT NOT NULL REFERENCES "%s"(key) ON DELETE CASCADE,
		%s,
		PRIMARY KEY (key)
    )`
	createFieldsIndexFmt = `CREATE INDEX "%s_%s_index" ON "%s_fields"("%s")`
	deleteFieldsFmt      = `DELETE FROM "%s_fields"`
	dropFieldsFmt        = `DROP TABLE IF EXISTS "%s_fields"`

	createLabelsTableFmt = `CREATE TABLE IF NOT EXISTS "%s_labels" (
		key TEXT NOT NULL REFERENCES "%s"(key) ON DELETE CASCADE,
		label TEXT NOT NULL,
		value TEXT NOT NULL,
		PRIMARY KEY (key, label)
	)`
	createLabelsTableIndexFmt = `CREATE INDEX IF NOT EXISTS "%s_labels_index" ON "%s_labels"(label, value)`

	upsertLabelsStmtFmt = `
INSERT INTO "%s_labels" (key, label, value)
VALUES (?, ?, ?)
ON CONFLICT(key, label) DO UPDATE SET
  value = excluded.value`
	deleteLabelsStmtFmt = `DELETE FROM "%s_labels"`
	dropLabelsStmtFmt   = `DROP TABLE IF EXISTS "%s_labels"`
)

type ListOptionIndexerOptions struct {
	// Fields is a list of fields within the object that we want indexed for
	// filtering & sorting. Each field is specified as a slice.
	//
	// For example, .metadata.resourceVersion should be specified as []string{"metadata", "resourceVersion"}
	Fields [][]string
	// Used for specifying types of non-TEXT database fields.
	// The key is a fully-qualified field name, like 'metadata.fields[1]'.
	// The value is a type name, most likely "INT" but could be "REAL". The default type is "TEXT",
	// and we don't (currently) use NULL or BLOB types.
	TypeGuidance map[string]string
	// IsNamespaced determines whether the GVK for this ListOptionIndexer is
	// namespaced
	IsNamespaced bool
	// GCInterval is how often to run the garbage collection
	GCInterval time.Duration
	// GCKeepCount is how many events to keep in _events table when gc runs
	GCKeepCount int
}

// NewListOptionIndexer returns a SQLite-backed cache.Indexer of unstructured.Unstructured Kubernetes resources of a certain GVK
// ListOptionIndexer is also able to satisfy ListOption queries on indexed (sub)fields.
func NewListOptionIndexer(ctx context.Context, s Store, opts ListOptionIndexerOptions) (*ListOptionIndexer, error) {
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
		Indexer:       i,
		namespaced:    opts.IsNamespaced,
		indexedFields: indexedFields,
		watchers:      make(map[*watchKey]*watcher),
	}
	l.RegisterAfterAdd(l.addIndexFields)
	l.RegisterAfterAdd(l.addLabels)
	l.RegisterAfterAdd(l.notifyEventAdded)
	l.RegisterAfterUpdate(l.addIndexFields)
	l.RegisterAfterUpdate(l.addLabels)
	l.RegisterAfterUpdate(l.notifyEventModified)
	l.RegisterAfterDelete(l.notifyEventDeleted)
	l.RegisterAfterDeleteAll(l.deleteFields)
	l.RegisterAfterDeleteAll(l.deleteLabels)
	l.RegisterBeforeDropAll(l.dropEvents)
	l.RegisterBeforeDropAll(l.dropLabels)
	l.RegisterBeforeDropAll(l.dropFields)
	columnDefs := make([]string, len(indexedFields))
	for index, field := range indexedFields {
		typeName := "TEXT"
		newTypeName, ok := opts.TypeGuidance[field]
		if ok {
			typeName = newTypeName
		}
		column := fmt.Sprintf(`"%s" %s`, field, typeName)
		columnDefs[index] = column
	}

	dbName := db.Sanitize(i.GetName())
	columns := make([]string, 0, len(indexedFields))
	qmarks := make([]string, 0, len(indexedFields))
	setStatements := make([]string, 0, len(indexedFields))

	err = l.WithTransaction(ctx, true, func(tx db.TxClient) error {
		dropEventsQuery := fmt.Sprintf(dropEventsFmt, dbName)
		if _, err := tx.Exec(dropEventsQuery); err != nil {
			return err
		}

		createEventsTableQuery := fmt.Sprintf(createEventsTableFmt, dbName)
		if _, err := tx.Exec(createEventsTableQuery); err != nil {
			return err
		}

		dropFieldsQuery := fmt.Sprintf(dropFieldsFmt, dbName)
		if _, err := tx.Exec(dropFieldsQuery); err != nil {
			return err
		}

		createFieldsTableQuery := fmt.Sprintf(createFieldsTableFmt, dbName, dbName, strings.Join(columnDefs, ", "))
		if _, err := tx.Exec(createFieldsTableQuery); err != nil {
			return err
		}

		for _, field := range indexedFields {
			// create index for field
			createFieldsIndexQuery := fmt.Sprintf(createFieldsIndexFmt, dbName, field, dbName, field)
			if _, err := tx.Exec(createFieldsIndexQuery); err != nil {
				return err
			}

			// format field into column for prepared statement
			column := fmt.Sprintf(`"%s"`, field)
			columns = append(columns, column)

			// add placeholder for column's value in prepared statement
			qmarks = append(qmarks, "?")

			// add formatted set statement for prepared statement
			// optimization: avoid SET for fields which cannot change
			if !immutableFields.Has(field) {
				setStatement := fmt.Sprintf(`"%s" = excluded."%s"`, field, field)
				setStatements = append(setStatements, setStatement)
			}
		}

		dropLabelsQuery := fmt.Sprintf(dropLabelsStmtFmt, dbName)
		if _, err := tx.Exec(dropLabelsQuery); err != nil {
			return err
		}

		createLabelsTableQuery := fmt.Sprintf(createLabelsTableFmt, dbName, dbName)
		if _, err := tx.Exec(createLabelsTableQuery); err != nil {
			return err
		}

		createLabelsTableIndexQuery := fmt.Sprintf(createLabelsTableIndexFmt, dbName, dbName)
		if _, err := tx.Exec(createLabelsTableIndexQuery); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	l.upsertEventsStmt = l.Prepare(fmt.Sprintf(upsertEventsStmtFmt, dbName))
	l.listEventsAfterStmt = l.Prepare(fmt.Sprintf(listEventsAfterFmt, dbName))
	l.findEventsRowByRVStmt = l.Prepare(fmt.Sprintf(findEventsRowByRVFmt, dbName))
	l.deleteEventsByCountStmt = l.Prepare(fmt.Sprintf(deleteEventsByCountFmt, dbName, dbName))
	l.dropEventsStmt = l.Prepare(fmt.Sprintf(dropEventsFmt, dbName))

	addFieldsOnConflict := "NOTHING"
	if len(setStatements) > 0 {
		addFieldsOnConflict = "UPDATE SET " + strings.Join(setStatements, ", ")
	}
	l.addFieldsStmt = l.Prepare(fmt.Sprintf(
		`INSERT INTO "%s_fields"(key, %s) VALUES (?, %s) ON CONFLICT DO %s`,
		dbName,
		strings.Join(columns, ", "),
		strings.Join(qmarks, ", "),
		addFieldsOnConflict,
	))
	l.deleteFieldsStmt = l.Prepare(fmt.Sprintf(deleteFieldsFmt, dbName))
	l.dropFieldsStmt = l.Prepare(fmt.Sprintf(dropFieldsFmt, dbName))

	l.upsertLabelsStmt = l.Prepare(fmt.Sprintf(upsertLabelsStmtFmt, dbName))
	l.deleteLabelsStmt = l.Prepare(fmt.Sprintf(deleteLabelsStmtFmt, dbName))
	l.dropLabelsStmt = l.Prepare(fmt.Sprintf(dropLabelsStmtFmt, dbName))

	l.gcInterval = opts.GCInterval
	l.gcKeepCount = opts.GCKeepCount

	return l, nil
}

func (l *ListOptionIndexer) GetLatestResourceVersion() []string {
	var latestRV []string

	l.lock.RLock()
	latestRV = []string{l.latestRV}
	l.lock.RUnlock()

	return latestRV
}

func (l *ListOptionIndexer) Watch(ctx context.Context, opts WatchOptions, eventsCh chan<- watch.Event) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// We can keep receiving events while replaying older events for the watcher.
	// By early registering this watcher, this channel will buffer any new events while we are still backfilling old events.
	// When we finish, calling backfillDone will write all events in the buffer, then listen to new events as normal.
	const maxBufferSize = 100
	watcherChannel, backfillDone, closeWatcher := watcherWithBackfill(ctx, eventsCh, maxBufferSize)
	defer closeWatcher()

	l.lock.Lock()
	latestRV := l.latestRV
	key := l.addWatcherLocked(watcherChannel, opts.Filter)
	l.lock.Unlock()
	defer l.removeWatcher(key)

	targetRV := opts.ResourceVersion
	if targetRV == "" {
		targetRV = latestRV
	}

	if err := l.WithTransaction(ctx, false, func(tx db.TxClient) error {
		var rowID int
		// use a closure to ensure rows is always closed immediately after it's needed
		if err := func() error {
			rows, err := l.QueryForRows(ctx, tx.Stmt(l.findEventsRowByRVStmt), targetRV)
			if err != nil {
				return err
			}
			defer rows.Close()

			if !rows.Next() {
				// query returned no results
				if targetRV != latestRV {
					return ErrTooOld
				}
				return nil
			}
			if err := rows.Scan(&rowID); err != nil {
				return fmt.Errorf("failed scan rowid: %w", err)
			}
			return nil
		}(); err != nil {
			return err
		}

		// Backfilling previous events from resourceVersion
		rows, err := l.QueryForRows(ctx, tx.Stmt(l.listEventsAfterStmt), rowID)
		if err != nil {
			return err
		}
		defer rows.Close()

		var latestRevisionReached bool
		for !latestRevisionReached && rows.Next() {
			obj := &unstructured.Unstructured{}
			eventType, err := l.decryptScanEvent(rows, obj)
			if err != nil {
				return fmt.Errorf("scanning event row: %w", err)
			}
			if obj.GetResourceVersion() == latestRV {
				// This iteration will be the last one, as we already reached the last event at the moment we started the loop
				latestRevisionReached = true
			}
			filter := opts.Filter
			if !matchFilter(filter.ID, filter.Namespace, filter.Selector, obj) {
				continue
			}

			ev := watch.Event{
				Type:   eventType,
				Object: obj,
			}
			select {
			case eventsCh <- ev:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return rows.Err()
	}); err != nil {
		return err
	}
	backfillDone()

	<-ctx.Done()
	return nil
}

func (l *ListOptionIndexer) decryptScanEvent(rows db.Rows, into runtime.Object) (watch.EventType, error) {
	var typ, rv string
	var serialized db.SerializedObject
	if err := rows.Scan(&typ, &rv, &serialized.Bytes, &serialized.Nonce, &serialized.KeyID); err != nil {
		return watch.Error, err
	}
	if err := l.Deserialize(serialized, into); err != nil {
		return watch.Error, err

	}
	return watch.EventType(typ), nil
}

// watcherWithBackfill creates a proxy channel that buffers events during a "backfill" phase
// and then seamlessly transitions to live event processing.
func watcherWithBackfill[T any](ctx context.Context, eventsCh chan<- T, maxBufferSize int) (chan T, func(), func()) {
	backfillCtx, signalBackfillDone := context.WithCancel(ctx)
	watcherCh := make(chan T)
	done := make(chan struct{})

	// The single proxy goroutine that manages all state.
	go func() {
		defer close(done)
		defer func() {
			// this goroutine can exit prematurely when the parent context is cancelled
			// this ensures the producer can finish writing and finish the cancellation sequence (closeWatcher is called from the parent)
			for range watcherCh {
			}
		}()

		var queue []T // Use a slice as an internal FIFO queue.

		// Phase 1: Accumulate while we're backfilling
	acc:
		for len(queue) < maxBufferSize { // Only accumulate until reaching max buffer size, then block ingestion instead
			select {
			case event, ok := <-watcherCh:
				if !ok {
					// writeChan was closed, assume that context is done, so the remaining queue will never be sent
					return
				}
				queue = append(queue, event)
			case <-backfillCtx.Done():
				break acc
			}
		}

		// Check backfill was completed, in case the above loop aborted early
		<-backfillCtx.Done()
		if ctx.Err() != nil {
			return
		}

		// Phase 2: start flushing while still accepting events from watcherCh
		for len(queue) > 0 {
			// Only accept new events from write buffer if the queue has space, blocking the sender (equivalent to a full buffered channel)
			// cases reading from a nil channel will be ignored
			var readChan <-chan T
			if len(queue) < maxBufferSize {
				readChan = watcherCh
			}

			select {
			case <-ctx.Done():
				return
			case event, ok := <-readChan: // This case is disabled if readChan is nil (queue is full)
				if !ok {
					// watcherCh was closed, assume that context is done, so the remaining queue will never be sent
					return
				}
				queue = append(queue, event)
			case eventsCh <- queue[0]:
				// We successfully sent the event, so we can remove it from the queue.
				queue = queue[1:]
			}
		}
		queue = nil // no longer needed, release the backing array for GC

		// Final phase: when flushing is completed, the original channel is piped to watcherCh
		for {
			select {
			case event, ok := <-watcherCh:
				if !ok {
					return // watcherCh was closed.
				}
				// Send event directly, blocking until the consumer is ready.
				select {
				case eventsCh <- event:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return watcherCh, signalBackfillDone, func() {
		close(watcherCh)
		<-done
	}
}

type watchKey struct {
	_ bool // ensure watchKey is NOT zero-sized to get unique pointers
}

type watcher struct {
	ch     chan<- watch.Event
	filter WatchFilter
}

func (l *ListOptionIndexer) addWatcherLocked(eventCh chan<- watch.Event, filter WatchFilter) *watchKey {
	key := new(watchKey)
	l.watchers[key] = &watcher{
		ch:     eventCh,
		filter: filter,
	}
	return key
}

func (l *ListOptionIndexer) removeWatcher(key *watchKey) {
	l.lock.Lock()
	delete(l.watchers, key)
	l.lock.Unlock()
}

/* Core methods */

func (l *ListOptionIndexer) notifyEventAdded(key string, obj any, tx db.TxClient) error {
	return l.notifyEvent(watch.Added, nil, obj, tx)
}

func (l *ListOptionIndexer) notifyEventModified(key string, obj any, tx db.TxClient) error {
	oldObj, exists, err := l.GetByKey(key)
	if err != nil {
		return fmt.Errorf("error getting old object: %w", err)
	}

	if !exists {
		return fmt.Errorf("old object %q should be in store but was not", key)
	}

	return l.notifyEvent(watch.Modified, oldObj, obj, tx)
}

func (l *ListOptionIndexer) notifyEventDeleted(key string, obj any, tx db.TxClient) error {
	oldObj, exists, err := l.GetByKey(key)
	if err != nil {
		return fmt.Errorf("error getting old object: %w", err)
	}

	if !exists {
		return fmt.Errorf("old object %q should be in store but was not", key)
	}
	return l.notifyEvent(watch.Deleted, oldObj, obj, tx)
}

func (l *ListOptionIndexer) notifyEvent(eventType watch.EventType, oldObj any, obj any, tx db.TxClient) error {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	latestRV := acc.GetResourceVersion()

	err = l.upsertEvent(tx, eventType, latestRV, obj)
	if err != nil {
		return err
	}

	l.lock.RLock()
	for _, watcher := range l.watchers {
		if !matchWatch(watcher.filter.ID, watcher.filter.Namespace, watcher.filter.Selector, oldObj, obj) {
			continue
		}

		watcher.ch <- watch.Event{
			Type:   eventType,
			Object: obj.(runtime.Object).DeepCopyObject(),
		}
	}
	l.lock.RUnlock()

	l.lock.Lock()
	defer l.lock.Unlock()
	l.latestRV = latestRV
	return nil
}

func (l *ListOptionIndexer) upsertEvent(tx db.TxClient, eventType watch.EventType, latestRV string, obj any) error {
	serialized, err := l.Serialize(obj, l.GetShouldEncrypt())
	if err != nil {
		return err
	}
	_, err = tx.Stmt(l.upsertEventsStmt).Exec(latestRV, eventType, serialized.Bytes, serialized.Nonce, serialized.KeyID)
	return err
}

func (l *ListOptionIndexer) dropEvents(tx db.TxClient) error {
	_, err := tx.Stmt(l.dropEventsStmt).Exec()
	return err
}

// addIndexFields saves sortable/filterable fields into tables
func (l *ListOptionIndexer) addIndexFields(key string, obj any, tx db.TxClient) error {
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
	return err
}

// labels are stored in tables that shadow the underlying object table for each GVK
func (l *ListOptionIndexer) addLabels(key string, obj any, tx db.TxClient) error {
	k8sObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("addLabels: unexpected object type, expected unstructured.Unstructured: %v", obj)
	}
	incomingLabels := k8sObj.GetLabels()
	for k, v := range incomingLabels {
		if _, err := tx.Stmt(l.upsertLabelsStmt).Exec(key, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (l *ListOptionIndexer) deleteFields(tx db.TxClient) error {
	_, err := tx.Stmt(l.deleteFieldsStmt).Exec()
	return err
}

func (l *ListOptionIndexer) dropFields(tx db.TxClient) error {
	_, err := tx.Stmt(l.dropFieldsStmt).Exec()
	return err
}

func (l *ListOptionIndexer) deleteLabels(tx db.TxClient) error {
	_, err := tx.Stmt(l.deleteLabelsStmt).Exec()
	return err
}

func (l *ListOptionIndexer) dropLabels(tx db.TxClient) error {
	_, err := tx.Stmt(l.dropLabelsStmt).Exec()
	return err
}

// ListByOptions returns objects according to the specified list options and partitions.
// Specifically:
//   - an unstructured list of resources belonging to any of the specified partitions
//   - the total number of resources (returned list might be a subset depending on pagination options in lo)
//   - a summary object, containing the possible values for each field specified in a summary= subquery
//   - a continue token, if there are more pages after the returned one
//   - an error instead of all of the above if anything went wrong
func (l *ListOptionIndexer) ListByOptions(ctx context.Context, lo *sqltypes.ListOptions, partitions []partition.Partition, namespace string) (list *unstructured.UnstructuredList, total int, summary *types.APISummary, continueToken string, err error) {
	dbName := db.Sanitize(l.GetName())
	if len(lo.SummaryFieldList) > 0 {
		if summary, err = l.ListSummaryFields(ctx, lo, partitions, dbName, namespace); err != nil {
			return
		}
	}
	var queryInfo *QueryInfo
	if queryInfo, err = l.constructQuery(lo, partitions, namespace, dbName); err != nil {
		return
	}
	logrus.Debugf("ListOptionIndexer prepared statement: %v", queryInfo.query)
	logrus.Debugf("Params: %v", queryInfo.params)
	logrus.Tracef("ListOptionIndexer prepared count-statement: %v", queryInfo.countQuery)
	logrus.Tracef("Params: %v", queryInfo.countParams)
	list, total, continueToken, err = l.executeQuery(ctx, queryInfo)
	return
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

func (l *ListOptionIndexer) executeQuery(ctx context.Context, queryInfo *QueryInfo) (result *unstructured.UnstructuredList, total int, token string, err error) {
	stmt := l.Prepare(queryInfo.query)
	defer func() {
		if cerr := stmt.Close(); cerr != nil && err == nil {
			err = errors.Join(err, cerr)
		}
	}()

	var items []any
	err = l.WithTransaction(ctx, false, func(tx db.TxClient) error {
		now := time.Now()
		rows, err := l.QueryForRows(ctx, tx.Stmt(stmt), queryInfo.params...)
		if err != nil {
			return err
		}
		elapsed := time.Since(now)
		logLongQuery(elapsed, queryInfo.query, queryInfo.params)
		items, err = l.ReadObjects(rows, l.GetType())
		if err != nil {
			return fmt.Errorf("read objects: %w", err)
		}

		total = len(items)
		if queryInfo.countQuery != "" {
			countStmt := l.Prepare(queryInfo.countQuery)
			defer func() {
				if cerr := countStmt.Close(); cerr != nil {
					err = errors.Join(err, cerr)
				}
			}()
			now = time.Now()
			rows, err := l.QueryForRows(ctx, tx.Stmt(countStmt), queryInfo.countParams...)
			if err != nil {
				return err
			}
			elapsed = time.Since(now)
			logLongQuery(elapsed, queryInfo.countQuery, queryInfo.countParams)
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

	l.lock.RLock()
	latestRV := l.latestRV
	l.lock.RUnlock()

	return toUnstructuredList(items, latestRV), total, continueToken, nil
}

func logLongQuery(elapsed time.Duration, query string, params []any) {
	threshold := 500 * time.Millisecond
	if elapsed < threshold {
		return
	}
	logrus.Debugf("Query took more than %v (took %v): %s with params %v", threshold, elapsed, query, params)
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

func (l *ListOptionIndexer) RunGC(ctx context.Context) {
	if l.gcInterval == 0 || l.gcKeepCount == 0 {
		return
	}

	ticker := time.NewTicker(l.gcInterval)
	defer ticker.Stop()

	logrus.Infof("Started SQL cache garbage collection for %s (interval=%s, keep=%d)", l.GetName(), l.gcInterval, l.gcKeepCount)
	defer logrus.Infof("Stopped SQL cache garbage collection for %s (interval=%s, keep=%d)", l.GetName(), l.gcInterval, l.gcKeepCount)

	for {
		select {
		case <-ticker.C:
			err := l.WithTransaction(ctx, true, func(tx db.TxClient) error {
				_, err := tx.Stmt(l.deleteEventsByCountStmt).Exec(l.gcKeepCount)
				return err
			})
			if err != nil {
				logrus.Errorf("garbage collection for %s: %v", l.GetName(), err)
			}
		case <-ctx.Done():
			return
		}
	}
}
