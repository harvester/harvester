/*
Package store contains the sql backed store. It persists objects to a sqlite database.
*/
package store

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"

	"github.com/rancher/lasso/pkg/log"
	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/db/transaction"
	"k8s.io/client-go/tools/cache"

	// needed for drivers
	_ "modernc.org/sqlite"
)

const (
	upsertStmtFmt   = `REPLACE INTO "%s"(key, object, objectnonce, dekid) VALUES (?, ?, ?, ?)`
	deleteStmtFmt   = `DELETE FROM "%s" WHERE key = ?`
	getStmtFmt      = `SELECT object, objectnonce, dekid FROM "%s" WHERE key = ?`
	listStmtFmt     = `SELECT object, objectnonce, dekid FROM "%s"`
	listKeysStmtFmt = `SELECT key FROM "%s"`
	createTableFmt  = `CREATE TABLE IF NOT EXISTS "%s" (
		key TEXT UNIQUE NOT NULL PRIMARY KEY,
		object BLOB,
		objectnonce BLOB,
		dekid INTEGER
	)`
)

// Store is a SQLite-backed cache.Store
type Store struct {
	db.Client

	ctx           context.Context
	name          string
	typ           reflect.Type
	keyFunc       cache.KeyFunc
	shouldEncrypt bool

	upsertQuery   string
	deleteQuery   string
	getQuery      string
	listQuery     string
	listKeysQuery string

	upsertStmt   *sql.Stmt
	deleteStmt   *sql.Stmt
	getStmt      *sql.Stmt
	listStmt     *sql.Stmt
	listKeysStmt *sql.Stmt

	afterUpsert []func(key string, obj any, tx transaction.Client) error
	afterDelete []func(key string, tx transaction.Client) error
}

// Test that Store implements cache.Indexer
var _ cache.Store = (*Store)(nil)

// NewStore creates a SQLite-backed cache.Store for objects of the given example type
func NewStore(ctx context.Context, example any, keyFunc cache.KeyFunc, c db.Client, shouldEncrypt bool, name string) (*Store, error) {
	s := &Store{
		ctx:           ctx,
		name:          name,
		typ:           reflect.TypeOf(example),
		Client:        c,
		keyFunc:       keyFunc,
		shouldEncrypt: shouldEncrypt,
		afterUpsert:   []func(key string, obj any, tx transaction.Client) error{},
		afterDelete:   []func(key string, tx transaction.Client) error{},
	}

	dbName := db.Sanitize(s.name)

	// once multiple informer-factories are needed, this can accept the case where table already exists error is received
	err := s.WithTransaction(ctx, true, func(tx transaction.Client) error {
		createTableQuery := fmt.Sprintf(createTableFmt, dbName)
		_, err := tx.Exec(createTableQuery)
		if err != nil {
			return &db.QueryError{QueryString: createTableQuery, Err: err}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.upsertQuery = fmt.Sprintf(upsertStmtFmt, dbName)
	s.deleteQuery = fmt.Sprintf(deleteStmtFmt, dbName)
	s.getQuery = fmt.Sprintf(getStmtFmt, dbName)
	s.listQuery = fmt.Sprintf(listStmtFmt, dbName)
	s.listKeysQuery = fmt.Sprintf(listKeysStmtFmt, dbName)

	s.upsertStmt = s.Prepare(s.upsertQuery)
	s.deleteStmt = s.Prepare(s.deleteQuery)
	s.getStmt = s.Prepare(s.getQuery)
	s.listStmt = s.Prepare(s.listQuery)
	s.listKeysStmt = s.Prepare(s.listKeysQuery)

	return s, nil
}

/* Core methods */
// upsert saves an obj with its key, or updates key with obj if it exists in this Store
func (s *Store) upsert(key string, obj any) error {
	return s.WithTransaction(s.ctx, true, func(tx transaction.Client) error {
		err := s.Upsert(tx, s.upsertStmt, key, obj, s.shouldEncrypt)
		if err != nil {
			return &db.QueryError{QueryString: s.upsertQuery, Err: err}
		}

		err = s.runAfterUpsert(key, obj, tx)
		if err != nil {
			return err
		}

		return nil
	})
}

// deleteByKey deletes the object associated with key, if it exists in this Store
func (s *Store) deleteByKey(key string) error {
	return s.WithTransaction(s.ctx, true, func(tx transaction.Client) error {
		_, err := tx.Stmt(s.deleteStmt).Exec(key)
		if err != nil {
			return &db.QueryError{QueryString: s.deleteQuery, Err: err}
		}

		err = s.runAfterDelete(key, tx)
		if err != nil {
			return err
		}

		return nil
	})
}

// GetByKey returns the object associated with the given object's key
func (s *Store) GetByKey(key string) (item any, exists bool, err error) {
	rows, err := s.QueryForRows(s.ctx, s.getStmt, key)
	if err != nil {
		return nil, false, &db.QueryError{QueryString: s.getQuery, Err: err}
	}
	result, err := s.ReadObjects(rows, s.typ, s.shouldEncrypt)
	if err != nil {
		return nil, false, err
	}

	if len(result) == 0 {
		return nil, false, nil
	}

	return result[0], true, nil
}

/* Satisfy cache.Store */

// Add saves an obj, or updates it if it exists in this Store
func (s *Store) Add(obj any) error {
	key, err := s.keyFunc(obj)
	if err != nil {
		return err
	}

	err = s.upsert(key, obj)
	if err != nil {
		log.Errorf("Error in Store.Add for type %v: %v", s.name, err)
		return err
	}
	return nil
}

// Update saves an obj, or updates it if it exists in this Store
func (s *Store) Update(obj any) error {
	return s.Add(obj)
}

// Delete deletes the given object, if it exists in this Store
func (s *Store) Delete(obj any) error {
	key, err := s.keyFunc(obj)
	if err != nil {
		return err
	}
	err = s.deleteByKey(key)
	if err != nil {
		log.Errorf("Error in Store.Delete for type %v: %v", s.name, err)
		return err
	}
	return nil
}

// List returns a list of all the currently known objects
// Note: I/O errors will panic this function, as the interface signature does not allow returning errors
func (s *Store) List() []any {
	rows, err := s.QueryForRows(s.ctx, s.listStmt)
	if err != nil {
		panic(&db.QueryError{QueryString: s.listQuery, Err: err})
	}
	result, err := s.ReadObjects(rows, s.typ, s.shouldEncrypt)
	if err != nil {
		panic(fmt.Errorf("error in Store.List: %w", err))
	}
	return result
}

// ListKeys returns a list of all the keys currently in this Store
// Note: Atm it doesn't appear returning nil in the case of an error has any detrimental effects. An error is not
// uncommon enough nor does it appear to necessitate a panic.
func (s *Store) ListKeys() []string {
	rows, err := s.QueryForRows(s.ctx, s.listKeysStmt)
	if err != nil {
		fmt.Printf("Unexpected error in store.ListKeys: while executing query: %s got error: %v", s.listKeysQuery, err)
		return []string{}
	}
	result, err := s.ReadStrings(rows)
	if err != nil {
		fmt.Printf("Unexpected error in store.ListKeys: %v\n", err)
		return []string{}
	}
	return result
}

// Get returns the object with the same key as obj
func (s *Store) Get(obj any) (item any, exists bool, err error) {
	key, err := s.keyFunc(obj)
	if err != nil {
		return nil, false, err
	}

	return s.GetByKey(key)
}

// Replace will delete the contents of the Store, using instead the given list
func (s *Store) Replace(objects []any, _ string) error {
	objectMap := map[string]any{}

	for _, object := range objects {
		key, err := s.keyFunc(object)
		if err != nil {
			return err
		}
		objectMap[key] = object
	}
	return s.replaceByKey(objectMap)
}

// replaceByKey will delete the contents of the Store, using instead the given key to obj map
func (s *Store) replaceByKey(objects map[string]any) error {
	return s.WithTransaction(s.ctx, true, func(txC transaction.Client) error {
		txCListKeys := txC.Stmt(s.listKeysStmt)

		rows, err := s.QueryForRows(s.ctx, txCListKeys)
		if err != nil {
			return err
		}
		keys, err := s.ReadStrings(rows)
		if err != nil {
			return err
		}

		for _, key := range keys {
			_, err = txC.Stmt(s.deleteStmt).Exec(key)
			if err != nil {
				return err
			}
			err = s.runAfterDelete(key, txC)
			if err != nil {
				return err
			}
		}

		for key, obj := range objects {
			err = s.Upsert(txC, s.upsertStmt, key, obj, s.shouldEncrypt)
			if err != nil {
				return err
			}
			err = s.runAfterUpsert(key, obj, txC)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// Resync is a no-op and is deprecated
func (s *Store) Resync() error {
	return nil
}

/* Utilities */

// RegisterAfterUpsert registers a func to be called after each upsert
func (s *Store) RegisterAfterUpsert(f func(key string, obj any, txC transaction.Client) error) {
	s.afterUpsert = append(s.afterUpsert, f)
}

func (s *Store) GetName() string {
	return s.name
}

func (s *Store) GetShouldEncrypt() bool {
	return s.shouldEncrypt
}

func (s *Store) GetType() reflect.Type {
	return s.typ
}

// keep
// runAfterUpsert executes functions registered to run after upsert
func (s *Store) runAfterUpsert(key string, obj any, txC transaction.Client) error {
	for _, f := range s.afterUpsert {
		err := f(key, obj, txC)
		if err != nil {
			return err
		}
	}
	return nil
}

// RegisterAfterDelete registers a func to be called after each deletion
func (s *Store) RegisterAfterDelete(f func(key string, txC transaction.Client) error) {
	s.afterDelete = append(s.afterDelete, f)
}

// keep
// runAfterDelete executes functions registered to run after upsert
func (s *Store) runAfterDelete(key string, txC transaction.Client) error {
	for _, f := range s.afterDelete {
		err := f(key, txC)
		if err != nil {
			return err
		}
	}
	return nil
}
