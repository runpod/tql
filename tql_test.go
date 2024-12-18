package tql

import (
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type Account struct {
	Id int `db:"id"`
}

func mock(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE User (
			id INTEGER PRIMARY KEY,
			name TEXT,
			createdAt DATETIME DEFAULT CURRENT_TIMESTAMP,
			uuid TEXT
		);
		CREATE TABLE Account (
			id INTEGER PRIMARY KEY,
			userId INTEGER,
			FOREIGN KEY (userId) REFERENCES User(id)
		);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO User (id, name) VALUES (1, 'John Doe')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO Account (id, userId) VALUES (2, 1)"); err != nil {
		t.Fatal(err)
	}
	return db
}

type User struct {
	Id        int             `db:"id"`
	Name      *sql.NullString `db:"name"`
	UUID      *sql.NullString `db:"uuid"`
	CreatedAt *time.Time      `db:"createdAt"`
}

func TestSimple(t *testing.T) {
	type Results struct {
		User
	}
	db := mock(t)
	query, err := New[Results](`SELECT User.id, User.name, User.createdAt FROM User where User.id = ?`)
	if err != nil {
		t.Fatal(err)
	}
	results, err := query.Execute(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	slog.Info("results", "results", results)
	if results[0].User.Id != 1 {
		t.Fatal("expected id 1, got", results[0].User.Id)
	}
	if results[0].User.Name.String != "John Doe" {
		t.Fatal("expected name John Doe, got", results[0].User.Name.String)
	}
}

func TestWithMissingFunction(t *testing.T) {
	if _, err := New[any](`SELECT {{ uuid }} FROM User`); !errors.Is(err, ErrInvalidType) {
		t.Fatal("expected error to be ErrParsingQuery, got", err)
	}
}

func TestWithNilDB(t *testing.T) {
	type UserAccount struct {
		User
		Account
	}
	query, err := New[UserAccount](`SELECT * FROM User WHERE User.id =`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := query.Prepare(nil); !errors.Is(err, ErrPreparingQuery) {
		t.Fatal("expected error to be ErrPreparingQuery, got", err)
	}
}

func TestJoin(t *testing.T) {
	db := mock(t)
	type UserAccount struct {
		User
		Account
	}
	query, err := New[UserAccount](`SELECT User.id, User.name, Account.id FROM User JOIN Account ON User.id = Account.userId where User.id = ?`)
	if err != nil {
		t.Fatal(err)
	}
	results, err := query.Execute(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("user", "results", results)
}

func TestWithTemplate(t *testing.T) {
	db := mock(t)
	type Results struct {
		User User `tql:"omit=createdAt"`
	}
	query, err := New[Results](`SELECT User.uuid, User.name FROM User WHERE User.createdAt > '{{ . }}'`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := query.Prepare(db, time.Now().Format("2006-01-02 15:04:05")); err != nil {
		t.Fatal(err)
	}
	results, err := query.Execute(db)
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("results", "results", results)
}

func TestWithNilQuery(t *testing.T) {
	db := mock(t)
	var nilQuery *query[any]
	if _, err := nilQuery.Prepare(db, time.Now().Format("2006-01-02 15:04:05")); !errors.Is(err, ErrPreparingQuery) {
		t.Fatal(err)
	}
	if _, err := nilQuery.Execute(db); !errors.Is(err, ErrExecutingQuery) {
		t.Fatal(err)
	}
}

func TestWithNilTemplate(t *testing.T) {
	db := mock(t)
	queryWithNilTemplate := query[any]{}
	if _, err := queryWithNilTemplate.Prepare(db); !errors.Is(err, ErrNilTemplate) {
		t.Fatal(err)
	}
}

func TestWithFunctions(t *testing.T) {
	db := mock(t)
	type Results struct {
		User User `tql:"omit=createdAt" db:"user"`
	}
	query, err := WithFuncs[Results](Funcs{"uuid": func() string { return "123" }}, `INSERT INTO User (name, id, uuid) VALUES (?, ?, '{{ uuid }}')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := query.Prepare(db); err != nil {
		t.Fatal(err)
	}
	results, err := query.Execute(db, "Billy Joel", 2)
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("results", "results", results)
}

func TestComplex(t *testing.T) {
	db := mock(t)
	type Results struct {
		User User `tql:"omit=createdAt"`
	}
	type Params struct {
		Select string
		Where  string
	}
	// templates are only rendered during the prepare to prevent SQL injections use
	query, err := New[Results](`SELECT {{ .Select }} FROM User {{ with not .Where}} WHERE {{ .Where }} {{end}}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := query.Prepare(db, Params{Select: "User.uuid, User.name", Where: "User.id = 1"}); err != nil {
		t.Fatal(err)
	}
	results, err := query.Execute(db)
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("results", "results", results)
}

func TestSelectAll(t *testing.T) {
	db := mock(t)
	type Results struct {
		User User
	}
	query, err := New[Results](`SELECT * FROM User`)
	if err != nil {
		t.Fatal(err)
	}
	results, err := query.Execute(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].User.Id != 1 {
		t.Fatal("expected id 1, got", results[0].User.Id)
	}
}

func TestSelectAllFromTable(t *testing.T) {
	db := mock(t)
	type Results struct {
		User    User
		Account Account
	}
	query, err := New[Results](`SELECT User.*, Account.id FROM User LEFT JOIN Account ON User.id = Account.userId`)
	if err != nil {
		t.Fatal(err)
	}
	results, err := query.Execute(db)
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("results", "results", results)
}

func TestStructParams(t *testing.T) {
	db := mock(t)
	type Results struct {
		User User
	}
	query, err := New[Results](`SELECT * FROM User WHERE User.id = ?`)
	if err != nil {
		t.Fatal(err)
	}
	results, err := query.Execute(db, User{Id: 1})
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("results", "results", results)
}
