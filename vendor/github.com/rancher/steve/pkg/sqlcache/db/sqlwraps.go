package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/rancher/steve/pkg/sqlcache/db/logging"
)

// Row implements a subset of the methods provided by sql.Row
type Row interface {
	Err() error
	Scan(dest ...any) error
}

// Rows represents sql rows. It exposes method to navigate the rows, read their outputs, and close them.
type Rows interface {
	Next() bool
	Err() error
	Close() error
	Scan(dest ...any) error
}

// Stmt is an interface over a subset of sql.Stmt methods
// rationale: allow mocking
type Stmt interface {
	Exec(args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, args ...any) (Rows, error)
	Close() error

	// SQLStmt unwraps the original sql.Stmt
	SQLStmt() *sql.Stmt

	// GetQueryString returns the original text used to prepare this statement
	GetQueryString() string
}

// rows wraps a sql.Rows, keeping track of the original query used to produce it
type rows struct {
	*sql.Rows
	queryString string
}

// Err wraps the original *sql.Rows's Err() with a QueryError
func (r rows) Err() error {
	if err := r.Rows.Err(); err != nil {
		return &QueryError{QueryString: r.queryString, Err: err}
	}
	return nil
}

// stmt implements the Stmt interface, wrapping a sql.Stmt and keeping track of the original query string
// Most of the methods will wrap original errors with a QueryError
type stmt struct {
	*sql.Stmt
	queryString string

	queryLogger logging.QueryLogger
}

func (s *stmt) log(startTime time.Time, query string, args []any) {
	if s.queryLogger == nil {
		return
	}
	s.queryLogger.Log(startTime, query, args)
}

func (s *stmt) Exec(args ...any) (sql.Result, error) {
	defer s.log(time.Now(), s.queryString, args)
	res, err := s.Stmt.Exec(args...)
	if err != nil {
		err = &QueryError{
			QueryString: s.queryString,
			Err:         err,
		}
	}
	return res, err
}

func (s *stmt) QueryContext(ctx context.Context, args ...any) (Rows, error) {
	defer s.log(time.Now(), s.queryString, args)
	res, err := s.Stmt.QueryContext(ctx, args...)
	if err != nil {
		return res, &QueryError{
			QueryString: s.queryString,
			Err:         err,
		}
	}
	return rows{Rows: res, queryString: s.queryString}, nil
}

func (s *stmt) Close() error {
	if err := s.Stmt.Close(); err != nil {
		return &QueryError{QueryString: s.queryString, Err: err}
	}
	return nil
}

func (s *stmt) SQLStmt() *sql.Stmt {
	return s.Stmt
}

func (s *stmt) GetQueryString() string {
	return s.queryString
}
