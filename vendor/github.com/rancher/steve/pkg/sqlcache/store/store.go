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
	upsertStmtFmt    = `REPLACE INTO "%s"(key, object, objectnonce, dekid) VALUES (?, ?, ?, ?)`
	deleteStmtFmt    = `DELETE FROM "%s" WHERE key = ?`
	deleteAllStmtFmt = `DELETE FROM "%s"`
	getStmtFmt       = `SELECT object, objectnonce, dekid FROM "%s" WHERE key = ?`
	listStmtFmt      = `SELECT object, objectnonce, dekid FROM "%s"`
	listKeysStmtFmt  = `SELECT key FROM "%s"`
	createTableFmt   = `CREATE TABLE IF NOT EXISTS "%s" (
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

	upsertQuery    string
	deleteQuery    string
	deleteAllQuery string
	getQuery       string
	listQuery      string
	listKeysQuery  string

	upsertStmt    *sql.Stmt
	deleteStmt    *sql.Stmt
	deleteAllStmt *sql.Stmt
	getStmt       *sql.Stmt
	listStmt      *sql.Stmt
	listKeysStmt  *sql.Stmt

	afterAdd       []func(key string, obj any, tx transaction.Client) error
	afterUpdate    []func(key string, obj any, tx transaction.Client) error
	afterDelete    []func(key string, obj any, tx transaction.Client) error
	afterDeleteAll []func(tx transaction.Client) error
}

// Test that Store implements cache.Indexer
var _ cache.Store = (*Store)(nil)

// NewStore creates a SQLite-backed cache.Store for objects of the given example type
func NewStore(ctx context.Context, example any, keyFunc cache.KeyFunc, c db.Client, shouldEncrypt bool, name string) (*Store, error) {
	s := &Store{
		ctx:            ctx,
		name:           name,
		typ:            reflect.TypeOf(example),
		Client:         c,
		keyFunc:        keyFunc,
		shouldEncrypt:  shouldEncrypt,
		afterAdd:       []func(key string, obj any, tx transaction.Client) error{},
		afterUpdate:    []func(key string, obj any, tx transaction.Client) error{},
		afterDelete:    []func(key string, obj any, tx transaction.Client) error{},
		afterDeleteAll: []func(tx transaction.Client) error{},
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
	s.deleteAllQuery = fmt.Sprintf(deleteAllStmtFmt, dbName)
	s.getQuery = fmt.Sprintf(getStmtFmt, dbName)
	s.listQuery = fmt.Sprintf(listStmtFmt, dbName)
	s.listKeysQuery = fmt.Sprintf(listKeysStmtFmt, dbName)

	s.upsertStmt = s.Prepare(s.upsertQuery)
	s.deleteStmt = s.Prepare(s.deleteQuery)
	s.deleteAllStmt = s.Prepare(s.deleteAllQuery)
	s.getStmt = s.Prepare(s.getQuery)
	s.listStmt = s.Prepare(s.listQuery)
	s.listKeysStmt = s.Prepare(s.listKeysQuery)

	return s, nil
}

/* Core methods */

// deleteByKey deletes the object associated with key, if it exists in this Store
func (s *Store) deleteByKey(key string, obj any) error {
	return s.WithTransaction(s.ctx, true, func(tx transaction.Client) error {
		_, err := tx.Stmt(s.deleteStmt).Exec(key)
		if err != nil {
			return &db.QueryError{QueryString: s.deleteQuery, Err: err}
		}

		err = s.runAfterDelete(key, obj, tx)
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

	err = s.WithTransaction(s.ctx, true, func(tx transaction.Client) error {
		err := s.Upsert(tx, s.upsertStmt, key, obj, s.shouldEncrypt)
		if err != nil {
			return &db.QueryError{QueryString: s.upsertQuery, Err: err}
		}

		err = s.runAfterAdd(key, obj, tx)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		log.Errorf("Error in Store.Add for type %v: %v", s.name, err)
		return err
	}
	return nil
}

// Update saves an obj, or updates it if it exists in this Store
func (s *Store) Update(obj any) error {
	key, err := s.keyFunc(obj)
	if err != nil {
		return err
	}

	err = s.WithTransaction(s.ctx, true, func(tx transaction.Client) error {
		err := s.Upsert(tx, s.upsertStmt, key, obj, s.shouldEncrypt)
		if err != nil {
			return &db.QueryError{QueryString: s.upsertQuery, Err: err}
		}

		err = s.runAfterUpdate(key, obj, tx)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		log.Errorf("Error in Store.Update for type %v: %v", s.name, err)
		return err
	}
	return nil
}

// Delete deletes the given object, if it exists in this Store
func (s *Store) Delete(obj any) error {
	key, err := s.keyFunc(obj)
	if err != nil {
		return err
	}
	err = s.deleteByKey(key, obj)
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
	err := s.replaceByKey(objectMap)
	if err != nil {
		log.Errorf("Error in Store.Replace for type %v: %v", s.name, err)
		return err
	}
	return nil
}

// replaceByKey will delete the contents of the Store, using instead the given key to obj map
func (s *Store) replaceByKey(objects map[string]any) error {
	return s.WithTransaction(s.ctx, true, func(txC transaction.Client) error {
		_, err := txC.Stmt(s.deleteAllStmt).Exec()
		if err != nil {
			return &db.QueryError{QueryString: s.deleteAllQuery, Err: err}
		}

		err = s.runAfterDeleteAll(txC)
		if err != nil {
			return err
		}

		for key, obj := range objects {
			err = s.Upsert(txC, s.upsertStmt, key, obj, s.shouldEncrypt)
			if err != nil {
				return err
			}
			err = s.runAfterAdd(key, obj, txC)
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

func (s *Store) GetName() string {
	return s.name
}

func (s *Store) GetShouldEncrypt() bool {
	return s.shouldEncrypt
}

func (s *Store) GetType() reflect.Type {
	return s.typ
}

// RegisterAfterAdd registers a func to be called after each add event
func (s *Store) RegisterAfterAdd(f func(key string, obj any, txC transaction.Client) error) {
	s.afterAdd = append(s.afterAdd, f)
}

// RegisterAfterUpdate registers a func to be called after each update event
func (s *Store) RegisterAfterUpdate(f func(key string, obj any, txC transaction.Client) error) {
	s.afterUpdate = append(s.afterUpdate, f)
}

// RegisterAfterDelete registers a func to be called after each deletion
func (s *Store) RegisterAfterDelete(f func(key string, obj any, txC transaction.Client) error) {
	s.afterDelete = append(s.afterDelete, f)
}

// RegisterAfterDelete registers a func to be called after each deletion
func (s *Store) RegisterAfterDeleteAll(f func(txC transaction.Client) error) {
	s.afterDeleteAll = append(s.afterDeleteAll, f)
}

// runAfterAdd executes functions registered to run after add event
func (s *Store) runAfterAdd(key string, obj any, txC transaction.Client) error {
	for _, f := range s.afterAdd {
		err := f(key, obj, txC)
		if err != nil {
			return err
		}
	}
	return nil
}

// runAfterUpdate executes functions registered to run after update event
func (s *Store) runAfterUpdate(key string, obj any, txC transaction.Client) error {
	for _, f := range s.afterUpdate {
		err := f(key, obj, txC)
		if err != nil {
			return err
		}
	}
	return nil
}

// runAfterDelete executes functions registered to run after delete event
func (s *Store) runAfterDelete(key string, obj any, txC transaction.Client) error {
	for _, f := range s.afterDelete {
		err := f(key, obj, txC)
		if err != nil {
			return err
		}
	}
	return nil
}

// runAfterDeleteAll executes functions registered to run after delete events when
// the database is being replaced.
func (s *Store) runAfterDeleteAll(txC transaction.Client) error {
	for _, f := range s.afterDeleteAll {
		err := f(txC)
		if err != nil {
			return err
		}
	}
	return nil
}
