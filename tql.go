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

	// selectAllRegex matches SELECT statements to parse column selection
	selectAllRegex = regexp.MustCompile(`(?m)(?is)SELECT\s+(.+?)\s+FROM\b`)

	// defaultFunctions contains the default template functions
	defaultFunctions = Functions{}

	// ErrNilQuery is returned when attempting to use a nil query
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
)

// Functions is an alias for template.Functions to provide custom template functions
type Functions template.FuncMap
type Params map[string]any

type DbOrTx interface {
	*sql.DB | *sql.Tx
}

// QueryTemplate implements the Query interface with template and statement preparation
type Template interface {
	Generate(data ...any) (string, error)
	MustGenerate(data ...any) string
}

type QueryTemplate[T any] struct {
	template *template.Template
}

type QueryStmt[T any] struct {
	template *QueryTemplate[T]
	prepared *sql.Stmt
	indices  [][]int
	SQL      string
}

// New creates a new query with default template functions
func New[S any](sqlTemplate string, maybeFunctions ...Functions) (*QueryTemplate[S], error) {
	funcs := defaultFunctions
	if len(maybeFunctions) > 0 {
		funcs = maybeFunctions[0]
	}
	var s S
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Struct {
		log.Error("a struct is required", "received", s)
		return nil, ErrInvalidType
	}
	tmpl, err := template.New(v.Type().Name()).Funcs(template.FuncMap(funcs)).Option("missingkey=zero").Parse(sqlTemplate)
	if err != nil {
		log.Error("failed to create query with functions", "error", err)
		return nil, errors.Join(ErrParsingTemplate, err)
	}
	query := &QueryTemplate[S]{template: tmpl}
	return query, nil
}

// Must creates a new query and panics if an error occurs
func Must[S any](sqlTemplate string, maybePipelines ...Functions) *QueryTemplate[S] {
	q, err := New[S](sqlTemplate, maybePipelines...)
	if err != nil {
		panic(err)
	}
	return q
}

func Query[T any, Q DbOrTx](query *QueryTemplate[T], db Q, data ...any) ([]T, error) {
	return QueryContext(query, context.Background(), db, data...)
}

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

func Exec[T any, Q DbOrTx](query *QueryTemplate[T], db Q, data ...any) (sql.Result, error) {
	return ExecContext(query, context.Background(), db, data...)
}

func Generate[T any](query *QueryTemplate[T], data ...any) (string, error) {
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

func MustGenerate[T any](query *QueryTemplate[T], data ...any) string {
	sql, err := Generate(query, data...)
	if err != nil {
		return ""
	}
	return sql
}

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
	// register a function to cleanup the query when the context is done
	context.AfterFunc(ctx, func() {
		queryStmt.Close()
	})
	return queryStmt, nil
}

func Prepare[T any, Q DbOrTx](tqlQuery *QueryTemplate[T], db Q, data ...any) (*QueryStmt[T], error) {
	return PrepareContext(tqlQuery, context.Background(), db, data...)
}

// Parse parses the SQL string and extracts field information for scanning
func Parse[T any](sql string) (string, [][]int) {
	var tmp T
	tableOrTables := reflect.ValueOf(tmp).Type()
	selectedFields := []string{}
	matches := selectAllRegex.FindAllStringSubmatch(sql, -1)
	allIndices := [][]int{}
	// parse the sql template to see if we are selecting all fields
	if len(matches) > 0 {
		selectAll := strings.TrimSpace(matches[0][1]) == "*"
		// iterate over the fields of the struct to get the indices of the fields that we are selecting
		for tableOrField := range iterStructFields(tableOrTables) {
			tableName := ""
			tableOrFieldType := tableOrField.Type
			indices := []int{}
			tableOrFieldTag := parseTQLTag(tableOrField)
			if tableOrFieldType.Kind() != reflect.Struct {
				// this means that this is a single table query
				tableOrFieldType = tableOrTables
				tableName = tableOrTables.Name()
			} else {
				tableName = tableOrFieldTag.field // parseFieldName(tableOrField)
				indices = append(indices, tableOrField.Index[0])
			}
			// to select all fields from the table means we have a "*" or a "X.*" and that the fields are narrowed by a subquery
			selectAllFromTable := (selectAll || containsWords(matches[0][1], tableName+`\.\*`)) && !matchesContainsWords(matches, tableName+`\.\b`)
			for field := range iterStructFields(tableOrFieldType) {
				// fieldName := parseFieldName(field)
				fieldTag := parseTQLTag(field)
				qualifiedName := tableName + "." + fieldTag.field
				if fieldTag.omit == "true" || containsWords(tableOrFieldTag.omit, fieldTag.field, qualifiedName) {
					continue
				}
				if !matchesContainsWords(matches, tableName+`\.`+fieldTag.field, fieldTag.field) && !selectAllFromTable {
					log.Debug("column not found in the sql statement", "column", qualifiedName, "sql", sql)
					continue
				}
				selectedFields = append(selectedFields, qualifiedName)
				allIndices = append(allIndices, append(indices[:], field.Index...))
			}

			if tableOrFieldType == tableOrTables {
				// make sure we break out of this loop if this is a single table query
				break
			}
		}
		sql = strings.Replace(sql, matches[0][1], strings.Join(selectedFields, ", "), 1)
	}
	return sql, allIndices
}

func (query *QueryTemplate[T]) Generate(data ...any) (string, error) {
	return Generate(query, data...)
}

func (query *QueryTemplate[T]) MustGenerate(data ...any) string {
	return MustGenerate(query, data...)
}

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

func (query *QueryStmt[T]) ExecContext(ctx context.Context, data ...any) (sql.Result, error) {
	if query.prepared == nil {
		log.ErrorContext(ctx, "ExecContext called on a nil prepared query")
		return nil, ErrNilStmt
	}
	return query.prepared.ExecContext(ctx, data...)
}

func (query *QueryStmt[T]) Exec(data ...any) (sql.Result, error) {
	return query.ExecContext(context.Background(), data...)
}

func (query *QueryStmt[T]) QueryContext(ctx context.Context, data ...any) (results []T, err error) {
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

func (query *QueryStmt[T]) Query(data ...any) (results []T, err error) {
	return query.QueryContext(context.Background(), data...)
}

// parseTQLTag parses the tql struct tag options
func parseTQLTag(field reflect.StructField) (results struct {
	omit  string
	field string
}) {
	matches := tagRegex.FindAllStringSubmatch(field.Tag.Get("tql"), -1)
	results.field = field.Name
	for _, match := range matches {
		if match[2] != "" {
			switch strings.TrimSpace(match[1]) {
			case "-":
				results.omit = "true"
			case "omit":
				results.omit = strings.TrimSpace(match[2])
			}
		} else {
			results.field = strings.TrimSpace(match[0])
		}
	}
	return results
}

func matchesContainsWords(matches [][]string, words ...string) bool {
	for _, match := range matches {
		if containsWords(match[1], words...) {
			return true
		}
	}
	return false
}

func containsWords(source string, words ...string) bool {
	for _, word := range words {
		if regexp.MustCompile(`\b` + word).MatchString(source) {
			return true
		}
	}
	return false
}

// iterStructFields returns an iterator over the fields of a struct type
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
