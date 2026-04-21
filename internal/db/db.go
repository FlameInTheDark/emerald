package db

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

var gooseMu sync.Mutex

type DB struct {
	*sql.DB
}

func New(path string) (*DB, error) {
	journalMode := "WAL"
	if runtime.GOOS == "windows" {
		journalMode = "DELETE"
	}

	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=%s&_foreign_keys=ON", path, journalMode)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{DB: db}, nil
}

func (d *DB) Close() error {
	if d == nil || d.DB == nil {
		return nil
	}

	var errs []error
	if _, err := d.Exec(`PRAGMA wal_checkpoint(TRUNCATE);`); err != nil && !isIgnorableCloseError(err) {
		errs = append(errs, fmt.Errorf("checkpoint sqlite wal: %w", err))
	}
	if _, err := d.Exec(`PRAGMA journal_mode=DELETE;`); err != nil && !isIgnorableCloseError(err) {
		errs = append(errs, fmt.Errorf("switch sqlite journal mode: %w", err))
	}
	if err := d.DB.Close(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func Migrate(d *DB) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(d.DB, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

func isIgnorableCloseError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is closed")
}
