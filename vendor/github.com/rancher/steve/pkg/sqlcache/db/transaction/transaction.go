/*
Package transaction provides a client for a live transaction, and interfaces for some relevant sql types. The transaction client automatically performs rollbacks  on failures.
The use of this package simplifies testing for callers by making the underlying transaction mock-able.
*/
package transaction

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Client provides a way to interact with the underlying sql transaction.
type Client struct {
	sqlTx SQLTx
}

// SQLTx represents a sql transaction
type SQLTx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Stmt(stmt *sql.Stmt) *sql.Stmt
	Commit() error
	Rollback() error
}

// Stmt represents a sql stmt. It is used as a return type to offer some testability over returning sql's Stmt type
// because we are able to mock its outputs and do not need an actual connection.
type Stmt interface {
	Exec(args ...any) (sql.Result, error)
	Query(args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, args ...any) (*sql.Rows, error)
}

// NewClient returns a Client with the given transaction assigned.
func NewClient(tx SQLTx) *Client {
	return &Client{sqlTx: tx}
}

// Commit commits the transaction and then unlocks the database.
func (c *Client) Commit() error {
	return c.sqlTx.Commit()
}

// Exec uses the sqlTX Exec() with the given stmt and args. The transaction will be automatically rolled back if Exec()
// returns an error.
func (c *Client) Exec(stmt string, args ...any) error {
	_, err := c.sqlTx.Exec(stmt, args...)
	if err != nil {
		return c.rollback(c.sqlTx, err)
	}
	return nil
}

// Stmt adds the given sql.Stmt to the client's transaction and then returns a Stmt. An interface is being returned
// here to aid in testing callers by providing a way to configure the statement's behavior.
func (c *Client) Stmt(stmt *sql.Stmt) Stmt {
	s := c.sqlTx.Stmt(stmt)
	return s
}

// StmtExec Execs the given statement with the given args. It assumes the stmt has been added to the transaction. The
// transaction is rolled back if Stmt.Exec() returns an error.
func (c *Client) StmtExec(stmt Stmt, args ...any) error {
	_, err := stmt.Exec(args...)
	if err != nil {
		logrus.Debugf("StmtExec failed: query %s, args: %s, err: %s", stmt, args, err)
		return c.rollback(c.sqlTx, err)
	}
	return nil
}

// rollback handles rollbacks and wraps errors if needed
func (c *Client) rollback(tx SQLTx, err error) error {
	rerr := tx.Rollback()
	if rerr != nil {
		return errors.Wrapf(err, "Encountered error, then encountered another error while rolling back: %v", rerr)
	}
	return errors.Wrapf(err, "Encountered error, successfully rolled back")
}

// Cancel rollbacks the transaction without wrapping an error. This only needs to be called if Client has not returned
// an error yet or has not committed. Otherwise, transaction has already rolled back, or in the case of Commit() it is too
// late.
func (c *Client) Cancel() error {
	rerr := c.sqlTx.Rollback()
	if rerr != sql.ErrTxDone {
		return rerr
	}
	return nil
}
