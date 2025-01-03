// Package tql provides a type-safe SQL query builder and executor that uses Go templates
// and struct reflection to generate and execute SQL queries.
package tql

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"iter"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"text/template"
)

var (
	// log is the package logger
	log = slog.Default().WithGroup("tql")

	// tagRegex matches key=value pairs in struct tags
	tagRegex = regexp.MustCompile(`(\w+)(?:=([^;]*))?`)

	// selectRegex matches SELECT statements to parse column selection
	selectRegex = regexp.MustCompile(`(?m)(?is)SELECT\s+(.+?)\s+FROM\b`)

	// cteRegex matches CTEs to parse column selection
	cteRegex = regexp.MustCompile(`(?ms)(?:\bWITH\s+)?([a-zA-Z_][a-zA-Z0-9_]+)\s+AS\s*\((.*?)\)`)

	// defaultFunctions contains the default template functions
	defaultFunctions = Functions{}

	// ErrNilQuery is returned when attempting to use a nil query
	ErrNilQuery = errors.New("query is nil")
	// ErrNilStmt is returned when attempting to use a nil statement
	ErrNilStmt = errors.New("statement is nil")
	// ErrNilTemplate is returned when attempting to use a nil template
	ErrNilTemplate = errors.New("template is nil")

	// ErrPreparingQuery is returned when query preparation fails
	ErrPreparingQuery = errors.New("failed to prepare query")

	// ErrInvalidQueryable is returned when the queryable is not a valid type
	ErrInvalidQueryable = errors.New("invalid queryable")

	// ErrExecutingQuery is returned when query execution fails
	ErrExecutingQuery = errors.New("failed to execute query")

	// ErrParsingQuery is returned when SQL template parsing fails
	ErrParsingQuery = errors.New("failed to parse sql template")

	// ErrParsingTemplate is returned when Go template parsing fails
	ErrParsingTemplate = errors.New("failed to parse template")

	// ErrParsingSQL is returned when SQL syntax parsing fails
	ErrParsingSQL = errors.New("failed to parse sql")

	// ErrInvalidType is returned when the type parameter is not a struct
	ErrInvalidType = errors.New("failed to create query type parameter is invalid")

	// ErrUnsupportedCTE is returned when the sql template contains unsupported CTEs
	ErrUnsupportedCTE = errors.New("unsupported CTEs in sql template")
)

// Functions is an alias for template.Functions to provide custom template functions
type Functions = template.FuncMap
type Params = map[string]any

type DbOrTx interface {
	*sql.DB | *sql.Tx
}

// Template is an interface that represents a template that can be generated
type Template interface {
	Generate(data ...any) (string, error)
	MustGenerate(data ...any) string
}

// QueryTemplate is a struct that represents a template that can be generated
type QueryTemplate[T any] struct {
	template *template.Template
}

// QueryStmt is a struct that represents a prepared statement that can be executed
type QueryStmt[T any] struct {
	template *QueryTemplate[T]
	prepared *sql.Stmt
	indices  [][]int
	SQL      string
}

// New creates a new QueryTemplate with the given SQL template and optional template functions.
// The type parameter T must be a struct that is a table or a struct that contains tables.
//
// Example table struct:
//
//	type User struct {
//	    ID        int
//	    Name      string
//	    CreatedAt time.Time
//	}
//
// Example struct containing tables:
//
//	type UserWithAccount struct {
//	    User    User    `tql:"user"` // optional tag to specify the table alias
//	    Account Account `tql:"account"` // optional tag to specify the table alias
//	}
//
// The sqlTemplate parameter supports Go template syntax for dynamic SQL generation.
// Template variables can be accessed using {{ .VarName }} syntax. see https://pkg.go.dev/text/template for more details.
//
// Example usage:
//
//	query, err := New[User]("SELECT * FROM users WHERE created_at > {{ .since }}")
//	query, err := New[UserWithAccount]("SELECT Users.*, Accounts.* FROM Users JOIN Accounts ON Users.id = Accounts.user_id")
//
// Optional template functions can be provided to extend template capabilities. see https://pkg.go.dev/text/template#FuncMap for more details.
// If no functions are provided, default functions will be used.
//
// Parameters:
//   - sqlTemplate: The SQL template string to use for the query.
//   - maybeFunctions: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - *QueryTemplate[S]: A new QueryTemplate with the given SQL template and optional template functions.
//   - error: If the query template parsing fails
func New[T any](sqlTemplate string, maybeFunctions ...Functions) (*QueryTemplate[T], error) {
	funcs := defaultFunctions
	if len(maybeFunctions) > 0 {
		funcs = maybeFunctions[0]
	}
	var s T
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Struct {
		log.Error("a struct is required", "received", s)
		return nil, ErrInvalidType
	}
	if strings.HasPrefix(strings.TrimSpace(sqlTemplate), "WITH") {
		log.Error("sql template contains unsupported CTEs", "sql", sqlTemplate)
		return nil, ErrUnsupportedCTE
	}
	tmpl, err := template.New(v.Type().Name()).Funcs(template.FuncMap(funcs)).Option("missingkey=zero").Parse(sqlTemplate)
	if err != nil {
		log.Error("failed to create query with functions", "error", err)
		return nil, errors.Join(ErrParsingTemplate, err)
	}
	query := &QueryTemplate[T]{template: tmpl}
	return query, nil
}

// Must creates a new QueryTemplate and panics if an error occurs.
// This is useful for queries that are known to be valid at compile time.
// The type parameter T must be a struct that is a table or a struct that contains tables. see New[T] for more details.
//
// Example usage:
//
//	query := Must[User]("SELECT * FROM users WHERE id = ?")
//
// Parameters:
//   - sqlTemplate: The SQL template string to use for the query.
//   - maybePipelines: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - *QueryTemplate[S]: A new QueryTemplate with the given SQL template and optional template functions.
//   - error: If the query template parsing fails
//
// Note: Only use Must for queries that are guaranteed to be valid, otherwise use New to handle errors gracefully.
func Must[T any](sqlTemplate string, maybePipelines ...Functions) *QueryTemplate[T] {
	q, err := New[T](sqlTemplate, maybePipelines...)
	if err != nil {
		panic(err)
	}
	return q
}

// Query executes a QueryTemplate with the given database connection and optional template data.
// It returns a slice of results of type T and any error that occurred.
//
// The type parameter T specifies the result type, which must be a struct. See New[T] for more details.
// The type parameter Q must be either *sql.DB or *sql.Tx.
//
// Parameters:
//   - query: The QueryTemplate to execute. Must not be nil.
//   - db: Database connection, can be either *sql.DB or *sql.Tx
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - []T: A slice of results of type T
//   - error: If query preparation or execution fails
func Query[T any, Q DbOrTx](query *QueryTemplate[T], db Q, data ...any) ([]T, error) {
	return QueryContext(query, context.Background(), db, data...)
}

// QueryContext executes a QueryTemplate with the given context, database connection, and optional template data.
// It returns a slice of results of type T and any error that occurred.
//
// The type parameter T specifies the result type, which must be a struct. See New[S] for more details.
// The type parameter Q must be either *sql.DB or *sql.Tx.
//
// Parameters:
//   - query: The QueryTemplate to execute. Must not be nil.
//   - ctx: The context for the query execution. Used for cancellation and timeouts.
//   - db: Database connection, can be either *sql.DB or *sql.Tx
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - []T: A slice of results of type T
//   - error: If query preparation or execution fails
func QueryContext[T any, Q DbOrTx](query *QueryTemplate[T], ctx context.Context, txOrDb Q, data ...any) ([]T, error) {
	results := []T{}
	if query == nil {
		log.ErrorContext(ctx, "Execute called on a nil query", "error", ErrNilQuery)
		return results, errors.Join(ErrExecutingQuery, ErrNilQuery)
	}
	var err error
	stmt, err := PrepareContext(query, ctx, txOrDb)
	if err != nil {
		return results, errors.Join(ErrExecutingQuery, err)
	}
	return stmt.QueryContext(ctx, data...)
}

// ExecContext executes a QueryTemplate with the given context, database connection, and optional template data.
// It returns the result of the query execution and any error that occurred.
//
// The type parameter T specifies the result type, which must be a struct. See New[S] for more details.
// The type parameter Q must be either *sql.DB or *sql.Tx.
//
// Parameters:
//   - query: The QueryTemplate to execute. Must not be nil.
//   - ctx: The context for the query execution. Used for cancellation and timeouts.
//   - db: Database connection, can be either *sql.DB or *sql.Tx
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - sql.Result containing the execution results
//   - error if query preparation or execution fails
func ExecContext[T any, Q DbOrTx](query *QueryTemplate[T], ctx context.Context, db Q, data ...any) (sql.Result, error) {
	if query == nil {
		log.ErrorContext(ctx, "Execute called on a nil query", "error", ErrNilQuery)
		return nil, errors.Join(ErrExecutingQuery, ErrNilQuery)
	}
	stmt, err := PrepareContext(query, ctx, db)
	if err != nil {
		log.ErrorContext(ctx, "failed to prepare query", "error", err)
		return nil, errors.Join(ErrExecutingQuery, err)
	}
	return stmt.ExecContext(ctx, data...)
}

// Exec executes a QueryTemplate with the given database connection and optional template data.
// It returns the result of the query execution and any error that occurred.
//
// The type parameter T specifies the result type, which must be a struct. See New[S] for more details.
// The type parameter Q must be either *sql.DB or *sql.Tx.
//
// Parameters:
//   - query: The QueryTemplate to execute. Must not be nil.
//   - db: Database connection, can be either *sql.DB or *sql.Tx
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - sql.Result containing the execution results
//   - error if query preparation or execution fails
func Exec[T any, Q DbOrTx](query *QueryTemplate[T], db Q, data ...any) (sql.Result, error) {
	return ExecContext(query, context.Background(), db, data...)
}

// Generate generates the SQL template with the given data and returns the generated SQL string and any error that occurred.
//
// Parameters:
//   - query: The QueryTemplate to generate. Must not be nil.
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - string: The generated SQL string
//   - error: If the template execution fails
func Generate[T any](query *QueryTemplate[T], data ...any) (string, error) {
	if query == nil {
		log.Error("Generate called on a nil query")
		return "", ErrNilQuery
	}
	var buf bytes.Buffer
	templateData := any(nil)
	if len(data) > 0 {
		templateData = data[0]
	}
	if err := query.template.Execute(&buf, templateData); err != nil {
		log.Error("error executing template", "error", err)
		return "", errors.Join(ErrPreparingQuery, err)
	}
	return buf.String(), nil
}

// MustGenerate generates the SQL template with the given data and returns the generated SQL string.
// It panics if an error occurs.
//
// Parameters:
//   - query: The QueryTemplate to generate. Must not be nil.
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - string: The generated SQL string or an empty string if the template execution fails
func MustGenerate[T any](query *QueryTemplate[T], data ...any) string {

	sql, err := Generate(query, data...)
	if err != nil {
		panic(err)
	}
	return sql
}

// PrepareContext prepares a QueryTemplate with the given context, database connection, and optional template data.
// It returns a prepared statement and any error that occurred.
// NOTE: Like Go Stmt, the prepared statement is invalidated once the transaction is committed or rolled back. You are responsible for closing the statement or re-preparing it.
//
// The type parameter T specifies the result type, which must be a struct. See New[S] for more details.
// The type parameter Q must be either *sql.DB or *sql.Tx.
//
// Parameters:
//   - query: The QueryTemplate to prepare. Must not be nil.
//   - ctx: The context for the query preparation. Used for cancellation and timeouts.
//   - txOrDb: Database connection, can be either *sql.DB or *sql.Tx
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - *QueryStmt[T]: A prepared statement
//   - error: If query preparation fails
func PrepareContext[T any, Q DbOrTx](query *QueryTemplate[T], ctx context.Context, txOrDb Q, data ...any) (*QueryStmt[T], error) {
	// make sure the query is not nil
	if query == nil {
		log.ErrorContext(ctx, "Prepare called on a nil query")
		return nil, errors.Join(ErrPreparingQuery, ErrNilQuery)
	}
	if query.template == nil {
		// this should never happen but just in case we will check it anyway
		log.ErrorContext(ctx, "Prepare called with a nil template")
		return nil, errors.Join(ErrPreparingQuery, ErrNilTemplate)
	}
	if txOrDb == nil {
		log.ErrorContext(ctx, "Prepare called with a nil tx or db")
		return nil, errors.Join(ErrPreparingQuery, ErrPreparingQuery)
	}
	generatedSQL, err := Generate(query, data...)
	if err != nil {
		log.ErrorContext(ctx, "Error parsing sql template", "error", err)
		return nil, errors.Join(ErrPreparingQuery, err)
	}
	transformedSQL, indices := Parse[T](generatedSQL)
	var stmt *sql.Stmt
	switch db := any(txOrDb).(type) {
	case *sql.DB:
		stmt, err = db.PrepareContext(ctx, transformedSQL)
	case *sql.Tx:
		stmt, err = db.PrepareContext(ctx, transformedSQL)
	default:
		log.ErrorContext(ctx, "Prepare called with an invalid queryable", "error", ErrPreparingQuery)
		return nil, errors.Join(ErrPreparingQuery, ErrInvalidQueryable)
	}
	if err != nil {
		log.ErrorContext(ctx, "failed to prepare query", "error", err)
		return nil, errors.Join(ErrPreparingQuery, err)
	}
	queryStmt := &QueryStmt[T]{template: query, indices: indices, SQL: transformedSQL, prepared: stmt}
	return queryStmt, nil
}

// Prepare prepares a QueryTemplate with the given database connection and optional template data.
// It returns a prepared statement and any error that occurred.
//
// The type parameter T specifies the result type, which must be a struct. See New[S] for more details.
// The type parameter Q must be either *sql.DB or *sql.Tx.
//
// Parameters:
//   - query: The QueryTemplate to prepare. Must not be nil.
//   - db: Database connection, can be either *sql.DB or *sql.Tx
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - *QueryStmt[T]: A prepared statement
//   - error: If query preparation fails
func Prepare[T any, Q DbOrTx](tqlQuery *QueryTemplate[T], db Q, data ...any) (*QueryStmt[T], error) {
	return PrepareContext(tqlQuery, context.Background(), db, data...)
}

// Parse parses the SQL string and extracts field information for scanning
//
// Parameters:
//   - sql: The SQL string to parse
//
// Returns:
//   - string: The parsed SQL string
//   - [][]int: The indices of the fields that are selected
func Parse[T any](sql string) (string, [][]int) {
	var tmp T
	tableOrTables := reflect.ValueOf(tmp).Type()
	selectedFields := []string{}
	matches := selectRegex.FindAllStringSubmatch(sql, -1)
	allIndices := [][]int{}
	// parse the sql template to see if we are selecting all fields
	if len(matches) > 0 {
		selectAll := strings.TrimSpace(matches[0][1]) == "*"
		splitFields := strings.Split(matches[0][1], ",")
		// iterate over the fields of the struct to get the indices of the fields that we are selecting
		for tableOrField := range iterStructFields(tableOrTables) {
			tableName := ""
			tableOrFieldType := tableOrField.Type
			indices := []int{}
			tableOrFieldTag := parseTQLTag(tableOrField)
			if tableOrFieldType.Kind() != reflect.Struct {
				// this means that this is a single table query
				tableOrFieldType = tableOrTables
			} else {
				tableName = tableOrFieldTag.field
				indices = append(indices, tableOrField.Index[0])
			}
			// to select all fields from the table means we have a "*" or a "X.*" and that the fields are narrowed by a subquery
			selectAllFromTable := (selectAll || containsWords(matches[0][1], tableName+`\.\*`)) && !matchesContainsWords(matches, tableName+`\.\b`)
			for field := range iterStructFields(tableOrFieldType) {
				fieldTag := parseTQLTag(field)
				var qualifiedName string
				if tableName != "" {
					qualifiedName = tableName + "." + fieldTag.field
				} else {
					qualifiedName = fieldTag.field
				}
				// check if the field is omitted via the tql tag or the table tql tag
				if fieldTag.omit == "true" || containsWords(tableOrFieldTag.omit, fieldTag.field, qualifiedName) {
					continue
				}
				if !matchesContainsWords(matches, qualifiedName, tableName+`\.`+fieldTag.field, fieldTag.field) && !selectAllFromTable {
					log.Debug("column not found in the sql statement", "column", qualifiedName, "sql", sql)
					continue
				}
				selectedFields = append(selectedFields, toSelectedField(qualifiedName, splitFields))
				allIndices = append(allIndices, append(indices[:], field.Index...))
			}

			if tableOrFieldType == tableOrTables {
				// make sure we break out of this loop if this is a single table query
				break
			}
		}
		// replace the selected fields with the qualified names
		sql = strings.Replace(sql, matches[0][1], strings.Join(selectedFields, ", "), 1)
	}
	return sql, allIndices
}

// Generate generates the SQL template with the given data and returns the generated SQL string and any error that occurred.
//
// Parameters:
//   - query: The QueryTemplate to generate. Must not be nil.
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - string: The generated SQL string
//   - error: If the template execution fails
func (query *QueryTemplate[T]) Generate(data ...any) (string, error) {
	return Generate(query, data...)
}

// MustGenerate generates the SQL template with the given data and returns the generated SQL string.
// It panics if an error occurs.
//
// Parameters:
//   - query: The QueryTemplate to generate. Must not be nil.
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - string: The generated SQL string
//   - error: If the template execution fails
func (query *QueryTemplate[T]) MustGenerate(data ...any) string {
	return MustGenerate(query, data...)
}

// Close closes the prepared statement and any error that occurred.
//
// Parameters:
//   - query: The QueryStmt to close. Must not be nil.
//
// Returns:
//   - error: If closing the prepared statement fails
func (query *QueryStmt[T]) Close() error {
	if query == nil {
		log.Error("Close called on a nil query")
		return ErrNilQuery
	}
	if query.prepared != nil {
		query.prepared.Close()
		query.prepared = nil
	}
	return nil
}

// ExecContext executes a prepared statement with the given context and optional template data.
// It returns the result of the query execution and any error that occurred.
//
// Parameters:
//   - query: The QueryStmt to execute. Must not be nil.
//   - ctx: The context for the query execution. Used for cancellation and timeouts.
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - sql.Result: The result of the query execution
//   - error: If query execution fails
func (query *QueryStmt[T]) ExecContext(ctx context.Context, data ...any) (sql.Result, error) {
	if query == nil {
		log.ErrorContext(ctx, "ExecContext called on a nil query")
		return nil, ErrNilQuery
	}
	if query.prepared == nil {
		log.ErrorContext(ctx, "ExecContext called on a nil prepared query")
		return nil, ErrNilStmt
	}
	return query.prepared.ExecContext(ctx, data...)
}

// Exec executes a prepared statement with the given database connection and optional template data.
// It returns the result of the query execution and any error that occurred.
//
// Parameters:
//   - query: The QueryStmt to execute. Must not be nil.
//   - db: Database connection, can be either *sql.DB or *sql.Tx
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - sql.Result: The result of the query execution
//   - error: If query execution fails
func (query *QueryStmt[T]) Exec(data ...any) (sql.Result, error) {
	if query == nil {
		log.Error("Exec called on a nil query")
		return nil, ErrNilQuery
	}
	return query.ExecContext(context.Background(), data...)
}

// QueryContext executes a prepared statement with the given context and optional template data.
// It returns a slice of results of type T and any error that occurred.
//
// Parameters:
//   - query: The QueryStmt to execute. Must not be nil.
//   - ctx: The context for the query execution. Used for cancellation and timeouts.
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - []T: A slice of results of type T
//   - error: If query execution fails
func (query *QueryStmt[T]) QueryContext(ctx context.Context, data ...any) (results []T, err error) {
	if query == nil {
		log.ErrorContext(ctx, "QueryContext called on a nil query")
		return nil, ErrNilQuery
	}
	var scanDest T
	scanDestValue := reflect.ValueOf(&scanDest).Elem()
	fields := []any{}
	for _, fieldIndex := range query.indices {
		field := scanDestValue.FieldByIndex(fieldIndex)
		fields = append(fields, field.Addr().Interface())
	}
	rows, err := query.prepared.QueryContext(ctx, data...)
	if err != nil {
		return results, errors.Join(ErrExecutingQuery, err)
	}
	for rows.Next() {
		err := rows.Scan(fields...)
		if err != nil {
			return results, errors.Join(ErrExecutingQuery, err)
		}
		results = append(results, scanDest)
	}
	return results, nil
}

// Query executes a prepared statement with the given database connection and optional template data.
// It returns a slice of results of type T and any error that occurred.
//
// Parameters:
//   - query: The QueryStmt to execute. Must not be nil.
//   - db: Database connection, can be either *sql.DB or *sql.Tx
//   - data: Optional variadic parameters to pass to the query execution
//
// Returns:
//   - []T: A slice of results of type T
//   - error: If query execution fails
func (query *QueryStmt[T]) Query(data ...any) (results []T, err error) {
	if query == nil {
		log.Error("Query called on a nil query")
		return nil, ErrNilQuery
	}
	return query.QueryContext(context.Background(), data...)
}

// parseTQLTag parses the tql struct tag options
//
// Parameters:
//   - field: The struct field to parse
//
// Returns:
//   - struct {
//     omit  string
//     field string
//     }: The parsed struct tag options
func parseTQLTag(field reflect.StructField) (results struct {
	omit  string
	field string
}) {
	matches := tagRegex.FindAllStringSubmatch(field.Tag.Get("tql"), -1)
	results.field = field.Name
	for _, match := range matches {
		value := strings.TrimSpace(match[2])
		if value != "" {
			switch strings.TrimSpace(match[1]) {
			case "omit":
				results.omit = strings.TrimSpace(match[2])
			}
			continue
		} else if value != "-" {
			results.field = strings.TrimSpace(match[0])
		}
	}
	return results
}

// toSelectedField converts the qualified name to the selected field
//
// Parameters:
//   - qualifiedName: The qualified name of the field
//   - selectedFields: The selected fields
//
// Returns:
//   - string: The selected field
func toSelectedField(qualifiedName string, selectedFields []string) string {
	for _, field := range selectedFields {
		maybeAlias := strings.Split(field, " as ")
		if len(maybeAlias) > 1 {
			if strings.TrimSpace(maybeAlias[1]) == qualifiedName {
				return maybeAlias[0] + " as " + qualifiedName
			}
		}
	}
	return qualifiedName
}

// matchesContainsWords checks if the matches contain any of the words
//
// Parameters:
//   - matches: The matches to check
//   - words: The words to check for
//
// Returns:
//   - bool: True if any of the words are found in the matches, false otherwise
func matchesContainsWords(matches [][]string, words ...string) bool {
	for _, match := range matches {
		if containsWords(match[1], words...) {
			return true
		}
	}
	return false
}

// containsWords checks if the source string contains any of the words
//
// Parameters:
//   - source: The source string to check
//   - words: The words to check for
//
// Returns:
//   - bool: True if any of the words are found in the source string, false otherwise
func containsWords(source string, words ...string) bool {
	for _, word := range words {
		regex, err := regexp.Compile(`(^|[^.])\b` + word)
		if err != nil {
			return false
		}
		if regex.MatchString(source) {
			return true
		}
	}
	return false
}

// iterStructFields returns an iterator over the fields of a struct type
//
// Parameters:
//   - reflectedType: The reflected type of the struct
//
// Returns:
//   - iter.Seq[reflect.StructField]: An iterator over the fields of the struct
func iterStructFields(reflectedType reflect.Type) iter.Seq[reflect.StructField] {
	return iter.Seq[reflect.StructField](
		func(yield func(reflect.StructField) bool) {
			for tableIndex := 0; tableIndex < reflectedType.NumField(); tableIndex++ {
				if !yield(reflectedType.Field(tableIndex)) {
					return
				}
			}
		},
	)
}
