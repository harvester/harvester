/*
Package store contains the sql backed store. It persists objects to a sqlite database.
*/
package store

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/rancher/lasso/pkg/log"
	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/db/transaction"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	// needed for drivers
	_ "modernc.org/sqlite"
)

const (
	upsertStmtFmt    = `REPLACE INTO "%s"(key, object, objectnonce, dekid) VALUES (?, ?, ?, ?)`
	deleteStmtFmt    = `DELETE FROM "%s" WHERE key = ?`
	deleteAllStmtFmt = `DELETE FROM "%s"`
	dropBaseStmtFmt  = `DROP TABLE IF EXISTS "%s"`
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

	ctx                context.Context
	gvk                schema.GroupVersionKind
	name               string
	externalUpdateInfo *sqltypes.ExternalGVKUpdates
	selfUpdateInfo     *sqltypes.ExternalGVKUpdates
	typ                reflect.Type
	keyFunc            cache.KeyFunc
	shouldEncrypt      bool

	upsertQuery    string
	deleteQuery    string
	deleteAllQuery string
	getQuery       string
	listQuery      string
	listKeysQuery  string
	dropBaseQuery  string

	upsertStmt    *sql.Stmt
	deleteStmt    *sql.Stmt
	deleteAllStmt *sql.Stmt
	dropBaseStmt  *sql.Stmt
	getStmt       *sql.Stmt
	listStmt      *sql.Stmt
	listKeysStmt  *sql.Stmt

	afterAdd       []func(key string, obj any, tx transaction.Client) error
	afterUpdate    []func(key string, obj any, tx transaction.Client) error
	afterDelete    []func(key string, obj any, tx transaction.Client) error
	afterDeleteAll []func(tx transaction.Client) error
	beforeDropAll  []func(tx transaction.Client) error
}

// Test that Store implements cache.Indexer
var _ cache.Store = (*Store)(nil)

// NewStore creates a SQLite-backed cache.Store for objects of the given example type
func NewStore(ctx context.Context, example any, keyFunc cache.KeyFunc, c db.Client, shouldEncrypt bool, gvk schema.GroupVersionKind, name string, externalUpdateInfo *sqltypes.ExternalGVKUpdates, selfUpdateInfo *sqltypes.ExternalGVKUpdates) (*Store, error) {
	s := &Store{
		ctx:                ctx,
		name:               name,
		gvk:                gvk,
		externalUpdateInfo: externalUpdateInfo,
		selfUpdateInfo:     selfUpdateInfo,
		typ:                reflect.TypeOf(example),
		Client:             c,
		keyFunc:            keyFunc,
		shouldEncrypt:      shouldEncrypt,
		afterAdd:           []func(key string, obj any, tx transaction.Client) error{},
		afterUpdate:        []func(key string, obj any, tx transaction.Client) error{},
		afterDelete:        []func(key string, obj any, tx transaction.Client) error{},
		afterDeleteAll:     []func(tx transaction.Client) error{},
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
	s.dropBaseQuery = fmt.Sprintf(dropBaseStmtFmt, dbName)
	s.getQuery = fmt.Sprintf(getStmtFmt, dbName)
	s.listQuery = fmt.Sprintf(listStmtFmt, dbName)
	s.listKeysQuery = fmt.Sprintf(listKeysStmtFmt, dbName)

	s.upsertStmt = s.Prepare(s.upsertQuery)
	s.deleteStmt = s.Prepare(s.deleteQuery)
	s.deleteAllStmt = s.Prepare(s.deleteAllQuery)
	s.dropBaseStmt = s.Prepare(s.dropBaseQuery)
	s.getStmt = s.Prepare(s.getQuery)
	s.listStmt = s.Prepare(s.listQuery)
	s.listKeysStmt = s.Prepare(s.listKeysQuery)

	return s, nil
}

func isDBError(e error) bool {
	return strings.Contains(e.Error(), "SQL logic error: no such table:")
}

func (s *Store) checkUpdateExternalInfo(key string) {
	for _, updateBlock := range []*sqltypes.ExternalGVKUpdates{s.externalUpdateInfo, s.selfUpdateInfo} {
		if updateBlock != nil {
			s.WithTransaction(s.ctx, true, func(tx transaction.Client) error {
				err := s.updateExternalInfo(tx, key, updateBlock)
				if err != nil && !isDBError(err) {
					// Just report and ignore errors
					logrus.Errorf("Error updating external info %v: %s", s.externalUpdateInfo, err)
				}
				return nil
			})
		}
	}
}

// This function is called in two different conditions:
// Let's say resource B has a field X that we want to copy into resource A
// When a B is upserted, we update any A's that depend on it
// When an A is upserted, we check to see if any B's have that info
// The `key` argument here can belong to either an A or a B, depending on which resource is being updated.
// So it's only used in debug messages.
// The SELECT queries are more generic -- find *all* the instances of A that have a connection to B,
// ignoring any cases where A.X == B.X, as there's no need to update those.
//
// Some code later on in the function verifies that we aren't overwriting a non-empty value
// with the empty string. I assume this is never desired.

func (s *Store) updateExternalInfo(tx transaction.Client, key string, externalUpdateInfo *sqltypes.ExternalGVKUpdates) error {
	for _, labelDep := range externalUpdateInfo.ExternalLabelDependencies {
		rawGetStmt := fmt.Sprintf(`SELECT DISTINCT f.key, ex2."%s" FROM "%s_fields" f
  LEFT OUTER JOIN "%s_labels" lt1 ON f.key = lt1.key
  JOIN "%s_fields" ex2 ON lt1.value = ex2."%s"
 WHERE lt1.label = ? AND f."%s" != ex2."%s"`,
			labelDep.TargetFinalFieldName,
			labelDep.SourceGVK,
			labelDep.SourceGVK,
			labelDep.TargetGVK,
			labelDep.TargetKeyFieldName,
			labelDep.TargetFinalFieldName,
			labelDep.TargetFinalFieldName,
		)
		getStmt := s.Prepare(rawGetStmt)
		rows, err := s.QueryForRows(s.ctx, getStmt, labelDep.SourceLabelName)
		if err != nil {
			if !isDBError(err) {
				logrus.Infof("Error getting external info for table %s, key %s: %v", labelDep.TargetGVK, key, &db.QueryError{QueryString: rawGetStmt, Err: err})
			}
			continue
		}
		result, err := s.ReadStrings2(rows)
		if err != nil {
			logrus.Infof("Error reading objects for table %s, key %s: %s", labelDep.TargetGVK, key, err)
			continue
		}
		if len(result) == 0 {
			continue
		}
		for _, innerResult := range result {
			sourceKey := innerResult[0]
			finalTargetValue := innerResult[1]
			ignoreUpdate, err := s.overrideCheck(labelDep.TargetFinalFieldName, labelDep.SourceGVK, sourceKey, finalTargetValue)
			if ignoreUpdate || err != nil {
				continue
			}
			rawStmt := fmt.Sprintf(`UPDATE "%s_fields" SET "%s" = ? WHERE key = ?`,
				labelDep.SourceGVK, labelDep.TargetFinalFieldName)
			preparedStmt := s.Prepare(rawStmt)
			_, err = tx.Stmt(preparedStmt).Exec(finalTargetValue, sourceKey)
			if err != nil {
				logrus.Infof("Error running %s(%s, %s): %s", rawStmt, finalTargetValue, sourceKey, err)
				continue
			}
		}
	}
	for _, nonLabelDep := range externalUpdateInfo.ExternalDependencies {
		rawGetStmt := fmt.Sprintf(`SELECT DISTINCT f.key, ex2."%s"
 FROM "%s_fields" f JOIN "%s_fields" ex2 ON f."%s" = ex2."%s"
 WHERE f."%s" != ex2."%s"`,
			nonLabelDep.TargetFinalFieldName,
			nonLabelDep.SourceGVK,
			nonLabelDep.TargetGVK,
			nonLabelDep.SourceFieldName,
			nonLabelDep.TargetKeyFieldName,
			nonLabelDep.TargetFinalFieldName,
			nonLabelDep.TargetFinalFieldName)
		// TODO: Try to fold the two blocks together

		getStmt := s.Prepare(rawGetStmt)
		rows, err := s.QueryForRows(s.ctx, getStmt)
		if err != nil {
			if !isDBError(err) {
				logrus.Infof("Error getting external info for table %s, key %s: %v", nonLabelDep.TargetGVK, key, &db.QueryError{QueryString: rawGetStmt, Err: err})
			}
			continue
		}
		result, err := s.ReadStrings2(rows)
		if err != nil {
			logrus.Infof("Error reading objects for table %s, key %s: %s", nonLabelDep.TargetGVK, key, err)
			continue
		}
		if len(result) == 0 {
			continue
		}
		for _, innerResult := range result {
			sourceKey := innerResult[0]
			finalTargetValue := innerResult[1]
			ignoreUpdate, err := s.overrideCheck(nonLabelDep.TargetFinalFieldName, nonLabelDep.SourceGVK, sourceKey, finalTargetValue)
			if ignoreUpdate || err != nil {
				continue
			}
			rawStmt := fmt.Sprintf(`UPDATE "%s_fields" SET "%s" = ? WHERE key = ?`,
				nonLabelDep.SourceGVK, nonLabelDep.TargetFinalFieldName)
			preparedStmt := s.Prepare(rawStmt)
			_, err = tx.Stmt(preparedStmt).Exec(finalTargetValue, sourceKey)
			if err != nil {
				logrus.Infof("Error running %s(%s, %s): %s", rawStmt, finalTargetValue, sourceKey, err)
				continue
			}
			logrus.Debugf("QQQ: non-label-Updated %s[%s].%s to %s",
				nonLabelDep.SourceGVK,
				sourceKey,
				nonLabelDep.TargetFinalFieldName,
				finalTargetValue)
		}
	}
	return nil

}

// If the new value will change a non-empty current value, return [true, error:nil]
func (s *Store) overrideCheck(finalFieldName, sourceGVK, sourceKey, finalTargetValue string) (bool, error) {
	rawGetValueStmt := fmt.Sprintf(`SELECT f."%s" FROM  "%s_fields" f WHERE f.key = ?`,
		finalFieldName, sourceGVK)
	getValueStmt := s.Prepare(rawGetValueStmt)
	rows, err := s.QueryForRows(s.ctx, getValueStmt, sourceKey)
	if err != nil {
		logrus.Debugf("Checking the field, got error %s", err)
		return false, err
	}
	results, err := s.ReadStrings(rows)
	if err != nil {
		logrus.Infof("Checking the field for table %s, key %s, got error %s", sourceGVK, sourceKey, err)
		return false, err
	}
	if len(results) == 1 {
		currentValue := results[0]
		if len(currentValue) > 0 && len(finalTargetValue) == 0 {
			logrus.Debugf("Don't override %s key %s, field %s=%s with an empty string",
				sourceGVK,
				sourceKey,
				finalFieldName,
				currentValue)
			return true, nil
		}
	}
	return false, nil
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

/* Core methods */

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
	s.checkUpdateExternalInfo(key)
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
	s.checkUpdateExternalInfo(key)
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

func (s *Store) RegisterBeforeDropAll(f func(txC transaction.Client) error) {
	s.beforeDropAll = append(s.beforeDropAll, f)
}

// DropAll effectively removes the store from the database. The store must be
// recreated with NewStore.
//
// The store shouldn't be used once DropAll is called.
func (s *Store) DropAll(ctx context.Context) error {
	err := s.WithTransaction(ctx, true, func(tx transaction.Client) error {
		err := s.runBeforeDropAll(tx)
		if err != nil {
			return err
		}

		_, err = tx.Stmt(s.dropBaseStmt).Exec(s.GetName())
		if err != nil {
			return &db.QueryError{QueryString: s.dropBaseQuery, Err: err}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("dropall for %q: %w", s.GetName(), err)
	}
	return nil
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

func (s *Store) runBeforeDropAll(txC transaction.Client) error {
	for _, f := range s.beforeDropAll {
		err := f(txC)
		if err != nil {
			return err
		}
	}
	return nil
}
