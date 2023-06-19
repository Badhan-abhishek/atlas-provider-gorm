// package recordriver provides a driver for database/sql which records queries and statements
// and allows you to set responses for queries. It is used for testing or providing a runtime replacement
// for a real database in cases where you want to learn the queries and statements that are executed.

package recordriver

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
)

func init() {
	sql.Register("recordriver", &drv{})
}

var (
	sessions = map[string]*session{}
	mu       sync.Mutex
)

type (
	// session is a session of recordriver which records queries and statements.
	session struct {
		Queries    []string
		Statements []string
		responses  map[string]*Response
	}
	// Response is a response to a query.
	Response struct {
		Cols []string
		Data [][]driver.Value
	}
	drv  struct{}
	conn struct {
		session string
	}
	stmt struct {
		query   string
		session string
	}
	tx          struct{}
	emptyResult struct{}
)

// Stmts returns the statements as a string, separated by semicolons and newlines.
func (s *session) Stmts() string {
	var sb strings.Builder
	for _, stmt := range s.Statements {
		sb.WriteString(stmt)
		sb.WriteString(";\n")
	}
	return sb.String()
}

// Session returns the session with the given name and reports whether it exists.
func Session(name string) (*session, bool) {
	mu.Lock()
	defer mu.Unlock()
	h, ok := sessions[name]
	return h, ok
}

// SetResponse sets the response for the given session and query.
func SetResponse(s string, query string, resp *Response) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := sessions[s]; !ok {
		sessions[s] = &session{
			responses: make(map[string]*Response),
		}
	}
	sessions[s].responses[query] = resp
}

// Open returns a new connection to the database.
func (d *drv) Open(name string) (driver.Conn, error) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := sessions[name]; !ok {
		sessions[name] = &session{
			responses: make(map[string]*Response),
		}
	}
	return &conn{session: name}, nil
}

// Prepare returns a prepared statement, bound to this connection.
func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return &stmt{query: query, session: c.session}, nil
}

// Close closes the connection.
func (c *conn) Close() error {
	mu.Lock()
	defer mu.Unlock()
	delete(sessions, c.session)
	return nil
}

// Begin starts and returns a new transaction.
func (c *conn) Begin() (driver.Tx, error) {
	return &tx{}, nil
}

// Commit commits the transaction. It is a noop.
func (*tx) Commit() error {
	return nil
}

// Rollback rolls back the transaction. It is a noop.
func (*tx) Rollback() error {
	return nil
}

// Close closes the statement.
func (*stmt) Close() error {
	return nil
}

// NumInput returns the number of placeholder parameters. Reporting -1 does not know the
// number of parameters.
func (*stmt) NumInput() int {
	return -1
}

// Exec executes a query that doesn't return rows, such as an CREATE or ALTER TABLE.
func (s *stmt) Exec(_ []driver.Value) (driver.Result, error) {
	mu.Lock()
	defer mu.Unlock()
	sessions[s.session].Statements = append(sessions[s.session].Statements, s.query)
	return emptyResult{}, nil
}

// Query executes a query that may return rows, such as an SELECT.
func (s *stmt) Query(_ []driver.Value) (driver.Rows, error) {
	mu.Lock()
	defer mu.Unlock()
	sess := s.session
	sessions[sess].Queries = append(sessions[sess].Queries, s.query)
	if resp, ok := sessions[sess].responses[s.query]; ok {
		return resp, nil
	}
	return &Response{}, nil
}

// Columns returns the names of the columns in the result set.
func (r *Response) Columns() []string {
	return r.Cols
}

// Close closes the rows iterator. It is a noop.
func (*Response) Close() error {
	return nil
}

// Next is called to populate the next row of data into the provided slice.
func (r *Response) Next(dest []driver.Value) error {
	if len(r.Data) == 0 {
		return io.EOF
	}
	copy(dest, r.Data[0])
	r.Data = r.Data[1:]
	return nil
}

// LastInsertId returns the integer generated by the database in response to a command. LastInsertId
// always returns a value of 0.
func (emptyResult) LastInsertId() (int64, error) {
	return 0, nil
}

// RowsAffected returns the number of rows affected by the query. RowsAffected always returns a
// value of 0.
func (emptyResult) RowsAffected() (int64, error) {
	return 0, nil
}