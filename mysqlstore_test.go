package session

import (
	"database/sql"
	"flag"
	_ "github.com/go-sql-driver/mysql"
	"net/http"
	"testing"
	"time"
)

var DSN = flag.String("dsn", "", "Database DSN for MySQL session storage.")

func Test_MySQLStore(t *testing.T) {
	if *DSN == "" {
		t.Log("SQL cacher untested. Please re-run with -dsn=\"go-mysql-driver dsn\"")
		return
	}

	db, err := sql.Open("mysql", *DSN)
	if err != nil {
		t.Errorf("Error opening database: %s", err)
		return
	}

	store, err := NewMySQLStore(db, "session_test", 60*time.Minute)
	if err != nil {
		t.Errorf("failed to create memory store: %s", err)
		return
	}

	sm, err := NewSessionManager(store, "test_session")
	if err != nil {
		t.Errorf("failed to create session manager: %s", err)
		return
	}

	memTest := SessionTestServer{t, sm}
	go http.ListenAndServe(listen, memTest)

	session_test(t)
}
