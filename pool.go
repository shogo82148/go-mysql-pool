// Package mysqlpool simplifies the creation and management of MySQL database pools for testing and development purposes.
package mysqlpool

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/go-sql-driver/mysql"
)

// ErrClosed is returned when the pool is closed.
var ErrClosed = errors.New("mysqlpool: pool is closed")

// Pool is a pool of MySQL databases.
type Pool struct {
	// MySQLConfig is the configuration for the MySQL connection.
	MySQLConfig *mysql.Config

	// DDL is Data Definition.
	DDL string

	mu      sync.Mutex
	closed  bool
	adminDB *sql.DB
	freeDB  []*sql.DB
	allDB   []*sql.DB
}

// Get returns a database from the pool. If the pool is empty, a new database is created.
func (p *Pool) Get(ctx context.Context) (*sql.DB, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrClosed
	}
	if len(p.freeDB) > 0 {
		l := len(p.freeDB)
		db := p.freeDB[l-1]
		p.freeDB = p.freeDB[:l-1]
		p.mu.Unlock()
		if err := resetDB(ctx, db); err != nil {
			return nil, err
		}
		return db, nil
	}
	p.mu.Unlock()

	db, err := p.new(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.allDB = append(p.allDB, db)
	p.mu.Unlock()
	return db, nil
}

// Put returns a database to the pool.
func (p *Pool) Put(db *sql.DB) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.freeDB = append(p.freeDB, db)
}

// Close drops all databases in the pool and closes all connections.
func (p *Pool) Close() error {
	var errs []error

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true

	ctx := context.Background()
	for _, db := range p.allDB {
		if err := dropDB(ctx, db); err != nil {
			errs = append(errs, err)
		}
		if err := db.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if p.adminDB != nil {
		if err := p.adminDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (p *Pool) new(ctx context.Context) (*sql.DB, error) {
	dbName, err := p.createDB(ctx)
	if err != nil {
		return nil, err
	}

	if err := p.initDB(ctx, dbName); err != nil {
		return nil, err
	}

	// Open a new connection to the database.
	cfg := p.MySQLConfig.Clone()
	cfg.DBName = dbName
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(conn), nil
}

// createDB creates a new database and returns the name of the database created.
func (p *Pool) createDB(ctx context.Context) (string, error) {
	adminDB, err := p.getAdminDB()
	if err != nil {
		return "", err
	}

	var buf [8]byte
	_, err = rand.Read(buf[:])
	if err != nil {
		return "", err
	}
	dbName := fmt.Sprintf("test_%x", buf)
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s`", dbName)); err != nil {
		return "", err
	}
	return dbName, nil
}

func (p *Pool) initDB(ctx context.Context, dbName string) error {
	// Open a new connection to the database.
	cfg := p.MySQLConfig.Clone()
	cfg.DBName = dbName
	cfg.MultiStatements = true
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		return err
	}
	db := sql.OpenDB(conn)
	defer db.Close()

	// Execute the DDL.
	_, err = db.ExecContext(ctx, p.DDL)
	if err != nil {
		return err
	}
	return nil
}

// resetDB truncates all tables in the database.
func resetDB(ctx context.Context, db *sql.DB) (err error) {
	tables, err := listNonEmptyTables(ctx, db)
	if err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0")
	if err != nil {
		return err
	}
	defer func() {
		if _, e := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1"); e != nil && err == nil {
			err = e
		}
	}()

	for _, table := range tables {
		if _, err := conn.ExecContext(ctx, "TRUNCATE TABLE `"+table+"`"); err != nil {
			return err
		}
	}

	return nil
}

func listNonEmptyTables(ctx context.Context, db *sql.DB) (tables []string, err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// get the current value of information_schema_stats_expiry
	row := conn.QueryRowContext(ctx, "SELECT @@information_schema_stats_expiry")
	var expiry int
	if err := row.Scan(&expiry); err != nil {
		return nil, err
	}

	// disable the cache for INFORMATION_SCHEMA TABLES
	_, err = conn.ExecContext(ctx, "SET information_schema_stats_expiry = 0")
	if err != nil {
		return nil, err
	}
	defer func() {
		// restore information_schema_stats_expiry
		if _, e := conn.ExecContext(ctx, "SET information_schema_stats_expiry = ?", expiry); e != nil && err == nil {
			err = e
		}
	}()

	rows, err := conn.QueryContext(
		ctx,
		"SELECT `table_name` FROM `information_schema`.`tables` "+
			"WHERE `table_schema` = DATABASE() AND ("+
			"  `table_rows` > 0 OR `auto_increment` > 1"+
			")",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	return tables, nil
}

func dropDB(ctx context.Context, db *sql.DB) error {
	row := db.QueryRowContext(ctx, "SELECT DATABASE()")
	var dbName string
	if err := row.Scan(&dbName); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "DROP DATABASE `"+dbName+"`"); err != nil {
		return err
	}
	return nil
}

func (p *Pool) getAdminDB() (*sql.DB, error) {
	// If adminDD is already created, return it.
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.adminDB != nil {
		db := p.adminDB
		return db, nil
	}

	// Create a new adminDB.
	cfg := p.MySQLConfig.Clone()
	cfg.DBName = ""
	cfg.MultiStatements = true
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(conn)
	p.adminDB = db
	return db, nil
}
