package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const (
	maxSQLLength = 10000
	// maxQueryRows and maxQueryResultBytes keep an answer small enough for
	// the context of a model.
	maxQueryRows        = 200
	maxQueryResultBytes = 256 << 10
	// maxQueryRowBytes is the largest row that the connection reads: 2 MB,
	// so that the 1 MB text of one source fits. Without it, one row can
	// have 1 GB.
	maxQueryRowBytes    = 2 << 20
	defaultQueryTimeout = 5 * time.Second
)

// QueryResult is the answer of Query.
type QueryResult struct {
	Columns []string `json:"columns"`
	// Rows holds each value as the text that Postgres gives, such as
	// 2026-10-11 for a date and t for true. A nil value is a NULL.
	Rows [][]*string `json:"rows"`
	// Truncated is true when the query has more rows than Rows holds.
	Truncated bool `json:"truncated"`
}

// UseQueryPool sets the pool that Query uses. The user of the pool must
// have no right to change a table: UseQueryPool refuses a pool that can
// write, so a wrong database URL cannot make Query a way to change data.
func (s *Service) UseQueryPool(ctx context.Context, pool *pgxpool.Pool) error {
	canChange, err := store.New(pool).CanChangeATable(ctx)
	if err != nil {
		return fmt.Errorf("query pool: %w", err)
	}
	if canChange {
		return errors.New("query pool: its user can change a table; use the read-only user lifeos_query")
	}
	s.queryPool = pool
	return nil
}

// Query runs one SQL statement as the read-only user and returns its rows.
// It is the one function that takes SQL from a caller: an agent writes the
// statement, so the database, not this code, is what stops a change. The
// statement runs in a read-only transaction, for at most 5 seconds.
func (s *Service) Query(ctx context.Context, sql string) (QueryResult, error) {
	if s.queryPool == nil {
		return QueryResult{}, errors.New("query: the service has no query pool")
	}
	if strings.TrimSpace(sql) == "" {
		return QueryResult{}, invalid("sql", "must not be empty")
	}
	if len(sql) > maxSQLLength {
		return QueryResult{}, invalid("sql", fmt.Sprintf("must be at most %d characters", maxSQLLength))
	}
	if !isText(sql) {
		return QueryResult{}, invalid("sql", "must be valid text without a NUL character")
	}

	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	conn, err := s.queryPool.Acquire(ctx)
	if err != nil {
		return QueryResult{}, fmt.Errorf("query: %w", err)
	}
	// A statement can leave a setting or a lock in its session. The
	// connection closes after each query, so the next query starts clean.
	// The close also ends the transaction.
	defer func() {
		conn.Conn().Close(context.Background())
		conn.Release()
	}()
	conn.Conn().PgConn().Frontend().SetMaxBodyLen(maxQueryRowBytes)
	if _, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}); err != nil {
		return QueryResult{}, fmt.Errorf("query: %w", err)
	}

	// ExecParams sends the statement as one prepared statement with no
	// parameter, so Postgres refuses a text that has two statements. It
	// gives each value in the text format, as psql shows it.
	reader := conn.Conn().PgConn().ExecParams(ctx, sql, nil, nil, nil, nil)
	result := QueryResult{Columns: []string{}, Rows: [][]*string{}}
	for _, field := range reader.FieldDescriptions() {
		result.Columns = append(result.Columns, field.Name)
	}
	size := 0
	for reader.NextRow() {
		if len(result.Rows) == maxQueryRows || size > maxQueryResultBytes {
			result.Truncated = true
			break
		}
		raw := reader.Values()
		row := make([]*string, len(raw))
		for i, value := range raw {
			if value != nil {
				text := string(value)
				row[i] = &text
				size += len(value)
			}
		}
		result.Rows = append(result.Rows, row)
	}
	// After a cut, the rows that the result holds are the answer.
	if _, err := reader.Close(); err != nil && !result.Truncated {
		return QueryResult{}, s.queryError(ctx, err)
	}
	return result, nil
}

// queryError turns an error of the statement into a ValidationError, so
// that the agent reads what is wrong with its SQL.
func (s *Service) queryError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return invalid("sql", fmt.Sprintf("took longer than %s: ask for fewer rows", s.queryTimeout))
	}
	var tooLarge *pgproto3.ExceededMaxBodyLenErr
	if errors.As(err, &tooLarge) {
		return invalid("sql", "a row is too large: select fewer columns, or cut a long text with left()")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return invalid("sql", "PostgreSQL refused the statement: "+pgErr.Message)
	}
	return fmt.Errorf("query: %w", err)
}
