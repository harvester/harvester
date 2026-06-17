/*
Package db offers client struct and  functions to interact with database connection. It provides encrypting, decrypting,
and a way to reset the database.
*/
package db

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rancher/steve/pkg/sqlcache/db/logging"

	"github.com/sirupsen/logrus"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	// needed for drivers
	_ "modernc.org/sqlite"
)

const (
	// InformerObjectCacheDBPath is where SQLite's object database file will be stored relative to process running steve
	// It's given in two parts because the root is used as the suffix for the tempfile, and then we'll add a ".db" after it.
	// In non-test mode, we can append the ".db" extension right here.
	InformerObjectCacheDBPathRoot = "informer_object_cache"
	InformerObjectCacheDBPath     = InformerObjectCacheDBPathRoot + ".db"

	informerObjectCachePerms fs.FileMode = 0o600

	maxBeginTXAttemptsOnBusyErrors = 3

	debugQueryLogPathEnvVar           = "CATTLE_DEBUG_QUERY_LOG"
	debugQueryIncludeParamsPathEnvVar = "CATTLE_DEBUG_QUERY_INCLUDE_PARAMS"
)

// Client defines a database client that provides encrypting, decrypting, and database resetting
type Client interface {
	WithTransaction(ctx context.Context, forWriting bool, f WithTransactionFunction) error
	Prepare(stmt string) (Stmt, error)
	QueryForRows(ctx context.Context, stmt Stmt, params ...any) (Rows, error)
	ReadObjects(rows Rows, typ reflect.Type) ([]any, error)
	ReadStrings(rows Rows) ([]string, error)
	ReadStringsN(rows Rows, numColumns int) ([][]string, error)
	ReadInt(rows Rows) (int, error)
	ReadStringIntString1or2(rows Rows, readThirdString bool) ([][]string, error)
	Upsert(tx TxClient, stmt Stmt, key string, obj SerializedObject) error
	NewConnection(isTemp bool) (string, error)
	Serialize(obj any, encrypt bool) (SerializedObject, error)
	Deserialize(SerializedObject, any) error
}

// WithTransaction runs f within a transaction.
//
// If forWriting is true, this method blocks until all other concurrent forWriting
// transactions have either committed or rolled back.
// If forWriting is false, it is assumed the returned transaction will exclusively
// be used for DQL (e.g. SELECT) queries.
// Not respecting the above rule might result in transactions failing with unexpected
// SQLITE_BUSY (5) errors (aka "Runtime error: database is locked").
// See discussion in https://github.com/rancher/lasso/pull/98 for details
//
// The transaction is committed if f returns nil, otherwise it is rolled back.
func (c *client) WithTransaction(ctx context.Context, forWriting bool, f WithTransactionFunction) error {
	if err := c.withTransaction(ctx, forWriting, f); err != nil {
		return fmt.Errorf("transaction: %w", err)
	}
	return nil
}

func (c *client) withTransaction(ctx context.Context, forWriting bool, f WithTransactionFunction) error {
	c.connLock.RLock()
	// note: this assumes _txlock=immediate in the connection string, see NewConnection
	tx, err := c.beginTX(ctx, forWriting)
	c.connLock.RUnlock()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if err := f(NewTxClient(tx, WithQueryLogger(c.queryLogger))); err != nil {
		rerr := c.rollback(ctx, tx)
		return errors.Join(err, rerr)
	}

	return c.commit(ctx, tx)
}

// beginTX handles automatic retries for writing transactions for specific error codes.
// Rationale: in WAL mode, BEGIN IMMEDIATE requires 2 steps: create a read transaction and promote it, which can fail if another thread wrote to the database in the meantime.
// See https://github.com/rancher/rancher/issues/52872 for more details
func (c *client) beginTX(ctx context.Context, forWriting bool) (Tx, error) {
	if !forWriting {
		return c.conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	}

	var attempts int
	for {
		attempts++
		tx, err := c.conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: false})
		if err == nil {
			return tx, nil
		} else if attempts == maxBeginTXAttemptsOnBusyErrors || !isRetriableSQLiteError(err) {
			return nil, err
		}
	}
}

func isRetriableSQLiteError(err error) bool {
	var serr *sqlite.Error
	if !errors.As(err, &serr) {
		return false
	}

	switch serr.Code() {
	case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_BUSY_SNAPSHOT:
		return true
	default:
		return false
	}
}

func (c *client) commit(ctx context.Context, tx Tx) error {
	err := tx.Commit()
	// When the context.Context given to BeginTx is canceled, then the
	// Tx is rolled back automatically, so rolling back again could have failed.
	if errors.Is(err, sql.ErrTxDone) && ctx.Err() == context.Canceled {
		return fmt.Errorf("commit failed due to canceled context")
	}
	return err
}

func (c *client) rollback(ctx context.Context, tx Tx) error {
	err := tx.Rollback()
	// When the context.Context given to BeginTx is canceled, then the
	// Tx is rolled back automatically, so rolling back again could have failed.
	if errors.Is(err, sql.ErrTxDone) && ctx.Err() == context.Canceled {
		return fmt.Errorf("rollback failed due to canceled context")
	}
	return err
}

// WithTransactionFunction is a function that uses a transaction
type WithTransactionFunction func(tx TxClient) error

// client is the main implementation of Client. Other implementations exist for test purposes
type client struct {
	Client
	conn      Connection
	connLock  sync.RWMutex
	encryptor Encryptor
	decryptor Decryptor
	encoding  encoding

	queryLogger logging.QueryLogger
}

// Connection represents a connection pool.
type Connection interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)
	Exec(query string, args ...any) (sql.Result, error)
	Prepare(query string) (*sql.Stmt, error)
	Close() error
}

type connection struct {
	*sql.DB
}

func (c *connection) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	return c.DB.BeginTx(ctx, opts)
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

type ClientOption func(*client)

// NewClient returns a client and the path to the database. If the given connection is nil then a default one will be created.
func NewClient(ctx context.Context, c Connection, encryptor Encryptor, decryptor Decryptor, useTempDir bool, opts ...ClientOption) (Client, string, error) {
	client := &client{
		encryptor: encryptor,
		decryptor: decryptor,
		encoding:  defaultEncoding,
	}
	for _, o := range opts {
		o(client)
	}
	if c != nil {
		client.conn = c
		return client, "", nil
	}
	dbPath, err := client.NewConnection(useTempDir)
	if err != nil {
		return nil, "", err
	}

	logger, err := logging.StartQueryLogger(ctx, os.Getenv(debugQueryLogPathEnvVar), os.Getenv(debugQueryIncludeParamsPathEnvVar) == "true")
	if err != nil {
		return nil, "", fmt.Errorf("starting query logger: %w", err)
	}
	client.queryLogger = logger

	return client, dbPath, nil
}

// Prepare prepares the given string into a sql statement on the client's connection.
func (c *client) Prepare(queryString string) (Stmt, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()
	prepared, err := c.conn.Prepare(queryString)
	if err != nil {
		return nil, fmt.Errorf("preparing statement: %s: %w", queryString, err)
	}
	return &stmt{
		Stmt:        prepared,
		queryString: queryString,
	}, nil
}

// QueryForRows queries the given stmt with the given params and returns the resulting rows. The query wil be retried
// given a sqlite busy error.
func (c *client) QueryForRows(ctx context.Context, stmt Stmt, params ...any) (Rows, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()

	return stmt.QueryContext(ctx, params...)
}

// ReadObjects Scans the given rows, performs any necessary decryption, converts the data to objects of the given type,
// and returns a slice of those objects.
func (c *client) ReadObjects(rows Rows, typ reflect.Type) ([]any, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()

	var result []any
	for rows.Next() {
		row, err := c.readRow(rows)
		if err != nil {
			return nil, closeRowsOnError(rows, err)
		}
		dest := reflect.New(typ.Elem()).Interface()
		if err := c.Deserialize(row, dest); err != nil {
			return nil, closeRowsOnError(rows, err)
		}
		result = append(result, dest)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	return result, nil
}

// ReadStrings scans the given rows into strings, and then returns the strings as a slice.
func (c *client) ReadStrings(rows Rows) ([]string, error) {
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
	if err := rows.Close(); err != nil {
		return nil, err
	}

	return result, nil
}

// ReadStringIntString1or2 scans rows into (string, int, string) tuples, and when readThirdString
// is true, into (string, int, string, string) tuples.
func (c *client) ReadStringIntString1or2(rows Rows, readThirdString bool) ([][]string, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()

	var result [][]string
	for rows.Next() {
		var val1 string
		var val2 int
		var val3 string
		if readThirdString {
			var val4 string
			err := rows.Scan(&val1, &val2, &val3, &val4)
			if err != nil {
				return nil, closeRowsOnError(rows, err)
			}
			result = append(result, []string{val1, strconv.Itoa(val2), val3, val4})
			continue
		}

		err := rows.Scan(&val1, &val2, &val3)
		if err != nil {
			return nil, closeRowsOnError(rows, err)
		}
		result = append(result, []string{val1, strconv.Itoa(val2), val3})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	return result, nil
}

// ReadStringsN scans the given rows into string-slices of the specified number of columns
func (c *client) ReadStringsN(rows Rows, numColumns int) ([][]string, error) {
	c.connLock.RLock()
	defer c.connLock.RUnlock()

	var result [][]string
	stringPointers := make([]any, numColumns)
	for rows.Next() {
		// stringList needs be reinitialized on each iteration or all the final data in
		// the `result` array will be the last row!
		stringList := make([]string, numColumns)
		for i := range stringList {
			stringPointers[i] = &stringList[i]
		}
		err := rows.Scan(stringPointers...)
		if err != nil {
			return nil, closeRowsOnError(rows, err)
		}
		result = append(result, stringList)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	return result, nil
}

// ReadInt scans the first of the given rows into a single int (eg. for COUNT() queries)
func (c *client) ReadInt(rows Rows) (int, error) {
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

	if err := rows.Close(); err != nil {
		return 0, err
	}

	return result, nil
}

type SerializedObject struct {
	Bytes sql.RawBytes
	// only set if encrypted
	Nonce sql.RawBytes
	KeyID uint32
}

func (s SerializedObject) encrypted() bool {
	return len(s.Nonce) > 0
}

func (c *client) readRow(rows Rows) (SerializedObject, error) {
	var obj SerializedObject
	if err := rows.Scan(&obj.Bytes, &obj.Nonce, &obj.KeyID); err != nil {
		return SerializedObject{}, err
	}
	return obj, nil
}

func (c *client) Serialize(obj any, encrypt bool) (SerializedObject, error) {
	var buf bytes.Buffer
	if err := c.encoding.Encode(&buf, obj); err != nil {
		return SerializedObject{}, err
	}

	if !encrypt {
		return SerializedObject{Bytes: buf.Bytes()}, nil
	}

	if c.encryptor == nil {
		return SerializedObject{}, fmt.Errorf("cannot encrypt object object without encryptor")
	}
	data, nonce, kid, err := c.encryptor.Encrypt(buf.Bytes())
	if err != nil {
		return SerializedObject{}, err
	}

	return SerializedObject{Bytes: data, Nonce: nonce, KeyID: kid}, nil
}

func (c *client) Deserialize(serialized SerializedObject, dest any) error {
	if !serialized.encrypted() {
		return c.encoding.Decode(bytes.NewReader(serialized.Bytes), dest)
	}

	if c.encryptor == nil {
		return fmt.Errorf("cannot deserialize encrypted object without decryptor")
	}
	data, err := c.decryptor.Decrypt(serialized.Bytes, serialized.Nonce, serialized.KeyID)
	if err != nil {
		return err
	}
	return c.encoding.Decode(bytes.NewReader(data), dest)
}

// Upsert executes an upsert statement
// note the statement should have 4 parameters: key, objBytes, dataNonce, kid
func (c *client) Upsert(tx TxClient, stmt Stmt, key string, serialized SerializedObject) error {
	_, err := tx.Stmt(stmt).Exec(key, serialized.Bytes, serialized.Nonce, serialized.KeyID)
	return err
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
func (c *client) NewConnection(useTempDir bool) (string, error) {
	c.connLock.Lock()
	defer c.connLock.Unlock()
	if c.conn != nil {
		err := c.conn.Close()
		if err != nil {
			return "", err
		}
	}
	if !useTempDir {
		for _, suffix := range []string{"", "-shm", "-wal"} {
			f := InformerObjectCacheDBPath + suffix
			err := os.RemoveAll(f)
			if err != nil {
				logrus.Errorf("error removing existing db file %s: %v", f, err)
			}
		}
	}

	// Set the permissions in advance, because we can't control them if
	// the file is created by a sql.Open call instead.
	var dbPath string
	if useTempDir {
		dir := os.TempDir()
		f, err := os.CreateTemp(dir, InformerObjectCacheDBPathRoot)
		if err != nil {
			return "", err
		}
		path := f.Name()
		dbPath = path + ".db"
		f.Close()
		os.Remove(path)
	} else {
		dbPath = InformerObjectCacheDBPath
	}
	if err := touchFile(dbPath, informerObjectCachePerms); err != nil {
		return dbPath, nil
	}

	sqlDB, err := sql.Open("sqlite", "file:"+dbPath+"?"+
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
		// store temporary tables to memory, to speed up queries making use
		// of temporary tables (eg: when using DISTINCT)
		"_pragma=temp_store=2&"+
		// default to IMMEDIATE mode for transactions. Setting this parameter is the only current way
		// to be able to switch between DEFERRED and IMMEDIATE modes in modernc.org/sqlite's implementation
		// of BeginTx
		"_txlock=immediate")
	if err != nil {
		return dbPath, err
	}
	sqlite.RegisterDeterministicScalarFunction("adjustTimestampForSorting", 1, adjustTimestampForSorting)
	sqlite.RegisterDeterministicScalarFunction("extractBarredValue", 2, extractBarredValue)
	sqlite.RegisterDeterministicScalarFunction("hasBarredValue", 2, hasBarredValue)
	sqlite.RegisterDeterministicScalarFunction("inet_aton", 1, inetAtoN)
	sqlite.RegisterDeterministicScalarFunction("memoryInBytes", 1, memoryInBytes)
	c.conn = &connection{sqlDB}
	return dbPath, nil
}

func adjustTimestampForSorting(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	var arg1 int64
	switch argTyped := args[0].(type) {
	case int64:
		arg1 = argTyped
	default:
		return nil, fmt.Errorf("unsupported type for arg1: expected an integer, got :%T", args[0])
	}
	if arg1 == 0 {
		return arg1, nil
	}
	arg1 = time.Now().Unix() - arg1
	return arg1, nil
}

func extractBarredValue(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	var arg1 string
	var arg2 int
	switch argTyped := args[0].(type) {
	case string:
		arg1 = argTyped
	case []byte:
		arg1 = string(argTyped)
	default:
		return nil, fmt.Errorf("unsupported type for arg1: expected a string, got :%T", args[0])
	}
	var err error
	switch argTyped := args[1].(type) {
	case int:
		arg2 = argTyped
	case string:
		arg2, err = strconv.Atoi(argTyped)
	case []byte:
		arg2, err = strconv.Atoi(string(argTyped))
	default:
		return nil, fmt.Errorf("unsupported type for arg2: expected an int, got: %T", args[0])
	}
	if err != nil {
		return nil, fmt.Errorf("extractBarredValue: problem with arg2: %w", err)
	}
	parts := strings.Split(arg1, "|")
	if arg2 >= len(parts) || arg2 < 0 {
		return "", nil
	}
	return parts[arg2], nil
}

func hasBarredValue(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	var arg1 string
	var arg2 string
	switch argTyped := args[0].(type) {
	case string:
		arg1 = argTyped
	case []byte:
		arg1 = string(argTyped)
	default:
		return nil, fmt.Errorf("hasBarredValue: unsupported type for arg1: expected a string, got :%T", args[0])
	}
	switch argTyped := args[1].(type) {
	case string:
		arg2 = argTyped
	case []byte:
		arg2 = string(argTyped)
	default:
		return nil, fmt.Errorf("hasBarredValue: unsupported type for arg2: expected a string, got: %T", args[0])
	}
	parts := strings.Split(arg1, "|")
	return slices.Contains(parts, arg2), nil
}

func inetAtoN(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	var arg1 string
	switch argTyped := args[0].(type) {
	case string:
		arg1 = argTyped
	case []byte:
		arg1 = string(argTyped)
	default:
		logrus.Errorf("inetAtoN: unsupported type for arg1: expected a string, got :%T", args[0])
		return int64(0), nil
	}
	ip := net.ParseIP(arg1)
	if ip == nil {
		logrus.Errorf("inetAtoN: invalid IP address: %s", arg1)
		return int64(0), nil
	}
	ipAs4 := ip.To4()
	if ipAs4 != nil {
		return int64(binary.BigEndian.Uint32(ipAs4)), nil
	}
	// By elimination it must be IPv6 (until IPv[n > 6] comes along one day
	ipAs16 := ip.To16()
	if ipAs16 == nil {
		logrus.Errorf("inetAtoN: invalid IPv6 address: %s", arg1)
		return int64(0), nil
	}
	return int64(binary.BigEndian.Uint64(ipAs16)), nil
}

// Convert a string representation of memory to a float giving the number of bytes
// See the `tbl` var for associated values of each suffix
// Values returned as REAL to allow for large values
func memoryInBytes(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	var arg1 string
	var val float64
	var finalValue driver.Value
	finalValue = val
	switch argTyped := args[0].(type) {
	case string:
		arg1 = argTyped
	case []byte:
		arg1 = string(argTyped)
	default:
		return finalValue, fmt.Errorf("unsupported type for arg1: expected a string, got :%T", args[0])
	}
	rx := `^([0-9]+)(\w{0,2})$`
	ptn := regexp.MustCompile(rx)
	m := ptn.FindStringSubmatch(arg1)
	if m == nil || len(m) != 3 {
		return finalValue, fmt.Errorf("couldn't parse '%s' as a numeric value", arg1)
	}
	tbl := map[string]int{
		"B": 0,
		"K": 1,
		"M": 2,
		"G": 3,
		"T": 4,
		"E": 5,
	}
	size, err := strconv.Atoi(m[1])
	if err != nil {
		return finalValue, fmt.Errorf("couldn't parse '%s' as a numeric value: %w", arg1, err)
	}
	factor := 0
	base := 1024
	var finalError error
	if len(m[2]) > 0 {
		var ok bool
		factor, ok = tbl[strings.ToUpper(m[2][0:1])]
		if !ok {
			factor = 0
		}
		if len(m[2]) > 2 {
			finalError = fmt.Errorf("numeric value '%s' has an unrecognized suffix '%s'", arg1, m[2])
		} else if len(m[2]) == 2 {
			if strings.ToUpper(m[2][1:2]) == "I" {
				base = 1000
			} else {
				finalError = fmt.Errorf("numeric value '%s' has an unrecognized suffix '%s'", arg1, m[2])
			}
		}
	}
	val = float64(size) * math.Pow(float64(base), float64(factor))
	finalValue = val
	return finalValue, finalError
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
