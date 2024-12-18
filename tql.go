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
	"os"
	"reflect"
	"regexp"
	"strings"
	"text/template"
)

var (
	// log is the package logger
	log = slog.New(slog.NewJSONHandler(os.Stdout, nil)).WithGroup("tql")

	// tagRegex matches key=value pairs in struct tags
	tagRegex = regexp.MustCompile(`(\w+)=([^;]+)`)

	// selectAllRegex matches SELECT statements to parse column selection
	selectAllRegex = regexp.MustCompile(`(?is)^\s*SELECT\s+(.*?)\s+FROM\b`)
)

// FuncMap is an alias for template.FuncMap to provide custom template functions
type FuncMap template.FuncMap

// Query is the interface for executing SQL queries with type safety
type TQ[T any] interface {
	Execute(db *sql.DB, data ...any) ([]T, error)
	Prepare(db *sql.DB, data ...any) (*TQ[T], error)
}

type DbOrTx interface {
	*sql.DB | *sql.Tx
}

// QueryTemplate implements the Query interface with template and statement preparation
type QueryTemplate[T any] struct {
	template *template.Template
	stmt     *sql.Stmt

	indices [][]int
}

var (
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

// defaultFuncs contains the default template functions
var defaultFuncs = FuncMap{}

// New creates a new query with default template functions
func New[S any](sqlTemplate string, maybeFuncs ...FuncMap) (*QueryTemplate[S], error) {
	funcs := defaultFuncs
	if len(maybeFuncs) > 0 {
		funcs = maybeFuncs[0]
	}
	var s S
	if reflect.ValueOf(s).Kind() != reflect.Struct {
		log.Error("a struct is required", "received", s)
		return nil, ErrInvalidType
	}
	tmpl, err := template.New("sql").Funcs(template.FuncMap(funcs)).Parse(sqlTemplate)
	if err != nil {
		log.Error("failed to create query with functions", "error", err)
		return nil, errors.Join(ErrParsingTemplate, err)
	}
	return &QueryTemplate[S]{template: tmpl}, nil
}

// Must creates a new query and panics if an error occurs
func Must[S any](sqlTemplate string) *QueryTemplate[S] {
	q, err := New[S](sqlTemplate)
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
		log.Error("Execute called on a nil query", "error", ErrNilQuery)
		return results, errors.Join(ErrExecutingQuery, ErrNilQuery)
	}
	var err error
	// this query hasn't been prepared yet
	if query.stmt == nil {
		// prepare the query
		if query, err = PrepareContext(query, ctx, txOrDb); err != nil {
			return results, errors.Join(ErrExecutingQuery, err)
		}

	}
	var scanDest T
	scanDestValue := reflect.ValueOf(&scanDest).Elem()
	fields := []any{}
	for _, fieldIndex := range query.indices {
		field := scanDestValue.FieldByIndex(fieldIndex)
		fields = append(fields, field.Addr().Interface())
	}

	rows, err := query.stmt.QueryContext(ctx, data...)
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

func ExecContext[T any, Q DbOrTx](query *QueryTemplate[T], ctx context.Context, db Q, data ...any) (sql.Result, error) {
	if query == nil {
		log.Error("Execute called on a nil query", "error", ErrNilQuery)
		return nil, errors.Join(ErrExecutingQuery, ErrNilQuery)
	}
	if query.stmt == nil {
		var err error
		if query, err = PrepareContext(query, ctx, db); err != nil {
			return nil, errors.Join(ErrExecutingQuery, err)
		}
	}
	return query.stmt.ExecContext(ctx, data...)
}

func Exec[T any, Q DbOrTx](query *QueryTemplate[T], db Q, data ...any) (sql.Result, error) {
	return ExecContext(query, context.Background(), db, data...)
}

func PrepareContext[T any, Q DbOrTx](query *QueryTemplate[T], ctx context.Context, txOrDb Q, data ...any) (*QueryTemplate[T], error) {
	// make sure the query is not nil
	if query == nil {
		log.Error("Prepare called on a nil query")
		return nil, errors.Join(ErrPreparingQuery, ErrNilQuery)
	}
	if query.template == nil {
		// this should never happen but just in case we will check it anyway
		log.Error("Prepare called with a nil template")
		return query, errors.Join(ErrPreparingQuery, ErrNilTemplate)
	}
	if txOrDb == nil {
		log.Error("Prepare called with a nil tx or db")
		return nil, errors.Join(ErrPreparingQuery, ErrPreparingQuery)
	}
	var buf bytes.Buffer
	templateData := any(nil)
	if len(data) > 0 {
		templateData = data[0]
	}
	if err := query.template.Execute(&buf, templateData); err != nil {
		log.Error("error executing template", "error", err)
		return nil, errors.Join(ErrPreparingQuery, err)
	}
	parsedSQL, err := query.Parse(buf.String())
	if err != nil {
		log.Error("Error parsing sql template", "error", err)
		return query, errors.Join(ErrPreparingQuery, err)
	}
	switch db := any(txOrDb).(type) {
	case *sql.DB:
		query.stmt, err = db.PrepareContext(ctx, parsedSQL)
	case *sql.Tx:
		query.stmt, err = db.PrepareContext(ctx, parsedSQL)
	default:
		log.Error("Prepare called with an invalid queryable", "error", ErrPreparingQuery)
		return nil, errors.Join(ErrPreparingQuery, ErrInvalidQueryable)
	}
	if err != nil {
		log.Error("failed to prepare query", "error", err)
		return nil, errors.Join(ErrPreparingQuery, err)
	}
	// register a function to cleanup the query when the context is done
	context.AfterFunc(ctx, func() {
		query.Close()
	})
	return query, nil
}

func Prepare[T any, Q DbOrTx](tqlQuery *QueryTemplate[T], db Q, data ...any) (*QueryTemplate[T], error) {
	return PrepareContext(tqlQuery, context.Background(), db, data...)
}

// Parse parses the SQL template and extracts field information for scanning
func (query *QueryTemplate[T]) Parse(sql string) (string, error) {
	var tmp T
	tableOrTables := reflect.ValueOf(tmp).Type()
	selectedFields := []string{}
	match := selectAllRegex.FindStringSubmatch(sql)
	// parse the sql template to see if we are selecting all fields
	if match != nil {
		selectAll := strings.TrimSpace(match[1]) == "*"
		if !selectAll {
			selectedFields = strings.Split(match[1], ",")
		}
		// iterate over the fields of the struct to get the indices of the fields that we are selecting
		for tableOrField := range iterStructFields(tableOrTables) {
			tableName := ""
			tableOrFieldType := tableOrField.Type
			indices := []int{}
			if tableOrFieldType.Kind() != reflect.Struct {
				// this means that this is a single table query
				tableOrFieldType = tableOrTables
			} else {
				tableName = parseFieldName(tableOrField)
				indices = append(indices, tableOrField.Index[0])
			}
			// check if we are selecting all fields from the table with X.*
			selectAllFromTable := selectAll || containsWords(match[1], tableName+`\.\*`)
			tags := parseTQLTag(tableOrField)
			for field := range iterStructFields(tableOrFieldType) {
				fieldName := parseFieldName(field)
				qualifiedName := tableName + "." + fieldName
				if strings.Contains(tags.omit, fieldName) || strings.Contains(tags.omit, qualifiedName) {
					continue
				}
				if selectAll {
					selectedFields = append(selectedFields, qualifiedName)
				} else if !selectAllFromTable && !containsWords(match[1], tableName+`\.`+fieldName, fieldName) {
					log.Debug("column not found in the sql statement", "column", qualifiedName, "sql", sql)
					continue
				}
				query.indices = append(query.indices, append(indices[:], field.Index...))
			}
			if tableOrFieldType == tableOrTables {
				// make sure we break out of this loop if this is a single table query
				break
			}
		}
		sql = strings.Replace(sql, match[1], strings.Join(selectedFields, ", "), 1)
	}
	_, err := query.template.Parse(sql)
	if err != nil {
		log.Error("failed to create query with functions", "error", err)
		return "", errors.Join(ErrParsingQuery, err)
	}
	return sql, nil
}

func (query *QueryTemplate[T]) Close() error {
	if query == nil {
		log.Error("Close called on a nil query")
		return ErrNilQuery
	}
	if query.stmt != nil {
		query.stmt.Close()
		query.stmt = nil
	}
	return nil
}

// parseFieldName extracts the field name from struct field tags
func parseFieldName(field reflect.StructField) string {
	fieldName := field.Tag.Get("db")
	if fieldName == "" {
		fieldName = field.Name[:1] + field.Name[1:]
	}
	return fieldName
}

// parseTQLTag parses the tql struct tag options
func parseTQLTag(field reflect.StructField) (results struct{ omit string }) {
	matches := tagRegex.FindAllStringSubmatch(field.Tag.Get("tql"), -1)
	for _, match := range matches {
		switch strings.TrimSpace(match[1]) {
		case "omit":
			results.omit = strings.TrimSpace(match[2])
		}
	}
	return results
}

func containsWords(source string, words ...string) bool {
	for _, word := range words {
		log.Debug("containsWords", "source", source, "word", word)
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
