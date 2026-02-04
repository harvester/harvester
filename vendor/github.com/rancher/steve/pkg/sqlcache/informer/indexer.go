package informer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/rancher/steve/pkg/sqlcache/db"
	"k8s.io/client-go/tools/cache"
)

const (
	selectQueryFmt = `
			SELECT object, objectnonce, dekid FROM "%[1]s"
				WHERE key IN (
					SELECT key FROM "%[1]s_indices"
						WHERE name = ? AND value IN (?%s)
				)
		`
	createTableFmt = `CREATE TABLE IF NOT EXISTS "%[1]s_indices" (
			name TEXT NOT NULL,
			value TEXT NOT NULL,
			key TEXT NOT NULL REFERENCES "%[1]s"(key) ON DELETE CASCADE,
			PRIMARY KEY (name, value, key)
        )`
	createIndexFmt = `CREATE INDEX IF NOT EXISTS "%[1]s_indices_key_fk_index" ON "%[1]s_indices"(key)`

	deleteIndicesFmt = `DELETE FROM "%s_indices" WHERE key = ?`
	dropIndicesFmt   = `DROP TABLE IF EXISTS "%s_indices"`
	// addIndexFmt expects to use a big insert, so the columns must be kept in sync with addIndexValuesPlaceholderFmt
	addIndexFmt                  = `INSERT INTO "%s_indices" (name, value, key) VALUES %s`
	addIndexValuesPlaceholderFmt = `(?, ?, ?)`
	listByIndexFmt               = `SELECT object, objectnonce, dekid FROM "%[1]s"
			WHERE key IN (
			    SELECT key FROM "%[1]s_indices"
			    	WHERE name = ? AND value = ?
			)`
	listKeyByIndexFmt  = `SELECT DISTINCT key FROM "%s_indices" WHERE name = ? AND value = ?`
	listIndexValuesFmt = `SELECT DISTINCT value FROM "%s_indices" WHERE name = ?`
)

// Indexer is a SQLite-backed cache.Indexer which builds upon Store adding an index table
type Indexer struct {
	ctx context.Context

	Store
	indexers     cache.Indexers
	indexersLock sync.RWMutex

	deleteIndicesStmt   db.Stmt
	dropIndicesStmt     db.Stmt
	listByIndexStmt     db.Stmt
	listKeysByIndexStmt db.Stmt
	listIndexValuesStmt db.Stmt
}

var _ cache.Indexer = (*Indexer)(nil)

type Store interface {
	db.Client
	cache.Store

	GetByKey(key string) (item any, exists bool, err error)
	GetName() string
	RegisterAfterAdd(f func(key string, obj any, tx db.TxClient) error)
	RegisterAfterUpdate(f func(key string, obj any, tx db.TxClient) error)
	RegisterAfterDelete(f func(key string, obj any, tx db.TxClient) error)
	RegisterAfterDeleteAll(f func(tx db.TxClient) error)
	RegisterBeforeDropAll(f func(tx db.TxClient) error)
	GetShouldEncrypt() bool
	GetType() reflect.Type
	DropAll(ctx context.Context) error
}

// NewIndexer returns a cache.Indexer backed by SQLite for objects of the given example type
func NewIndexer(ctx context.Context, indexers cache.Indexers, s Store) (*Indexer, error) {
	dbName := db.Sanitize(s.GetName())

	err := s.WithTransaction(ctx, true, func(tx db.TxClient) error {
		dropTableQuery := fmt.Sprintf(dropIndicesFmt, dbName)
		if _, err := tx.Exec(dropTableQuery); err != nil {
			return err
		}
		createTableQuery := fmt.Sprintf(createTableFmt, dbName)
		if _, err := tx.Exec(createTableQuery); err != nil {
			return err
		}
		createIndexQuery := fmt.Sprintf(createIndexFmt, dbName)
		_, err := tx.Exec(createIndexQuery)
		return err
	})
	if err != nil {
		return nil, err
	}

	i := &Indexer{
		ctx:      ctx,
		Store:    s,
		indexers: indexers,
	}
	i.RegisterAfterAdd(i.AfterUpsert)
	i.RegisterAfterUpdate(i.AfterUpsert)
	i.RegisterBeforeDropAll(i.dropIndices)

	i.deleteIndicesStmt = s.Prepare(fmt.Sprintf(deleteIndicesFmt, dbName))
	i.dropIndicesStmt = s.Prepare(fmt.Sprintf(dropIndicesFmt, dbName))
	i.listByIndexStmt = s.Prepare(fmt.Sprintf(listByIndexFmt, dbName))
	i.listKeysByIndexStmt = s.Prepare(fmt.Sprintf(listKeyByIndexFmt, dbName))
	i.listIndexValuesStmt = s.Prepare(fmt.Sprintf(listIndexValuesFmt, dbName))

	return i, nil
}

/* Core methods */

// AfterUpsert updates indices of an object
func (i *Indexer) AfterUpsert(key string, obj any, tx db.TxClient) error {
	// delete all
	if _, err := tx.Stmt(i.deleteIndicesStmt).Exec(key); err != nil {
		return err
	}

	// re-insert all values, using a single big insert
	var rowsToInsert int
	var valuesToInsert []any
	i.indexersLock.RLock()
	for _, indexName := range slices.Sorted(maps.Keys(i.indexers)) {
		values, err := i.indexers[indexName](obj)
		if err != nil {
			i.indexersLock.RUnlock()
			return err
		}

		for _, value := range values {
			valuesToInsert = append(valuesToInsert, indexName, value, key)
			rowsToInsert++
		}
	}
	i.indexersLock.RUnlock()

	if rowsToInsert == 0 {
		return nil
	}

	multiInsertQuery := fmt.Sprintf(addIndexFmt,
		db.Sanitize(i.Store.GetName()),
		strings.Join(slices.Repeat([]string{addIndexValuesPlaceholderFmt}, rowsToInsert), ", "))
	if _, err := tx.Stmt(i.Prepare(multiInsertQuery)).Exec(valuesToInsert...); err != nil {
		return err
	}

	return nil
}

/* Satisfy cache.Indexer */

// Index returns a list of items that match the given object on the index function
func (i *Indexer) Index(indexName string, obj any) (result []any, err error) {
	i.indexersLock.RLock()
	defer i.indexersLock.RUnlock()
	indexFunc := i.indexers[indexName]
	if indexFunc == nil {
		return nil, fmt.Errorf("index with name %s does not exist", indexName)
	}

	values, err := indexFunc(obj)
	if err != nil {
		return nil, err
	}

	if len(values) == 0 {
		return nil, nil
	}

	// typical case
	if len(values) == 1 {
		return i.ByIndex(indexName, values[0])
	}

	// atypical case - more than one value to lookup
	// HACK: sql.Statement.Query does not allow to pass slices in as of go 1.19 - create an ad-hoc statement
	query := fmt.Sprintf(selectQueryFmt, db.Sanitize(i.GetName()), strings.Repeat(", ?", len(values)-1))
	stmt := i.Prepare(query)

	defer func() {
		if cerr := stmt.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()
	// HACK: Query will accept []any but not []string
	params := []any{indexName}
	for _, value := range values {
		params = append(params, value)
	}

	rows, err := i.QueryForRows(i.ctx, stmt, params...)
	if err != nil {
		return nil, err
	}
	return i.ReadObjects(rows, i.GetType())
}

func (i *Indexer) dropIndices(tx db.TxClient) error {
	_, err := tx.Stmt(i.dropIndicesStmt).Exec()
	return err
}

// ByIndex returns the stored objects whose set of indexed values
// for the named index includes the given indexed value
func (i *Indexer) ByIndex(indexName, indexedValue string) ([]any, error) {
	rows, err := i.QueryForRows(i.ctx, i.listByIndexStmt, indexName, indexedValue)
	if err != nil {
		return nil, err
	}
	return i.ReadObjects(rows, i.GetType())
}

// IndexKeys returns a list of the Store keys of the objects whose indexed values in the given index include the given indexed value
func (i *Indexer) IndexKeys(indexName, indexedValue string) ([]string, error) {
	i.indexersLock.RLock()
	defer i.indexersLock.RUnlock()
	indexFunc := i.indexers[indexName]
	if indexFunc == nil {
		return nil, fmt.Errorf("Index with name %s does not exist", indexName)
	}

	rows, err := i.QueryForRows(i.ctx, i.listKeysByIndexStmt, indexName, indexedValue)
	if err != nil {
		return nil, err
	}
	return i.ReadStrings(rows)
}

// ListIndexFuncValues wraps safeListIndexFuncValues and panics in case of I/O errors
func (i *Indexer) ListIndexFuncValues(name string) []string {
	result, err := i.safeListIndexFuncValues(name)
	if err != nil {
		panic(fmt.Errorf("unexpected error in safeListIndexFuncValues: %w", err))
	}
	return result
}

// safeListIndexFuncValues returns all the indexed values of the given index
func (i *Indexer) safeListIndexFuncValues(indexName string) ([]string, error) {
	rows, err := i.QueryForRows(i.ctx, i.listIndexValuesStmt, indexName)
	if err != nil {
		return nil, err
	}
	return i.ReadStrings(rows)
}

// GetIndexers returns the indexers
func (i *Indexer) GetIndexers() cache.Indexers {
	i.indexersLock.RLock()
	defer i.indexersLock.RUnlock()
	return i.indexers
}

// AddIndexers adds more indexers to this Store.  If you call this after you already have data
// in the Store, the results are undefined.
func (i *Indexer) AddIndexers(newIndexers cache.Indexers) error {
	i.indexersLock.Lock()
	defer i.indexersLock.Unlock()
	if i.indexers == nil {
		i.indexers = make(map[string]cache.IndexFunc)
	}
	for k, v := range newIndexers {
		i.indexers[k] = v
	}
	return nil
}
