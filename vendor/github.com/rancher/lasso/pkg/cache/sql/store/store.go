/*
Package store contains the sql backed store. It persists objects to a sqlite database.
*/
package store

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"

	"github.com/rancher/lasso/pkg/cache/sql/db"
	"github.com/rancher/lasso/pkg/cache/sql/db/transaction"
	"k8s.io/client-go/tools/cache"
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
	DBClient

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

	afterUpsert []func(key string, obj any, tx db.TXClient) error
	afterDelete []func(key string, tx db.TXClient) error
}

// Test that Store implements cache.Indexer
var _ cache.Store = (*Store)(nil)

type DBClient interface {
	Begin() (db.TXClient, error)
	Prepare(stmt string) *sql.Stmt
	QueryForRows(ctx context.Context, stmt transaction.Stmt, params ...any) (*sql.Rows, error)
	ReadObjects(rows db.Rows, typ reflect.Type, shouldDecrypt bool) ([]any, error)
	ReadStrings(rows db.Rows) ([]string, error)
	ReadInt(rows db.Rows) (int, error)
	Upsert(tx db.TXClient, stmt *sql.Stmt, key string, obj any, shouldEncrypt bool) error
	CloseStmt(closable db.Closable) error
}

// NewStore creates a SQLite-backed cache.Store for objects of the given example type
func NewStore(example any, keyFunc cache.KeyFunc, c DBClient, shouldEncrypt bool, name string) (*Store, error) {
	s := &Store{
		name:          db.Sanitize(name),
		typ:           reflect.TypeOf(example),
		DBClient:      c,
		keyFunc:       keyFunc,
		shouldEncrypt: shouldEncrypt,
		afterUpsert:   []func(key string, obj any, tx db.TXClient) error{},
		afterDelete:   []func(key string, tx db.TXClient) error{},
	}

	// once multiple informerfactories are needed, this can accept the case where table already exists error is received
	txC, err := s.Begin()
	if err != nil {
		return nil, err
	}
	createTableQuery := fmt.Sprintf(createTableFmt, s.name)
	err = txC.Exec(createTableQuery)
	if err != nil {
		return nil, fmt.Errorf("while executing query: %s got error: %w", createTableQuery, err)
	}

	err = txC.Commit()
	if err != nil {
		return nil, err
	}

	s.upsertQuery = fmt.Sprintf(upsertStmtFmt, s.name)
	s.deleteQuery = fmt.Sprintf(deleteStmtFmt, s.name)
	s.getQuery = fmt.Sprintf(getStmtFmt, s.name)
	s.listQuery = fmt.Sprintf(listStmtFmt, s.name)
	s.listKeysQuery = fmt.Sprintf(listKeysStmtFmt, s.name)

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
	tx, err := s.Begin()
	if err != nil {
		return err
	}

	err = s.Upsert(tx, s.upsertStmt, key, obj, s.shouldEncrypt)
	if err != nil {
		return fmt.Errorf("while executing query: %s got error: %w", s.upsertQuery, err)
	}

	err = s.runAfterUpsert(key, obj, tx)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// deleteByKey deletes the object associated with key, if it exists in this Store
func (s *Store) deleteByKey(key string) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}

	err = tx.StmtExec(tx.Stmt(s.deleteStmt), key)
	if err != nil {
		return fmt.Errorf("while executing query: %s got error: %w", s.deleteQuery, err)
	}

	err = s.runAfterDelete(key, tx)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetByKey returns the object associated with the given object's key
func (s *Store) GetByKey(key string) (item any, exists bool, err error) {
	rows, err := s.QueryForRows(context.TODO(), s.getStmt, key)
	if err != nil {
		return nil, false, fmt.Errorf("while executing query: %s got error: %w", s.getQuery, err)
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
	return err
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
	return s.deleteByKey(key)
}

// List returns a list of all the currently known objects
// Note: I/O errors will panic this function, as the interface signature does not allow returning errors
func (s *Store) List() []any {
	rows, err := s.QueryForRows(context.TODO(), s.listStmt)
	if err != nil {
		panic(fmt.Errorf("while executing query: %s got error: %w", s.listQuery, err))
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
	rows, err := s.QueryForRows(context.TODO(), s.listKeysStmt)
	if err != nil {
		fmt.Printf("Unexpected error in store.ListKeys: %v\n", fmt.Errorf("while executing query: %s got error: %w", s.listKeysQuery, err))
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
	txC, err := s.Begin()
	if err != nil {
		return err
	}

	txCListKeys := txC.Stmt(s.listKeysStmt)

	rows, err := s.QueryForRows(context.TODO(), txCListKeys)
	if err != nil {
		return err
	}
	keys, err := s.ReadStrings(rows)
	if err != nil {
		return err
	}

	for _, key := range keys {
		err = txC.StmtExec(txC.Stmt(s.deleteStmt), key)
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

	return txC.Commit()
}

// Resync is a no-op and is deprecated
func (s *Store) Resync() error {
	return nil
}

/* Utilities */

// RegisterAfterUpsert registers a func to be called after each upsert
func (s *Store) RegisterAfterUpsert(f func(key string, obj any, txC db.TXClient) error) {
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
func (s *Store) runAfterUpsert(key string, obj any, txC db.TXClient) error {
	for _, f := range s.afterUpsert {
		err := f(key, obj, txC)
		if err != nil {
			return err
		}
	}
	return nil
}

// RegisterAfterDelete registers a func to be called after each deletion
func (s *Store) RegisterAfterDelete(f func(key string, txC db.TXClient) error) {
	s.afterDelete = append(s.afterDelete, f)
}

// keep
// runAfterDelete executes functions registered to run after upsert
func (s *Store) runAfterDelete(key string, txC db.TXClient) error {
	for _, f := range s.afterDelete {
		err := f(key, txC)
		if err != nil {
			return err
		}
	}
	return nil
}
