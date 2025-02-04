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
	Id int `tql:"id"`
}

func mock(t testing.TB) *sql.DB {
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
	Id        int             `tql:"id"`
	Name      *sql.NullString `tql:"name"`
	UUID      *sql.NullString `tql:"uuid"`
	CreatedAt *time.Time      `tql:"createdAt"`
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
	queryStmt, err := Prepare(query, db)
	if err != nil {
		t.Fatal(err)
	}
	results, err := queryStmt.Query(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].User.Id != 1 {
		t.Fatal("expected id 1, got", results[0].User.Id)
	}
	if results[0].User.Name.String != "John Doe" {
		t.Fatal("expected name John Doe, got", results[0].User.Name.String)
	}
}

func TestSimpleWithSingleTable(t *testing.T) {
	type Results struct {
		Id        int       `tql:"id"`
		Name      string    `tql:"name"`
		CreatedAt time.Time `tql:"createdAt"`
	}
	db := mock(t)
	query, err := New[Results](`SELECT User.id, User.name, User.createdAt FROM User where User.id = ?`)
	if err != nil {
		t.Fatal(err)
	}
	queryStmt, err := Prepare(query, db)
	if err != nil {
		t.Fatal(err)
	}
	results, err := queryStmt.Query(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].Id != 1 {
		t.Fatal("expected id 1, got", results[0].Id)
	}
	if results[0].Name != "John Doe" {
		t.Fatal("expected name John Doe, got", results[0].Name)
	}
}

func TestPrepareWithNilQuery(t *testing.T) {
	db := (*sql.DB)(nil)
	stmt, err := db.Prepare(`SELECT * FROM User`)
	if err != nil {
		t.Fatal(err)
	}
	stmt.Close()
}

func TestSimpleWithSingleTableAndAliasField(t *testing.T) {
	type Results struct {
		UserId    int       `tql:"userId"`
		Name      string    `tql:"name"`
		CreatedAt time.Time `tql:"createdAt"`
	}
	db := mock(t)
	query, err := New[Results](`SELECT User.id as userId, User.name, User.createdAt FROM User where User.id = ?`)
	if err != nil {
		t.Fatal(err)
	}
	queryStmt, err := Prepare(query, db)
	if err != nil {
		t.Fatal(err)
	}
	results, err := queryStmt.Query(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].UserId != 1 {
		t.Fatal("expected id 1, got", results[0].UserId)
	}
	if results[0].Name != "John Doe" {
		t.Fatal("expected name John Doe, got", results[0].Name)
	}
}

func TestSimpleWithSingleTableWithName(t *testing.T) {
	db := mock(t)
	query, err := New[User](`SELECT User.id, User.name, User.createdAt FROM User where User.id = ?`)
	if err != nil {
		t.Fatal(err)
	}
	queryStmt, err := Prepare(query, db)
	if err != nil {
		t.Fatal(err)
	}
	results, err := queryStmt.Query(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].Id != 1 {
		t.Fatal("expected id 1, got", results[0].Id)
	}
	if results[0].Name.String != "John Doe" {
		t.Fatal("expected name John Doe, got", results[0].Name)
	}
}

func TestSimpleWithSingleTableWithNameAndAlias(t *testing.T) {
	db := mock(t)
	query, err := New[User](`SELECT User.id, User.name, User.createdAt FROM User where User.id = {{ named "id" .Id}}`)
	if err != nil {
		t.Fatal(err)
	}
	queryStmt, err := Prepare(query, db, Params{"Id": 1})
	if err != nil {
		t.Fatal(err)
	}
	results, err := queryStmt.Query()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].Id != 1 {
		t.Fatal("expected id 1, got", results[0].Id)
	}
	if results[0].Name.String != "John Doe" {
		t.Fatal("expected name John Doe, got", results[0].Name)
	}
}

func TestWithOmitField(t *testing.T) {
	db := mock(t)
	type Results struct {
		User struct {
			Id   string  `tql:"id"`
			Name *string `tql:"omit"`
		}
	}
	query, err := New[Results](`SELECT User.id, User.name FROM User`)
	if err != nil {
		t.Fatal(err)
	}
	queryStmt, err := Prepare(query, db)
	if err != nil {
		t.Fatal(err)
	}
	log.Info("queryStmt", "queryStmt", queryStmt.SQL)
	results, err := queryStmt.Query()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].User.Id != "1" {
		t.Fatal("expected id 1, got", results[0].User.Id)
	}
	if results[0].User.Name != nil {
		t.Fatal("expected name to be empty, got", results[0].User.Name)
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
	nilDb := (*sql.DB)(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(query, nilDb); !errors.Is(err, ErrPreparingQuery) {
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
	results, err := Query(query, db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].User.Id != 1 {
		t.Fatal("expected id 1, got", results[0].User.Id)
	}
	if results[0].Account.Id != 2 {
		t.Fatal("expected id 2, got", results[0].Account.Id)
	}
}

func TestNestedSelect(t *testing.T) {
	db := mock(t)
	type Results struct {
		User    User
		Account Account
	}
	type Query struct {
		Account Account
		User    User
	}
	query, err := New[Results](`SELECT User.*, Account.id FROM Account INNER JOIN (SELECT User.id,  User.createdAt FROM User where User.id = ?) AS User ON User.id = Account.userId`)
	if err != nil {
		t.Fatal(err)
	}

	stmt, err := Prepare(query, db, Params{"User": Params{"Id": 1}, "Account": Account{Id: 2}})
	if err != nil {
		t.Fatal(err)
	}
	results, err := stmt.Query(1)
	if err != nil {
		t.Fatal(err)
	}
	log.Info("results", "results", results)
}

func TestNestedSelectWithAlias(t *testing.T) {
	db := mock(t)
	type Results struct {
		User struct {
			UserId int `tql:"userId"`
		}
		Account Account
	}
	type Query struct {
		Account Account
		User    User
	}
	query, err := New[Results](`SELECT User.*, Account.id FROM Account INNER JOIN (SELECT User.id as userId,  User.createdAt FROM User where User.id = ?) AS User ON User.userId = Account.userId`)
	if err != nil {
		t.Fatal(err)
	}

	stmt, err := Prepare(query, db, Params{"User": Params{"Id": 1}, "Account": Account{Id: 2}})
	if err != nil {
		t.Fatal(err)
	}
	results, err := stmt.Query(1)
	if err != nil {
		t.Fatal(err)
	}
	log.Info("results", "results", results)
}
func TestWithTemplate(t *testing.T) {
	db := mock(t)
	type Results struct {
		User User `tql:"omit=createdAt"`
	}
	query, err := New[User](`SELECT uuid, name FROM User WHERE User.createdAt > '{{ .createdAt }}'`)
	if err != nil {
		t.Fatal(err)
	}

	queryStmt, err := Prepare(query, db, Params{"createdAt": time.Now().Format("2006-01-02 15:04:05")})
	if err != nil {
		t.Fatal(err)
	}
	results, err := queryStmt.Query()
	slog.Info("results", "results", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 0 results, got", len(results))
	}
}

func TestWithConditionalTable(t *testing.T) {
	db := mock(t)
	type Results struct {
		User    User
		Account Account
	}
	query, err := New[Results](`SELECT {{ .Table }}.id FROM {{ .Table }} WHERE {{ .Table }}.id = ?`)
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := Prepare(query, db, Params{"Table": "User"})
	if err != nil {
		t.Fatal(err)
	}
	results, err := stmt.Query(1)
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("results", "results", results)
}

func TestWithNilQuery(t *testing.T) {
	db := mock(t)
	var nilQuery *QueryTemplate[any]
	if _, err := Prepare(nilQuery, db, Params{"createdAt": time.Now().Format("2006-01-02 15:04:05")}); !errors.Is(err, ErrPreparingQuery) {
		t.Fatal(err)
	}
	if _, err := Query(nilQuery, db); !errors.Is(err, ErrExecutingQuery) {
		t.Fatal(err)
	}
}

func TestWithNilTemplate(t *testing.T) {
	db := mock(t)
	queryWithNilTemplate := &QueryTemplate[any]{}
	if _, err := Prepare(queryWithNilTemplate, db); !errors.Is(err, ErrNilTemplate) {
		t.Fatal(err)
	}
}

func TestWithFunctions(t *testing.T) {
	db := mock(t)
	type Results struct {
		User User `tql:"user;omit=createdAt"`
	}
	query, err := New[Results](`INSERT INTO User (name, id, uuid) VALUES (?, ?, '{{ uuid }}')`, Functions{"uuid": func() string { return "123" }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(query, db); err != nil {
		t.Fatal(err)
	}
	if _, err := Exec(query, db, "Billy Joel", 2); err != nil {
		t.Fatal(err)
	}
}

func TestComplex(t *testing.T) {
	db := mock(t)
	type Results struct {
		User User `tql:"omit=createdAt"`
	}
	// templates are only rendered during the prepare to prevent SQL injections use
	query, err := New[Results](`SELECT {{ .Select }} FROM User {{ if .Where}} WHERE {{ .Where }} {{end}}`)
	if err != nil {
		t.Fatal(err)
	}
	queryStmt, err := Prepare(query, db, Params{"Select": "User.id, User.name", "Where": "User.id = 1"})
	if err != nil {
		t.Fatal(err)
	}
	results, err := queryStmt.Query()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].User.Id != 1 {
		slog.Info("results", "results", results)
		t.Fatal("expected id 1, got", results[0].User.Id)
	}
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
	queryStmt, err := Prepare(query, db)
	if err != nil {
		t.Fatal(err)
	}
	results, err := queryStmt.Query()
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

func TestTopLevelSelectAll(t *testing.T) {
	db := mock(t)
	query, err := New[User](`SELECT * FROM User`)
	if err != nil {
		t.Fatal(err)
	}
	queryStmt, err := Prepare(query, db)
	if err != nil {
		t.Fatal(err)
	}
	results, err := queryStmt.Query()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].Id != 1 {
		t.Fatal("expected id 1, got", results[0].Id)
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
	results, err := Query(query, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].User.Id != 1 {
		t.Fatal("expected id 1, got", results[0].User.Id)
	}
	if results[0].Account.Id != 2 {
		t.Fatal("expected id 2, got", results[0].Account.Id)
	}
}

func TestSelectAllFromTablWithOmit(t *testing.T) {
	db := mock(t)
	type Results struct {
		User    User `tql:"omit=createdAt"`
		Account Account
	}
	query, err := New[Results](`SELECT User.*, Account.id FROM User LEFT JOIN Account ON User.id = Account.userId`)
	if err != nil {
		t.Fatal(err)
	}
	results, err := Query(query, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result, got", len(results))
	}
	if results[0].User.Id != 1 {
		t.Fatal("expected id 1, got", results[0].User.Id)
	}
	if results[0].Account.Id != 2 {
		t.Fatal("expected id 2, got", results[0].Account.Id)
	}
}

func TestWithTransaction(t *testing.T) {
	db := mock(t)
	tx, err := db.Begin()
	defer tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	type Results struct {
		User User
	}
	query, err := New[Results](`SELECT User.id, User.name, User.createdAt FROM User where User.id = ?`)
	if err != nil {
		t.Fatal(err)
	}
	results, err := Query(query, tx, 1)
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

func BenchmarkTQLCreation(b *testing.B) {
	type Results struct {
		User User
	}
	for i := 0; i < b.N; i++ {
		_, err := New[Results](`SELECT User.id, User.name, User.createdAt FROM User where User.id = ?`)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnprepared(b *testing.B) {
	db := mock(b)
	type Results struct {
		User User
	}
	b.Run("Native", func(b *testing.B) {
		row := db.QueryRow(`SELECT id, name, createdAt FROM User where id = ?`, 1)
		var user User
		if err := row.Scan(&user.Id, &user.Name, &user.CreatedAt); err != nil {
			b.Fatal(err)
		}
	})
	b.Run("TQL", func(b *testing.B) {
		query := Must[Results](`SELECT User.id, User.name, User.createdAt FROM User where User.id = ?`)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			stmt, err := Prepare(query, db)
			if err != nil {
				b.Fatal(err)
			}
			_, err = stmt.Query(1)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkPrepared(b *testing.B) {
	db := mock(b)
	defer db.Close()

	// Native SQL benchmark
	b.Run("Native", func(b *testing.B) {
		stmt, err := db.Prepare(`SELECT User.id, User.name, User.createdAt FROM User WHERE User.id = ?`)
		if err != nil {
			b.Fatal(err)
		}
		defer stmt.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var id int
			var name sql.NullString
			var createdAt time.Time
			if err := stmt.QueryRow(1).Scan(&id, &name, &createdAt); err != nil {
				b.Fatal(err)
			}
		}
	})

	// TQL benchmark
	b.Run("TQL", func(b *testing.B) {
		type Results struct {
			User User
		}
		query, err := New[Results](`SELECT User.id, User.name, User.createdAt FROM User WHERE User.id = ?`)
		if err != nil {
			b.Fatal(err)
		}
		prepared, err := Prepare(query, db)
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := prepared.Query(1); err != nil {
				b.Fatal(err)
			}
		}
	})
}
