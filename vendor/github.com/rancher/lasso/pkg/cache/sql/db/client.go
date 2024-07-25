/*
Package db offers client struct and  functions to interact with database connection. It provides encrypting, decrypting,
and a way to reset the database.
*/
package db

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/lasso/pkg/cache/sql/attachdriver"
	"github.com/rancher/lasso/pkg/cache/sql/db/transaction"
	"k8s.io/apimachinery/pkg/util/wait"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	// InformerObjectCacheDBPath is where SQLite's object database file will be stored relative to process running lasso
	InformerObjectCacheDBPath = "informer_object_cache.db"
	// OnDiskInformerIndexedFieldDBPath is where SQLite's indexed fields database file will be stored if, env var value
	// found at EncryptAllEnvVar is "false".
	OnDiskInformerIndexedFieldDBPath = "informer_object_fields.db"
)

// Client is a database client that provides encrypting, decrypting, and database resetting.
type Client struct {
	conn      Connection
	connLock  sync.RWMutex
	encryptor Encryptor
	decryptor Decryptor
}

// Connection represents a connection pool.
type Connection interface {
	Begin() (*sql.Tx, error)
	Exec(query string, args ...any) (sql.Result, error)
	Prepare(query string) (*sql.Stmt, error)
	Close() error
}

// Closable Closes an underlying connection and returns an error on failure.
type Closable interface {
	Close() error
}

// Rows represents sql rows. It exposes method to navigate the rows, read their outputs, and close them.
type Rows interface {
	Next() bool
	Err() error
	Close() error
	Scan(dest ...any) error
}

// TXClient represents a sql transaction. The TXClient must manage rollbacks as rollback functionality is not exposed.
type TXClient interface {
	StmtExec(stmt transaction.Stmt, args ...any) error
	Exec(stmt string, args ...any) error
	Commit() error
	Stmt(stmt *sql.Stmt) transaction.Stmt
	Cancel() error
}

// Encryptor encrypts data with a key which is rotated to avoid wear-out.
type Encryptor interface {
	// Encrypt encrypts the specified data, returning: the encrypted data, the nonce used to encrypt the data, and an ID identifying the key that was used (as it rotates). On failure error is returned instead.
	Encrypt([]byte) ([]byte, []byte, uint32, error)
}

// Decryptor decrypts data previously encrypted by Encryptor.
type Decryptor interface {
	// Decrypt accepts a chunk of encrypted data, the nonce used to encrypt it and the ID of the used key (as it rotates). It returns the decrypted data or an error.
	Decrypt([]byte, []byte, uint32) ([]byte, error)
}

var backoffRetry = wait.Backoff{Duration: 50 * time.Millisecond, Factor: 2, Steps: 10}

// NewClient returns a Client. If the given connection is nil then a default one will be created.
func NewClient(c Connection, encryptor Encryptor, decryptor Decryptor) (*Client, error) {
	client := &Client{
		encryptor: encryptor,
		decryptor: decryptor,
	}
	if c != nil {
		client.conn = c
		return client, nil
	}
	err := client.NewConnection()
	if err != nil {
		return nil, err
	}
	return client, nil
}

// Prepare prepares the given string into a sql statement on the client's connection.
func (c *Client) Prepare(stmt string) *sql.Stmt {
	c.connLock.RLock()
	defer c.connLock.RUnlock()
	prepared, err := c.conn.Prepare(stmt)
	if err != nil {
		panic(errors.Errorf("Error preparing statement: %s\n%v", stmt, err))
	}
	return prepared
}

// QueryForRows queries the given stmt with the given params and returns the resulting rows. The query wil be retried
// given a sqlite busy error.
func (c *Client) QueryForRows(ctx context.Context, stmt transaction.Stmt, params ...any) (*sql.Rows, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()
	var rows *sql.Rows
	var err error

	err = wait.ExponentialBackoff(backoffRetry, func() (bool, error) {
		rows, err = stmt.QueryContext(ctx, params...)
		if err != nil {
			sqlErr, ok := err.(*sqlite.Error)
			if ok && sqlErr.Code() == sqlite3.SQLITE_BUSY {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// CloseStmt will call close on the given Closable. It is intended to be used with a sql statement. This function is meant
// to replace stmt.Close which can cause panics when callers unit-test since there usually is no real underlying connection.
func (c *Client) CloseStmt(closable Closable) error {
	return closable.Close()
}

// ReadObjects Scans the given rows, performs any necessary decryption, converts the data to objects of the given type,
// and returns a slice of those objects.
func (c *Client) ReadObjects(rows Rows, typ reflect.Type, shouldDecrypt bool) ([]any, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()

	var result []any
	for rows.Next() {
		data, err := c.decryptScan(rows, shouldDecrypt)
		if err != nil {
			return nil, closeRowsOnError(rows, err)
		}
		singleResult, err := fromBytes(data, typ)
		if err != nil {
			return nil, closeRowsOnError(rows, err)
		}
		result = append(result, singleResult.Elem().Interface())
	}
	err := rows.Err()
	if err != nil {
		return nil, closeRowsOnError(rows, err)
	}

	err = rows.Close()
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ReadStrings scans the given rows into strings, and then returns the strings as a slice.
func (c *Client) ReadStrings(rows Rows) ([]string, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()

	var result []string
	for rows.Next() {
		var key string
		err := rows.Scan(&key)
		if err != nil {
			return nil, closeRowsOnError(rows, err)
		}

		result = append(result, key)
	}
	err := rows.Err()
	if err != nil {
		return nil, closeRowsOnError(rows, err)
	}

	err = rows.Close()
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ReadInt scans the first of the given rows into a single int (eg. for COUNT() queries)
func (c *Client) ReadInt(rows Rows) (int, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()

	if !rows.Next() {
		return 0, closeRowsOnError(rows, sql.ErrNoRows)
	}

	var result int
	err := rows.Scan(&result)
	if err != nil {
		return 0, closeRowsOnError(rows, err)
	}

	err = rows.Err()
	if err != nil {
		return 0, closeRowsOnError(rows, err)
	}

	err = rows.Close()
	if err != nil {
		return 0, err
	}

	return result, nil
}

// Begin attempt to begin a transaction, and returns it along with a function for unlocking the
// database once the transaction is done.
func (c *Client) Begin() (TXClient, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()
	sqlTx, err := c.conn.Begin()
	if err != nil {
		return nil, err
	}
	return transaction.NewClient(sqlTx), nil
}

func (c *Client) decryptScan(rows Rows, shouldDecrypt bool) ([]byte, error) {
	var data, dataNonce sql.RawBytes
	var kid uint32
	err := rows.Scan(&data, &dataNonce, &kid)
	if err != nil {
		return nil, err
	}
	if c.decryptor != nil && shouldDecrypt {
		decryptedData, err := c.decryptor.Decrypt(data, dataNonce, kid)
		if err != nil {
			return nil, err
		}
		return decryptedData, nil
	}
	return data, nil
}

// Upsert used to be called upsertEncrypted in store package before move
func (c *Client) Upsert(tx TXClient, stmt *sql.Stmt, key string, obj any, shouldEncrypt bool) error {
	objBytes := toBytes(obj)
	var dataNonce []byte
	var err error
	var kid uint32
	if c.encryptor != nil && shouldEncrypt {
		objBytes, dataNonce, kid, err = c.encryptor.Encrypt(objBytes)
		if err != nil {
			return err
		}
	}

	return tx.StmtExec(tx.Stmt(stmt), key, objBytes, dataNonce, kid)
}

// toBytes encodes an object to a byte slice
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

// fromBytes decodes an object from a byte slice
func fromBytes(buf sql.RawBytes, typ reflect.Type) (reflect.Value, error) {
	dec := gob.NewDecoder(bytes.NewReader(buf))
	singleResult := reflect.New(typ)
	err := dec.DecodeValue(singleResult)
	return singleResult, err
}

// closeRowsOnError closes the sql.Rows object and wraps errors if needed
func closeRowsOnError(rows Rows, err error) error {
	ce := rows.Close()
	if ce != nil {
		return fmt.Errorf("error in closing rows while handling %s: %w", err.Error(), ce)
	}

	return err
}

// NewConnection checks for currently existing connection, closes one if it exists, removes any relevant db files, and opens a new connection which subsequently
// creates new files.
func (c *Client) NewConnection() error {
	c.connLock.Lock()
	defer c.connLock.Unlock()
	if c.conn != nil {
		err := c.conn.Close()
		if err != nil {
			return err
		}
	}
	err := os.RemoveAll(InformerObjectCacheDBPath)
	if err != nil {
		return err
	}

	err = os.RemoveAll(OnDiskInformerIndexedFieldDBPath)
	if err != nil {
		return err
	}
	sqlDB, err := sql.Open(attachdriver.Name, "file:"+InformerObjectCacheDBPath+"?mode=rwc&cache=shared&_journal_mode=wal&_synchronous=off&_foreign_keys=on&_busy_timeout=1000000")
	if err != nil {
		return err
	}

	// necessary to prevent memory races. Otherwise, all connections will close and in-memory database for fields table
	// will be closed automatically. Setting max connections also minimizes SQLITE_BUSY errors.
	// https://github.com/mattn/go-sqlite3/blob/master/README.md#faq
	sqlDB.SetConnMaxIdleTime(-1)
	sqlDB.SetConnMaxLifetime(-1)

	c.conn = sqlDB
	return nil
}
