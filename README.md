# TQL - Type-safe Zero Dependency Query Language

TQL is an internal SQL templating engine designed to provide type safety when scanning database results while staying as close as possible to raw SQL syntax. It aims to eliminate runtime errors from mismatched types or missing columns while preserving the flexibility and power of raw SQL.

## Key Features

- Uses Go's standard `text/template` package for SQL templating with compile-time validation
- Type-safe scanning of query results into Go structs with automatic field mapping
- Minimal abstraction over raw SQL - write queries naturally with full SQL functionality
- Support for complex joins and nested result structures through struct embedding
- Supports Prepared Statements
- Zero external dependencies beyond the Go standard library
- Compile-time validation of struct field tags and query parameters
- Automatic handling of NULL values through pointer types
- Support for both *sql.DB and *sql.Tx

## Basic Usage

```go
// Define your result structure
type User struct {
    Id        int             `db:"id"`
    Name      *sql.NullString `db:"name"`
    CreatedAt *time.Time      `db:"createdAt"`
}

type Results struct {
    User User
}

// Create a new query template
query, err := tql.New[Results](`
    SELECT User.id, User.name, User.createdAt 
    FROM User 
    WHERE User.id = ?
`)
if err != nil {
    return err
}

// Execute the query directly
db, _ := sql.Open("sqlite", ":memory:")
results, err := tql.Query(query, db, 1)

// Or prepare explicitly for reuse
prepared, err := tql.Prepare(query, db)
if err != nil {
    return err
}
results, err = prepared.Query(1)
```

## Context Support

TQL provides context-aware variants of its core functions with automatic cleanup:

```go
ctx := context.Background()
prepared, err := tql.PrepareContext(query, ctx, db)
// The prepared statement will be automatically closed when ctx is cancelled
results, err := prepared.QueryContext(ctx, 1)
```

## Transaction Support

TQL works seamlessly with both database connections and transactions:

```go
tx, err := db.Begin()
if err != nil {
    return err
}
defer tx.Rollback()

results, err := tql.Query(query, tx, 1)
```

## Advanced Features

### SELECT * Support
TQL automatically maps all columns when using `SELECT *`:

```go
type Results struct {
    User User
}

query, err := tql.New[Results](`SELECT * FROM User`)
```

It also supports table-specific wildcards in JOINs:

```go
type Results struct {
    User    User
    Account Account
}

query, err := tql.New[Results](`
    SELECT User.*, Account.id 
    FROM User 
    LEFT JOIN Account ON User.id = Account.userId
`)
```

### Template Functions

You can extend the template functionality using custom functions:

```go
funcs := tql.FuncMap{
    "uuid": func() string { 
        return "123" 
    },
}

query, err := tql.New[Results](`
    INSERT INTO User (name, id, uuid) 
    VALUES (?, ?, '{{ uuid }}')
`, funcs)
```

### Field Omission

You can selectively omit fields from being scanned using the `tql` tag:

```go
type Results struct {
    User User `tql:"omit=createdAt"`
}

query, err := tql.New[Results](`SELECT User.* FROM User`)
```

### Error Handling

TQL provides detailed error types that can be checked using `errors.Is()`:

```go
var (
    ErrNilQuery        = errors.New("query is nil")
    ErrNilTemplate     = errors.New("template is nil")
    ErrPreparingQuery  = errors.New("failed to prepare query")
    ErrExecutingQuery  = errors.New("failed to execute query")
    ErrParsingQuery    = errors.New("failed to parse sql template")
    ErrParsingTemplate = errors.New("failed to parse template")
    ErrInvalidType     = errors.New("failed to create query type parameter is invalid")
    ErrInvalidQueryable = errors.New("invalid queryable")
)
```

### Nested SELECT Support

TQL supports nested SELECT statements with template parameters. This is useful for complex queries that need to reference values from the template context:

```go
// Define your nested result structure
type Results struct {
    User    User
    Account Account
}

// Define the parameters for your query
type QueryParams struct {
    Account Account
    User    User
}

// Create a query with a nested SELECT
query, err := tql.New[Results](`
    SELECT User.*, Account.id 
    FROM Account 
    INNER JOIN (
        SELECT User.id, User.createdAt 
        FROM User 
        WHERE User.id = {{ .User.Id }}
    ) AS User ON User.id = Account.userId
`)

// Execute with parameters
stmt, err := tql.Prepare(query, db, QueryParams{
    User: User{Id: 1},
    Account: Account{Id: 2},
})
results, err := stmt.Query()
```

This feature allows you to:
- Use template parameters in subqueries
- Maintain type safety across nested queries
- Build complex queries while keeping the code readable
- Reference template context values in any part of the query

## Performance

TQL is designed to be performant while providing type safety. Here are the benchmark results comparing TQL with native SQL operations:

```bash
goos: darwin
goarch: arm64
cpu: Apple M4 Pro
BenchmarkTQLCreation-14                  1,134,435              1,068 ns/op
BenchmarkUnprepared/Native-14        1,000,000,000              0.02 ns/op
BenchmarkUnprepared/TQL-14                276,362              4,186 ns/op
BenchmarkPrepared/Native-14               300,410              3,898 ns/op
BenchmarkPrepared/TQL-14                  273,488              4,193 ns/op
```

Key observations:
1. Query creation has minimal overhead at ~1µs per operation
2. Prepared statements show similar performance between TQL (~4.2µs) and native SQL (~3.9µs)
3. For unprepared queries, native SQL is significantly faster, suggesting you should use prepared statements with TQL

For optimal performance:
1. Use `Prepare()` for queries that will be executed multiple times
2. Cache prepared statements when possible
3. Consider using context cancellation for automatic cleanup of prepared statements
4. Profile your specific use case to make informed decisions

The small overhead added by TQL is typically justified by the benefits of:
- Compile-time type checking
- Automatic field mapping
- Reduced potential for runtime errors
- Improved maintainability
- Automatic resource cleanup
