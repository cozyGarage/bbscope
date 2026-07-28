package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SetProgramIgnoredStatus sets the is_ignored flag for a program.
// programURL is treated as a substring pattern; LIKE metacharacters (% and _)
// in the user input are escaped so they match literally.
func (d *DB) SetProgramIgnoredStatus(ctx context.Context, programURL string, ignored bool) error {
	pattern := "%" + escapeLikePattern(programURL) + "%"
	res, err := d.sql.ExecContext(ctx, "UPDATE programs SET is_ignored = $1 WHERE url LIKE $2 ESCAPE '\\'", boolToInt(ignored), pattern)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no program found matching URL pattern: %s", programURL)
	}
	return nil
}

// GetIgnoredPrograms returns a map of program URLs that are marked as ignored for a specific platform.
func (d *DB) GetIgnoredPrograms(ctx context.Context, platform string) (map[string]bool, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT url FROM programs WHERE platform = $1 AND is_ignored = 1", platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ignoredMap := make(map[string]bool)
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		ignoredMap[url] = true
	}
	return ignoredMap, rows.Err()
}

// GetActiveProgramCount returns the number of active (not disabled, not ignored) programs for a platform.
func (d *DB) GetActiveProgramCount(ctx context.Context, platform string) (int, error) {
	var count int
	err := d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM programs WHERE platform = $1 AND disabled = 0 AND is_ignored = 0", platform).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// SyncPlatformPrograms marks programs that are no longer returned by a platform's API as 'disabled'
// and logs their removal as a single event, preventing spam from individual target removals.
func (d *DB) SyncPlatformPrograms(ctx context.Context, platform string, polledProgramURLs []string) ([]Change, error) {
	now := time.Now().UTC()
	changes := make([]Change, 0)

	// 1. Create a set of polled URLs for efficient lookup.
	polledURLSet := make(map[string]struct{}, len(polledProgramURLs))
	for _, url := range polledProgramURLs {
		polledURLSet[url] = struct{}{}
	}

	// 2. Get all active programs for this platform from the DB (read operation, no transaction needed).
	rows, err := d.sql.QueryContext(ctx, `
		SELECT p.id, p.url, p.handle
		FROM programs p
		WHERE p.platform = $1 AND p.disabled = 0 AND p.is_ignored = 0
	`, platform)
	if err != nil {
		return nil, fmt.Errorf("querying for active programs: %w", err)
	}
	defer rows.Close()

	type programToRemove struct {
		ID     int64
		URL    string
		Handle string
	}
	var toRemove []programToRemove

	for rows.Next() {
		var p programToRemove
		if err := rows.Scan(&p.ID, &p.URL, &p.Handle); err != nil {
			return nil, err
		}
		if _, found := polledURLSet[p.URL]; !found {
			toRemove = append(toRemove, p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 3. For each program that was not in the latest poll, process its removal
	// in its own short-lived transaction to avoid long-held locks.
	for _, p := range toRemove {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, fmt.Errorf("starting transaction for program removal %d: %w", p.ID, err)
		}

		// Mark the program as disabled
		if _, err := tx.ExecContext(ctx, `UPDATE programs SET disabled = 1 WHERE id = $1`, p.ID); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("disabling program %d: %w", p.ID, err)
		}

		// Delete associated targets
		if _, err := tx.ExecContext(ctx, `DELETE FROM targets_raw WHERE program_id = $1`, p.ID); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("deleting targets for program %d: %w", p.ID, err)
		}

		// Create a single "removed" change event for the entire program
		change := Change{
			OccurredAt:       now,
			ProgramURL:       p.URL,
			Platform:         platform,
			Handle:           p.Handle,
			TargetNormalized: p.URL, // Use the program URL as the "target"
			TargetRaw:        p.URL,
			Category:         "program",
			InScope:          false,
			IsBBP:            false,
			ChangeType:       "removed",
		}
		changes = append(changes, change)

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction for program removal %d: %w", p.ID, err)
		}
	}

	return changes, nil
}

func (d *DB) LogChanges(ctx context.Context, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO scope_changes(occurred_at, program_url, platform, handle, target_normalized, target_raw, target_ai_normalized, category, in_scope, is_bbp, change_type) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range changes {
		_, err := stmt.ExecContext(ctx, c.OccurredAt, c.ProgramURL, c.Platform, c.Handle, c.TargetNormalized, c.TargetRaw, c.TargetAINormalized, c.Category, boolToInt(c.InScope), boolToInt(c.IsBBP), c.ChangeType)
		if err != nil {
			return err // Rollback will be called
		}
	}

	return tx.Commit()
}

// AddCustomTarget adds a single target for a custom program.
// It returns true if the target was newly created, false if it already existed.
func (d *DB) AddCustomTarget(ctx context.Context, target, category, programURL string) (bool, error) {
	platform := "custom"
	// If programURL is "custom", use "custom" as the program URL (don't append target)
	if programURL == "custom" {
		programURL = "custom"
	}
	handle := target

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var programID int64
	programRow := tx.QueryRowContext(ctx, `
		INSERT INTO programs(platform, handle, url, first_seen_at, last_seen_at)
		VALUES($1,$2,$3,CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(url) DO UPDATE SET
			platform = excluded.platform,
			handle = excluded.handle,
			last_seen_at = CURRENT_TIMESTAMP,
			disabled = 0
		RETURNING id
	`, platform, handle, programURL)
	if err = programRow.Scan(&programID); err != nil {
		return false, fmt.Errorf("upserting custom program: %w", err)
	}

	targetExists := false
	var exists int
	err = tx.QueryRowContext(ctx, `
		SELECT 1 FROM targets_raw WHERE program_id = $1 AND category = $2 AND target = $3 LIMIT 1
	`, programID, category, target).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("checking existing custom target: %w", err)
	}
	if err == nil {
		targetExists = true
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO targets_raw(program_id, target, category, in_scope, is_bbp, first_seen_at, last_seen_at)
		VALUES($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(program_id, category, target) DO UPDATE SET
			last_seen_at = CURRENT_TIMESTAMP
	`, programID, target, category, boolToInt(true), boolToInt(false))
	if err != nil {
		return false, fmt.Errorf("upserting custom target: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return !targetExists, nil
}
