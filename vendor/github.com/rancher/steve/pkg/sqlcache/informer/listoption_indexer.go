package informer

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
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
	subfieldRegex           = regexp.MustCompile(`([a-zA-Z]+)|(\[[-a-zA-Z./]+])|(\[[0-9]+])`)
	containsNonNumericRegex = regexp.MustCompile(`\D`)

	ErrInvalidColumn   = errors.New("supplied column is invalid")
	ErrTooOld          = errors.New("resourceversion too old")
	ErrUnknownRevision = errors.New("unknown revision")

	projectIDFieldLabel = "field.cattle.io/projectId"
	namespacesDbName    = "_v1_Namespace"
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

	failedToGetFromSliceFmt = "[listoption indexer] failed to get subfield [%s] from slice items"

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
		// Close returns both the iteration error and the close error; it
		// must be called before reading any iteration error to release the
		// closemu RLock that decryptScanEvent's Scan(*sql.RawBytes) kept
		// open across return (see db.Rows interface doc).
		return rows.Close()
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
		case map[string]any:
			isPod := false
			if u, ok := obj.(*unstructured.Unstructured); ok {
				isPod = u.GroupVersionKind().Kind == "Pod"
			}
			if isPod {
				// The pod restart transform func will split pod restart column (eg: `X (D ago)`)
				// into two fields. We want to sort / filter on count.
				if count, ok := typedValue["count"]; ok {
					switch c := count.(type) {
					case int64:
						args = append(args, c)
					case int:
						args = append(args, fmt.Sprint(c))
					case float64:
						args = append(args, fmt.Sprint(int64(c)))
					default:
						args = append(args, "")
					}
				} else {
					args = append(args, "")
				}
			} else {
				args = append(args, "")
			}
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
	unboundSortLabels := getUnboundSortLabels(lo)
	queryInfo := &QueryInfo{}
	queryUsesLabels := hasLabelFilter(lo.Filters) || len(lo.ProjectsOrNamespaces.Filters) > 0
	joinTableIndexByLabelName := make(map[string]int)

	l.lock.RLock()
	latestRV := l.latestRV
	l.lock.RUnlock()

	if len(lo.Revision) > 0 {
		currentRevision, err := strconv.ParseInt(latestRV, 10, 64)
		if err != nil {
			return nil, err
		}

		requestRevision, err := strconv.ParseInt(lo.Revision, 10, 64)
		if err != nil {
			return nil, err
		}

		if currentRevision < requestRevision {
			return nil, ErrUnknownRevision
		}
	}

	// First, what kind of filtering will we be doing?
	// 1- Intro: SELECT and JOIN clauses
	// There's a 1:1 correspondence between a base table and its _Fields table
	// but it's possible that a key has no associated labels, so if we're doing a
	// non-existence test on labels we need to do a LEFT OUTER JOIN
	query := ""
	params := []any{}
	whereClauses := []string{}
	joinPartsToUse := []string{}
	if len(unboundSortLabels) > 0 {
		withParts, withParams, _, joinParts, err := getWithParts(unboundSortLabels, joinTableIndexByLabelName, dbName, "o")
		if err != nil {
			return nil, err
		}
		query = "WITH " + strings.Join(withParts, ",\n") + "\n"
		params = withParams
		joinPartsToUse = joinParts
	}
	query += "SELECT "
	if queryUsesLabels {
		query += "DISTINCT "
	}
	query += fmt.Sprintf(`o.object, o.objectnonce, o.dekid FROM "%s" o`, dbName)
	query += "\n  "
	query += fmt.Sprintf(`JOIN "%s_fields" f ON o.key = f.key`, dbName)
	if len(joinPartsToUse) > 0 {
		query += "\n  "
		query += strings.Join(joinPartsToUse, "\n  ")
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
						query += "\n  "
						query += fmt.Sprintf(`LEFT OUTER JOIN "%s_labels" lt%d ON o.key = lt%d.key`, dbName, jtIndex, jtIndex)
					}
				}
			}
		}
	}

	if len(lo.ProjectsOrNamespaces.Filters) > 0 {
		jtIndex := len(joinTableIndexByLabelName) + 1
		if _, exists := joinTableIndexByLabelName[projectIDFieldLabel]; !exists {
			joinTableIndexByLabelName[projectIDFieldLabel] = jtIndex
		}
		query += "\n  "
		query += fmt.Sprintf(`LEFT OUTER JOIN "%s_fields" nsf ON f."metadata.namespace" = nsf."metadata.name"`, namespacesDbName)
		query += "\n  "
		query += fmt.Sprintf(`LEFT OUTER JOIN "%s_labels" lt%d ON nsf.key = lt%d.key`, namespacesDbName, jtIndex, jtIndex)
	}

	// 2- Filtering: WHERE clauses (from lo.Filters)
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

	// WHERE clauses (from lo.ProjectsOrNamespaces)
	if len(lo.ProjectsOrNamespaces.Filters) > 0 {
		projOrNsClause, projOrNsParams, err := l.buildClauseFromProjectsOrNamespaces(lo.ProjectsOrNamespaces, dbName, joinTableIndexByLabelName)
		if err != nil {
			return queryInfo, err
		}
		whereClauses = append(whereClauses, projOrNsClause)
		params = append(params, projOrNsParams...)
	}

	// WHERE clauses (from namespace)
	if namespace != "" && namespace != "*" {
		whereClauses = append(whereClauses, fmt.Sprintf(`f."metadata.namespace" = ?`))
		params = append(params, namespace)
	}

	// WHERE clauses (from partitions and their corresponding parameters)
	partitionClauses, partitionClausesParams := generatePartitionClauses(namespace, partitions, "f")
	if n := len(partitionClauses); n > 0 {
		if n == 1 {
			whereClauses = append(whereClauses, partitionClauses[0])
		} else {
			whereClauses = append(whereClauses, "(\n      ("+strings.Join(partitionClauses, ") OR\n      (")+")\n)")
		}
		params = append(params, partitionClausesParams...)
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
				clause, err := buildSortLabelsClause(fields[2], joinTableIndexByLabelName, sortDirective.Order == sqltypes.ASC, sortDirective.SortAsIP)
				if err != nil {
					return nil, err
				}
				orderByClauses = append(orderByClauses, clause)
			} else {
				fieldEntry, err := l.getValidFieldEntry("f", fields)
				if err != nil {
					return queryInfo, err
				}
				if sortDirective.SortAsIP {
					fieldEntry = fmt.Sprintf("inet_aton(%s)", fieldEntry)
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
			// ID == metadata.namespace + "/" + metaqata.name
			query += "\n  ORDER BY f.\"id\" ASC "
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
			var index int
			index, err = internLabel(filter.Field[2], joinTableIndexByLabelName, -1)
			if err != nil {
				return "", nil, err
			}
			newClause, newParams, err = l.getLabelFilter(index, filter, dbName)
		} else {
			newClause, newParams, err = l.getFieldFilter(filter, "f")
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
	return fmt.Sprintf("%s %s NULLS %s", fieldEntry, dir, nullsPosition), nil
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

func getWithParts(unboundSortLabels []string, joinTableIndexByLabelName map[string]int, dbName string, mainFuncPrefix string) ([]string, []any, []string, []string, error) {
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
		joinParts[i] = fmt.Sprintf("LEFT OUTER JOIN lt%d ON %s.key = lt%d.key", idx, mainFuncPrefix, idx)
	}

	return parts, params, withNames, joinParts, nil
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

// Possible ops from the k8s parser:
// KEY = and == (same) VALUE
// KEY != VALUE
// KEY exists []  # ,KEY, => this filter
// KEY ! []  # ,!KEY, => assert KEY doesn't exist
// KEY in VALUES
// KEY notin VALUES

func (l *ListOptionIndexer) getFieldFilter(filter sqltypes.Filter, prefix string) (string, []any, error) {
	opString := ""
	escapeString := ""
	fieldEntry, err := l.getValidFieldEntry(prefix, filter.Field)
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

func (l *ListOptionIndexer) getProjectsOrNamespacesFieldFilter(filter sqltypes.Filter) (string, []any, error) {
	opString := ""
	fieldEntry, err := l.getValidFieldEntry("nsf", filter.Field)
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
		clause2 := fmt.Sprintf(`(o.key NOT IN (SELECT o1.key FROM "%s" o1
		JOIN "%s_fields" f1 ON o1.key = f1.key
		LEFT OUTER JOIN "_v1_Namespace_fields" nsf1 ON f1."metadata.namespace" = nsf1."metadata.name"
		LEFT OUTER JOIN "_v1_Namespace_labels" lt%di1 ON nsf1.key = lt%di1.key
		WHERE lt%di1.label = ?))`, dbName, dbName, index, index, index)
		matches = append(matches, labelName)
		clause := fmt.Sprintf("%s OR %s", clause1, clause2)
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
