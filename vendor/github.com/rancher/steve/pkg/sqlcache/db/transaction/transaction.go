/*
Package transaction provides mockable interfaces of sql package struct types.
*/
package transaction

import (
	"context"
	"database/sql"
)

// Client is an interface over a subset of sql.Tx methods
// rationale 1: explicitly forbid direct access to Commit and Rollback functionality
// as that is exclusively dealt with by WithTransaction in ../db
// rationale 2: allow mocking
type Client interface {
	Exec(query string, args ...any) (sql.Result, error)
	Stmt(stmt *sql.Stmt) Stmt
}

// client is the main implementation of Client, delegates to sql.Tx
// other implementations exist for testing purposes
type client struct {
	tx *sql.Tx
}

func NewClient(tx *sql.Tx) Client {
	return &client{tx: tx}
}

func (c client) Exec(query string, args ...any) (sql.Result, error) {
	return c.tx.Exec(query, args...)
}

func (c client) Stmt(stmt *sql.Stmt) Stmt {
	return c.tx.Stmt(stmt)
}

// Stmt is an interface over a subset of sql.Stmt methods
// rationale: allow mocking
type Stmt interface {
	Exec(args ...any) (sql.Result, error)
	Query(args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, args ...any) (*sql.Rows, error)
}
