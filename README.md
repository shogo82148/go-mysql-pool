[![Go Reference](https://pkg.go.dev/badge/github.com/shogo82148/go-mysql-pool.svg)](https://pkg.go.dev/github.com/shogo82148/go-mysql-pool)
[![test](https://github.com/shogo82148/go-mysql-pool/actions/workflows/test.yaml/badge.svg)](https://github.com/shogo82148/go-mysql-pool/actions/workflows/test.yaml)

# go-mysql-pool

Go-MySQL-Pool is a package that simplifies the creation and management of MySQL database pools for testing and development purposes.

## Synopsis

```go
package example_test

import (
    "context"
    "testing"

    "github.com/go-sql-driver/mysql"
    "github.com/shogo82148/go-mysql-pool"
)

var pool *mysqlpool.Pool

func TestMain(m *testing.M) {
    // setup the pool
    cfg := mysql.NewConfig()
    cfg.User = "username"
    cfg.Passwd = "password"
    cfg.Net = "tcp"
    cfg.Addr = "127.0.0.1:3306"
    pool = &mysqlpool.Pool{
        MySQLConfig: cfg,
        DDL:         "CREATE TABLE foo (id INT PRIMARY KEY)",
    }
    defer pool.Close() // cleanup all databases

    m.Run()
}

func TestFooBar(t *testing.T) {
    // get *sql.DB from the pool.
    db, err := pool.Get(context.Background())
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() {
        pool.Put(db)
    })

    // use db for testing.
}
```
