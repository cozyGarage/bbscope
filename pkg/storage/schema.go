package storage

import (
	"database/sql"
	"fmt"

	"github.com/cozyGarage/bbscope/v2/pkg/scope"
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
	fn      func(*sql.Tx) error // optional Go-side migration step
}

// migrations lists schema changes in ascending version order. To evolve the
// schema, append a new migration here — never edit or reorder a released one.
var migrations = []migration{
	{version: 1, name: "initial_schema", stmts: schema},
	{version: 2, name: "canonicalize_program_urls_and_targets", fn: canonicalizeProgramURLsAndTargets},
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
	if err := rows.Err(); err != nil {
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
		if m.stmts != "" {
			if _, err := tx.Exec(m.stmts); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("applying migration %d (%s): %w", m.version, m.name, err)
			}
		}
		if m.fn != nil {
			if err := m.fn(tx); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("applying migration %d (%s): %w", m.version, m.name, err)
			}
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

// canonicalizeProgramURLsAndTargets merges legacy duplicate programs/targets that
// only differ by URL/target canonicalization (trailing slash, host case, etc.).
func canonicalizeProgramURLsAndTargets(tx *sql.Tx) error {
	if err := mergeDuplicatePrograms(tx); err != nil {
		return err
	}
	return mergeDuplicateTargets(tx)
}

func mergeDuplicatePrograms(tx *sql.Tx) error {
	type prog struct {
		id       int64
		platform string
		url      string
	}
	rows, err := tx.Query(`SELECT id, platform, url FROM programs ORDER BY id ASC`)
	if err != nil {
		return fmt.Errorf("listing programs: %w", err)
	}
	defer rows.Close()

	byKey := make(map[string][]prog)
	for rows.Next() {
		var p prog
		if err := rows.Scan(&p.id, &p.platform, &p.url); err != nil {
			return err
		}
		key := p.platform + "|" + NormalizeProgramURL(p.url)
		byKey[key] = append(byKey[key], p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for key, group := range byKey {
		if len(group) == 0 {
			continue
		}
		if len(group) == 1 {
			// Still rewrite single rows to canonical URL when needed.
			canonical := NormalizeProgramURL(group[0].url)
			if canonical != "" && canonical != group[0].url {
				if _, err := tx.Exec(`UPDATE programs SET url = $1 WHERE id = $2`, canonical, group[0].id); err != nil {
					// Unique conflict means another row already owns it; ignore rewrite.
					_ = err
				}
			}
			continue
		}
		keeper := group[0]
		canonical := NormalizeProgramURL(keeper.url)
		for _, loser := range group[1:] {
			if err := reassignProgramTargets(tx, loser.id, keeper.id); err != nil {
				return fmt.Errorf("merging program group %s: %w", key, err)
			}
			if _, err := tx.Exec(`UPDATE scope_changes SET program_url = $1 WHERE program_url = $2`, canonical, loser.url); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM programs WHERE id = $1`, loser.id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE programs SET url = $1 WHERE id = $2`, canonical, keeper.id); err != nil {
			return fmt.Errorf("canonicalizing program %d: %w", keeper.id, err)
		}
	}
	return nil
}

func reassignProgramTargets(tx *sql.Tx, fromProgramID, toProgramID int64) error {
	rows, err := tx.Query(`
		SELECT id, target, category, description, in_scope, is_bbp, first_seen_at, last_seen_at
		FROM targets_raw WHERE program_id = $1 ORDER BY id ASC
	`, fromProgramID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type tgt struct {
		id                  int64
		target, category    string
		desc                sql.NullString
		inScope, isBBP      int
		firstSeen, lastSeen sql.NullTime
	}
	var targets []tgt
	for rows.Next() {
		var t tgt
		if err := rows.Scan(&t.id, &t.target, &t.category, &t.desc, &t.inScope, &t.isBBP, &t.firstSeen, &t.lastSeen); err != nil {
			return err
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, t := range targets {
		var existingID int64
		err := tx.QueryRow(`
			SELECT id FROM targets_raw
			WHERE program_id = $1 AND category = $2 AND target = $3
			LIMIT 1
		`, toProgramID, t.category, t.target).Scan(&existingID)
		if err == sql.ErrNoRows {
			if _, err := tx.Exec(`
				UPDATE targets_raw SET program_id = $1 WHERE id = $2
			`, toProgramID, t.id); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		// Conflict: move AI variants then delete loser target.
		if _, err := tx.Exec(`
			UPDATE targets_ai_enhanced SET target_id = $1
			WHERE target_id = $2
			  AND NOT EXISTS (
				SELECT 1 FROM targets_ai_enhanced x
				WHERE x.target_id = $1 AND x.target_ai_normalized = targets_ai_enhanced.target_ai_normalized
			  )
		`, existingID, t.id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM targets_ai_enhanced WHERE target_id = $1`, t.id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM targets_raw WHERE id = $1`, t.id); err != nil {
			return err
		}
	}
	return nil
}

func mergeDuplicateTargets(tx *sql.Tx) error {
	rows, err := tx.Query(`
		SELECT id, program_id, target, category
		FROM targets_raw
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type tgt struct {
		id, programID int64
		target, cat   string
	}
	byKey := make(map[string][]tgt)
	for rows.Next() {
		var t tgt
		if err := rows.Scan(&t.id, &t.programID, &t.target, &t.cat); err != nil {
			return err
		}
		key := fmt.Sprintf("%d|%s", t.programID, identityKey(t.target, t.cat))
		byKey[key] = append(byKey[key], t)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, group := range byKey {
		if len(group) == 0 {
			continue
		}
		keeper := group[0]
		canonicalTarget := NormalizeTarget(keeper.target)
		canonicalCat := scope.NormalizeCategory(keeper.cat)
		for _, loser := range group[1:] {
			if _, err := tx.Exec(`
				UPDATE targets_ai_enhanced SET target_id = $1
				WHERE target_id = $2
				  AND NOT EXISTS (
					SELECT 1 FROM targets_ai_enhanced x
					WHERE x.target_id = $1 AND x.target_ai_normalized = targets_ai_enhanced.target_ai_normalized
				  )
			`, keeper.id, loser.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM targets_ai_enhanced WHERE target_id = $1`, loser.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM targets_raw WHERE id = $1`, loser.id); err != nil {
				return err
			}
		}
		if canonicalTarget != keeper.target || canonicalCat != keeper.cat {
			if _, err := tx.Exec(`
				UPDATE targets_raw SET target = $1, category = $2 WHERE id = $3
			`, canonicalTarget, canonicalCat, keeper.id); err != nil {
				// Unique conflict: leave as-is rather than failing the whole migration.
				_ = err
			}
		}
	}
	return nil
}
