# TQL - Type-safe Zero Dependency Query Language

TQL is an internal SQL templating engine designed to provide type safety when scanning database results while staying as close as possible to raw SQL syntax. It aims to eliminate runtime errors from mismatched types or missing columns while preserving the flexibility and power of raw SQL.

## Key Features

- Uses Go's standard `text/template` package for SQL templating with compile-time validation
- Type-safe scanning of query results into Go structs with automatic field mapping
- Minimal abstraction over raw SQL - write queries naturally with full SQL functionality
- Support for complex joins and nested result structures through struct embedding
- Prepared statement caching for optimal performance
- Zero external dependencies beyond the Go standard library
- Compile-time validation of struct field tags and query parameters
- Automatic handling of NULL values through pointer types

## Usage Example

```go
// Define your result structure
type User struct {
    ID        int             `db:"id"`
    Name      *sql.NullString `db:"name"`
    UUID      *sql.NullString `db:"uuid"`
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
db, _ := sql.Open("sqlite", "dsn")
results, err := query.Execute(db, 1)
```

## Advanced Features

### SELECT * Support
TQL supports using `SELECT *` and will automatically map all columns from the specified table:

```go
query, err := tql.New[Results](`SELECT * FROM User`)
```

It also supports selecting all columns from specific tables in JOINs:

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

### Struct Parameters
You can pass structs directly as query parameters:

```go
query, err := tql.New[Results](`SELECT * FROM User WHERE User.id = ?`)
results, err := query.Execute(db, User{Id: 1})
```

### Template Parameters
TQL supports complex templating with struct parameters:

```go
type Params struct {
    Select string
    Where  string
}

query, err := tql.New[Results](`
    SELECT {{ .Select }} 
    FROM User 
    {{ with not .Where}} WHERE {{ .Where }} {{end}}
`)

// Prepare with parameters
query.Prepare(db, Params{
    Select: "User.uuid, User.name",
    Where: "User.id = 1",
})
```

### NULL Handling
TQL provides flexible NULL handling through pointer types and sql.Null* types:

```go
type User struct {
    ID        int             `db:"id"`
    Name      *sql.NullString `db:"name"`    // Using sql.NullString
    UUID      *sql.NullString `db:"uuid"`
    CreatedAt *time.Time      `db:"createdAt"` // Using pointer for nullable time
}
```

## Field Mapping

TQL automatically maps database columns to struct fields using the following rules:

1. By default, it uses the struct field name
2. Custom column names can be specified using the `db` tag
3. Nested structs are supported using dot notation (e.g., `User.id`, `Profile.bio`)
4. Fields can be excluded using the `tql` tag with the `omit` parameter

Example:
```go
type User struct {
    ID        int       `db:"id"`
    Name      string    `db:"name"`
    CreatedAt time.Time `db:"createdAt"`
}

type Results struct {
    User User `tql:"omit=createdAt"` // The createdAt field will be excluded
}
```

## Template Functions

You can extend the template functionality using custom functions:

```go
funcs := tql.Funcs{
    "uuid": func() string { 
        return "123" 
    },
}

query, err := tql.WithFuncs[Results](funcs, `
    INSERT INTO User (name, id, uuid) 
    VALUES (?, ?, '{{ uuid }}')
`)
```

## Error Handling

TQL provides detailed error types for common scenarios:

- `ErrNilQuery`: Attempted to execute a nil query
- `ErrNilTemplate`: Template initialization failed
- `ErrPreparingQuery`: Error during query preparation
- `ErrExecutingQuery`: Error during query execution
- `ErrParsingQuery`: Error parsing the SQL template

## Best Practices

1. Use struct embedding to organize related fields
2. Use pointer types for nullable columns
3. Always prepare queries before execution for better performance
4. Use meaningful struct and field names that match your database schema
5. Leverage the `db` tag for custom column mappings

## Limitations

- Currently supports struct types only for result mapping
- Field names must be unique across nested structs
- Template must reference all fields that should be scanned

## Contributing

Contributions are welcome! Please ensure that any pull requests include appropriate tests and documentation.
