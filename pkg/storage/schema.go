package storage

import (
	"database/sql"
	"fmt"
)

// schema is the initial (v1) database schema. It uses IF NOT EXISTS so it stays
// idempotent and safe to run against databases created before the migration
// framework existed.
const schema = `
CREATE TABLE IF NOT EXISTS programs (
	id        SERIAL PRIMARY KEY,
	platform  TEXT NOT NULL,
	handle    TEXT NOT NULL,
	url       TEXT NOT NULL UNIQUE,
	first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	strict    INTEGER NOT NULL DEFAULT 0 CHECK (strict IN (0,1)),
	disabled  INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0,1)),
	is_ignored INTEGER NOT NULL DEFAULT 0 CHECK (is_ignored IN (0,1))
);
CREATE INDEX IF NOT EXISTS idx_programs_platform ON programs(platform);
CREATE INDEX IF NOT EXISTS idx_programs_url ON programs(url);
CREATE TABLE IF NOT EXISTS targets_raw (
	id                SERIAL PRIMARY KEY,
	program_id        INTEGER NOT NULL,
	target            TEXT NOT NULL,
	category          TEXT NOT NULL,
	description       TEXT,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	is_bbp            INTEGER NOT NULL DEFAULT 0 CHECK (is_bbp IN (0,1)),
	first_seen_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(program_id) REFERENCES programs(id),
	UNIQUE(program_id, category, target)
);
CREATE INDEX IF NOT EXISTS idx_targets_raw_program_id ON targets_raw(program_id);
CREATE TABLE IF NOT EXISTS targets_ai_enhanced (
	id                   SERIAL PRIMARY KEY,
	target_id            INTEGER NOT NULL,
	target_ai_normalized TEXT NOT NULL,
	category             TEXT,
	in_scope             INTEGER,
	first_seen_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(target_id) REFERENCES targets_raw(id) ON DELETE CASCADE,
	UNIQUE(target_id, target_ai_normalized)
);
CREATE INDEX IF NOT EXISTS idx_targets_ai_enhanced_target_id ON targets_ai_enhanced(target_id);
CREATE TABLE IF NOT EXISTS scope_changes (
	id                SERIAL PRIMARY KEY,
	occurred_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	program_url       TEXT NOT NULL,
	platform          TEXT NOT NULL,
	handle            TEXT NOT NULL,
	target_normalized TEXT NOT NULL,
	target_raw        TEXT NOT NULL DEFAULT '',
	target_ai_normalized TEXT NOT NULL DEFAULT '',
	category          TEXT NOT NULL,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	is_bbp            INTEGER NOT NULL DEFAULT 0 CHECK (is_bbp IN (0,1)),
	change_type       TEXT NOT NULL CHECK (change_type IN ('added','updated','removed'))
);
CREATE INDEX IF NOT EXISTS idx_changes_time ON scope_changes(occurred_at);
CREATE INDEX IF NOT EXISTS idx_changes_program ON scope_changes(program_url, occurred_at);
`

// migration is a single ordered schema change.
type migration struct {
	version int
	name    string
	stmts   string
}

// migrations lists schema changes in ascending version order. To evolve the
// schema, append a new migration here — never edit or reorder a released one.
var migrations = []migration{
	{version: 1, name: "initial_schema", stmts: schema},
}

// applyMigrations ensures the schema_migrations bookkeeping table exists and
// runs any not-yet-applied migrations, each in its own transaction. It is safe
// to run on every startup and on databases predating this framework (migration
// 1 is idempotent, so it simply records itself as applied).
func applyMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("reading applied migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			_ = rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("reading applied migrations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("starting migration %d: %w", m.version, err)
		}
		if _, err := tx.Exec(m.stmts); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("applying migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, name) VALUES($1, $2)`, m.version, m.name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", m.version, err)
		}
	}
	return nil
}
