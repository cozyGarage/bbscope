package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	// ErrAbortingScopeWipe is returned when an update would wipe all targets from a program.
	ErrAbortingScopeWipe = errors.New("aborting update to prevent wiping out all targets for a program")
	// ErrInvalidDatabaseName is returned when the database name contains invalid characters.
	ErrInvalidDatabaseName = errors.New("invalid database name: must start with letter or underscore, contain only alphanumeric and underscore, max 63 chars")
	// validDBNameRegex matches valid PostgreSQL identifiers
	validDBNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// redactConnectionString redacts the password from a connection string for safe logging
func redactConnectionString(connStr string) string {
	parsed, err := url.Parse(connStr)
	if err != nil {
		return "[invalid connection string]"
	}
	if parsed.User != nil {
		if _, hasPass := parsed.User.Password(); hasPass {
			parsed.User = url.UserPassword(parsed.User.Username(), "****")
		}
	}
	return parsed.String()
}

type DB struct {
	sql *sql.DB
}

// PoolConfig holds database connection pool settings.
type PoolConfig struct {
	MaxOpenConns    int           // Maximum number of open connections (default: 25)
	MaxIdleConns    int           // Maximum number of idle connections (default: 5)
	ConnMaxLifetime time.Duration // Maximum connection lifetime (default: 5 minutes)
	ConnMaxIdleTime time.Duration // Maximum idle time for connections (default: 5 minutes)
}

// DefaultPoolConfig returns sensible defaults for connection pooling.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}

// OpenWithPool opens a database connection with custom pool configuration.
func OpenWithPool(connectionString string, pool PoolConfig) (*DB, error) {
	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection to %s: %w", redactConnectionString(connectionString), err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(pool.MaxOpenConns)
	db.SetMaxIdleConns(pool.MaxIdleConns)
	db.SetConnMaxLifetime(pool.ConnMaxLifetime)
	db.SetConnMaxIdleTime(pool.ConnMaxIdleTime)

	if err := db.Ping(); err != nil {
		// Check if database doesn't exist, try to create it
		if strings.Contains(err.Error(), "does not exist") {
			if createErr := createDatabase(connectionString); createErr != nil {
				return nil, fmt.Errorf("database does not exist and failed to create: %w", createErr)
			}
			// Retry connection after creating database
			if err = db.Ping(); err != nil {
				return nil, fmt.Errorf("failed to connect to %s: %w", redactConnectionString(connectionString), err)
			}
		} else {
			return nil, fmt.Errorf("failed to connect to %s: %w", redactConnectionString(connectionString), err)
		}
	}
	if err := applyMigrations(db); err != nil {
		return nil, fmt.Errorf("migrating schema: %w", err)
	}
	return &DB{sql: db}, nil
}

func Open(connectionString string) (*DB, error) {
	return OpenWithPool(connectionString, DefaultPoolConfig())
}

// createDatabase connects to the default "postgres" database and creates the target database
func createDatabase(connectionString string) error {
	parsed, err := url.Parse(connectionString)
	if err != nil {
		return fmt.Errorf("parsing connection string: %w", err)
	}

	// Extract database name from path (e.g., "/bbscope" -> "bbscope")
	dbName := strings.TrimPrefix(parsed.Path, "/")
	if dbName == "" {
		return errors.New("no database name in connection string")
	}

	// Validate database name to prevent SQL injection
	if err = validateDatabaseName(dbName); err != nil {
		return err
	}

	// Create connection string for the default "postgres" database
	parsed.Path = "/postgres"
	adminConnStr := parsed.String()

	adminDB, err := sql.Open("pgx", adminConnStr)
	if err != nil {
		return fmt.Errorf("connecting to postgres database: %w", err)
	}
	defer adminDB.Close()

	if err = adminDB.Ping(); err != nil {
		return fmt.Errorf("pinging postgres database: %w", err)
	}

	// Create the database using pgx.Identifier for safe quoting
	_, err = adminDB.Exec(fmt.Sprintf(`CREATE DATABASE %s`, pgx.Identifier{dbName}.Sanitize()))
	if err != nil {
		return fmt.Errorf("creating database %s: %w", dbName, err)
	}

	return nil
}

// validateDatabaseName checks that a database name is a valid PostgreSQL identifier
func validateDatabaseName(name string) error {
	if len(name) == 0 || len(name) > 63 {
		return fmt.Errorf("%w: length must be 1-63 characters", ErrInvalidDatabaseName)
	}
	if !validDBNameRegex.MatchString(name) {
		return fmt.Errorf("%w: got %q", ErrInvalidDatabaseName, name)
	}
	return nil
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func identityKey(raw, category string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	category = strings.TrimSpace(strings.ToLower(category))
	return raw + "|" + category
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// escapeLikePattern escapes \, %, and _ so user input is matched literally in LIKE.
func escapeLikePattern(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}
