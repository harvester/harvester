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
	"io/fs"
	"os"
	"reflect"
	"sync"

	"github.com/pkg/errors"
	"github.com/rancher/steve/pkg/sqlcache/db/transaction"

	// needed for drivers
	_ "modernc.org/sqlite"
)

const (
	// InformerObjectCacheDBPath is where SQLite's object database file will be stored relative to process running steve
	InformerObjectCacheDBPath = "informer_object_cache.db"

	informerObjectCachePerms fs.FileMode = 0o600
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
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
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

// QueryError encapsulates an error while executing a query
type QueryError struct {
	QueryString string
	Err         error
}

// Error returns a string representation of this QueryError
func (e *QueryError) Error() string {
	return "while executing query: " + e.QueryString + " got error: " + e.Err.Error()
}

// Unwrap returns the underlying error
func (e *QueryError) Unwrap() error {
	return e.Err
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

	return stmt.QueryContext(ctx, params...)
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

// BeginTx attempts to begin a transaction.
// If forWriting is true, this method blocks until all other concurrent forWriting
// transactions have either committed or rolled back.
// If forWriting is false, it is assumed the returned transaction will exclusively
// be used for DQL (e.g. SELECT) queries.
// Not respecting the above rule might result in transactions failing with unexpected
// SQLITE_BUSY (5) errors (aka "Runtime error: database is locked").
// See discussion in https://github.com/rancher/lasso/pull/98 for details
func (c *Client) BeginTx(ctx context.Context, forWriting bool) (TXClient, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()
	// note: this assumes _txlock=immediate in the connection string, see NewConnection
	sqlTx, err := c.conn.BeginTx(ctx, &sql.TxOptions{
		ReadOnly: !forWriting,
	})
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

	// Set the permissions in advance, because we can't control them if
	// the file is created by a sql.Open call instead.
	if err := touchFile(InformerObjectCacheDBPath, informerObjectCachePerms); err != nil {
		return nil
	}

	sqlDB, err := sql.Open("sqlite", "file:"+InformerObjectCacheDBPath+"?"+
		// open SQLite file in read-write mode, creating it if it does not exist
		"mode=rwc&"+
		// use the WAL journal mode for consistency and efficiency
		"_pragma=journal_mode=wal&"+
		// do not even attempt to attain durability. Database is thrown away at pod restart
		"_pragma=synchronous=off&"+
		// do check foreign keys and honor ON DELETE CASCADE
		"_pragma=foreign_keys=on&"+
		// if two transactions want to write at the same time, allow 2 minutes for the first to complete
		// before baling out
		"_pragma=busy_timeout=120000&"+
		// default to IMMEDIATE mode for transactions. Setting this parameter is the only current way
		// to be able to switch between DEFERRED and IMMEDIATE modes in modernc.org/sqlite's implementation
		// of BeginTx
		"_txlock=immediate")
	if err != nil {
		return err
	}

	c.conn = sqlDB
	return nil
}

// This acts like "touch" for both existing files and non-existing files.
// permissions.
//
// It's created with the correct perms, and if the file already exists, it will
// be chmodded to the correct perms.
func touchFile(filename string, perms fs.FileMode) error {
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, perms)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	return os.Chmod(filename, perms)
}
