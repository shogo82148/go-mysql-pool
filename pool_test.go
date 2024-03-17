package mysqlpool

import (
	"context"
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

func TestPool(t *testing.T) {
	cfg := newMySQLConfig(t)
	p := &Pool{
		MySQLConfig: cfg,
		DDL:         "CREATE TABLE foo (id INT PRIMARY KEY)",
	}
	db, err := p.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Put(db)
}
