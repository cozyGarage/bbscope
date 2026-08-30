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

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
)

var (
	// ErrAbortingScopeWipe is returned when an update would wipe all targets from a program.
	ErrAbortingScopeWipe = errors.New("aborting update to prevent wiping out all targets for a program")
	// ErrAbortingPartialSync is returned when SyncPlatformPrograms would disable an
	// implausibly large fraction of active programs (likely a partial/failed poll).
	ErrAbortingPartialSync = errors.New("aborting platform sync to prevent mass program disable from a partial poll")
	// ErrInvalidDatabaseName is returned when the database name contains invalid characters.
	ErrInvalidDatabaseName = errors.New("invalid database name: must start with letter or underscore, contain only alphanumeric and underscore, max 63 chars")
	// validDBNameRegex matches valid PostgreSQL identifiers
	validDBNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

const (
	// redactedPlaceholder stands in for secrets in logged connection strings.
	// Note: in URL form url.UserPassword percent-encodes it to "%2A%2A%2A%2A".
	redactedPlaceholder = "****"

	syncPartialDisableMaxRatio = 0.5

	// syncRatioMinActivePrograms is the smallest platform the half-removal ratio
	// is applied to. Below it a single legitimate removal already reaches half.
	syncRatioMinActivePrograms = 3
)

// redactConnectionString redacts the password from a connection string for safe
// logging. pgx accepts both URL form ("postgres://user:pass@host/db") and
// keyword/value DSN form ("host=... password=..."); url.Parse succeeds on the
// latter without populating User, so the DSN form is handled explicitly rather
// than being returned verbatim.
func redactConnectionString(connStr string) string {
	if isKeywordValueDSN(connStr) {
		return redactKeywordValueDSN(connStr)
	}
	parsed, err := url.Parse(connStr)
	if err != nil {
		return "[invalid connection string]"
	}
	if parsed.User != nil {
		if _, hasPass := parsed.User.Password(); hasPass {
			parsed.User = url.UserPassword(parsed.User.Username(), redactedPlaceholder)
		}
	}
	return parsed.String()
}

// isKeywordValueDSN reports whether connStr looks like a libpq keyword/value
// DSN rather than a URL. Anything carrying a scheme is treated as a URL.
func isKeywordValueDSN(connStr string) bool {
	if strings.Contains(connStr, "://") {
		return false
	}
	return strings.Contains(connStr, "=")
}

// redactKeywordValueDSN replaces the value of every password-bearing keyword in
// a libpq keyword/value DSN, preserving the rest for diagnostics. libpq allows
// single-quoted values containing spaces, so values are tokenized rather than
// split on whitespace — otherwise a quoted password would leak its tail.
func redactKeywordValueDSN(dsn string) string {
	var out []string
	rest := dsn
	for {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			break
		}
		key, remainder, found := strings.Cut(rest, "=")
		if !found {
			// Trailing junk with no "=": keep it as-is, it holds no value.
			out = append(out, rest)
			break
		}
		value, remainder := scanDSNValue(remainder)
		if isSecretDSNKeyword(key) {
			value = redactedPlaceholder
		}
		out = append(out, strings.TrimSpace(key)+"="+value)
		rest = remainder
	}
	return strings.Join(out, " ")
}

// scanDSNValue consumes one keyword/value DSN value, honoring single quotes and
// backslash escapes, and returns it alongside the unconsumed remainder.
func scanDSNValue(s string) (value, rest string) {
	var b strings.Builder
	quoted := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'':
			quoted = !quoted
			b.WriteByte(c)
		case c == '\\' && i+1 < len(s):
			i++
			b.WriteByte(c)
			b.WriteByte(s[i])
		case (c == ' ' || c == '\t') && !quoted:
			return b.String(), s[i:]
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), ""
}

// isSecretDSNKeyword reports whether a DSN keyword carries a secret value.
func isSecretDSNKeyword(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "password", "sslpassword", "sslkey":
		return true
	default:
		return false
	}
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

	// Every failure past this point must release the pool sql.Open just created,
	// otherwise a failed Open leaks connections for the life of the process.
	success := false
	defer func() {
		if !success {
			_ = db.Close()
		}
	}()

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

	success = true
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
	raw = NormalizeTarget(raw)
	if raw == "" {
		return ""
	}
	category = scope.NormalizeCategory(category)
	return raw + "|" + category
}

func uniqueIdentityStats(entries []UpsertEntry) (unique, duplicates int) {
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		key := identityKey(e.TargetRaw, e.Category)
		if key == "" {
			continue
		}
		if seen[key] {
			duplicates++
			continue
		}
		seen[key] = true
		unique++
	}
	return unique, duplicates
}

// shouldAbortPartialSync reports whether a sync would disable an implausibly
// large share of a platform's active programs, which signals a partial or failed
// poll rather than genuine removals.
//
// The ratio applies at every scale. An earlier version exempted platforms with
// fewer than ten active programs, which let a single bad poll disable every
// program a small platform had.
func shouldAbortPartialSync(activeCount, removeCount int) bool {
	if removeCount <= 0 {
		return false
	}
	// No poll legitimately removes every program a platform has, including a
	// one-program platform: an empty or truncated listing is indistinguishable
	// from a genuine last-program departure, and wiping is the worse mistake.
	if removeCount >= activeCount {
		return true
	}
	if activeCount <= 1 {
		return false
	}
	// A two-program platform reaches the half mark on a single genuine removal,
	// so the ratio cannot discriminate there. The full-wipe check above still
	// covers the dangerous case.
	if activeCount < syncRatioMinActivePrograms {
		return false
	}
	// Disabling half or more in one poll is the signature of a truncated
	// response. This is >= rather than >: an exactly-half removal is already
	// well outside normal churn.
	return float64(removeCount) >= float64(activeCount)*syncPartialDisableMaxRatio
}

// splitPlatformList parses a comma-separated --platform filter (e.g. "h1,bc")
// into canonical names plus long aliases so `--platform bugcrowd` matches rows
// stored as `bc`. "all" and empty entries are dropped since they mean "no filter".
func splitPlatformList(input string) []string {
	parts := strings.Split(input, ",")
	seen := make(map[string]bool)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		for _, name := range platforms.MatchingNames(p) {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
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
