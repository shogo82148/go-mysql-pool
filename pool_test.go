package mysqlpool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func newMySQLConfig(t *testing.T) *mysql.Config {
	t.Helper()

	cfg := mysql.NewConfig()
	cfg.User = os.Getenv("MYSQLPOOL_USER")
	cfg.Passwd = os.Getenv("MYSQLPOOL_PASS")
	cfg.Net = "tcp"
	host := os.Getenv("MYSQLPOOL_HOST")
	port := os.Getenv("MYSQLPOOL_PORT")
	if port == "" {
		port = "3306"
	}
	cfg.Addr = net.JoinHostPort(host, port)

	if host == "" {
		t.Skip("MYSQLPOOL_HOST is not set; skipping integration test")
	}
	return cfg
}

func TestPool_CleanupDB(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := newMySQLConfig(t)
	p := &Pool{
		MySQLConfig: cfg,
		DDL:         "CREATE TABLE foo (id INT PRIMARY KEY)",
	}

	// get the database from the pool
	db, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// get the database name
	var dbName string
	row := db.QueryRow("SELECT DATABASE()")
	if err := row.Scan(&dbName); err != nil {
		t.Error(err)
	}
	p.Put(db)

	if err := p.Close(); err != nil {
		t.Fatal(err)
	}

	// check if the database is dropped
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db = sql.OpenDB(conn)
	defer db.Close()

	row = db.QueryRowContext(ctx, fmt.Sprintf("SHOW DATABASES LIKE '%s'", dbName))
	err = row.Scan(&dbName)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected database to be dropped; got %v", err)
	}
}

func TestPool_ResetTables(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := newMySQLConfig(t)
	p := &Pool{
		MySQLConfig: cfg,
		DDL: "CREATE TABLE parent (id INT PRIMARY KEY);" +
			"CREATE TABLE child (id INT PRIMARY KEY, parent_id INT, FOREIGN KEY (parent_id) REFERENCES parent(id));",
	}

	// get the database from the pool
	db, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO parent (id) VALUES (1)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO child (id, parent_id) VALUES (1, 1)")
	if err != nil {
		t.Fatal(err)
	}
	p.Put(db)

	db, err = p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row := db.QueryRow("SELECT COUNT(*) FROM parent")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows; got %d", count)
	}
	p.Put(db)

	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}
