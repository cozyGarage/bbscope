package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
)

// SetProgramIgnoredStatus sets the is_ignored flag for a program.
// programURL is treated as a substring pattern; LIKE metacharacters (% and _)
// in the user input are escaped so they match literally.
func (d *DB) SetProgramIgnoredStatus(ctx context.Context, programURL string, ignored bool) error {
	pattern := "%" + escapeLikePattern(strings.ToLower(programURL)) + "%"
	res, err := d.sql.ExecContext(ctx, `
		UPDATE programs
		SET is_ignored = $1
		WHERE lower(url) LIKE $2 ESCAPE '\'
		   OR lower(handle) LIKE $2 ESCAPE '\'
	`, boolToInt(ignored), pattern)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no program found matching URL or handle pattern: %s", programURL)
	}
	return nil
}

// SetProgramLifecycle writes the disabled and ignored flags after an import.
// UpsertProgramEntries always re-enables a program (that is the poll path);
// a backup restore must put those flags back or a disabled program comes back live.
func (d *DB) SetProgramLifecycle(ctx context.Context, programURL string, disabled, ignored bool) error {
	programURL = NormalizeProgramURL(programURL)
	res, err := d.sql.ExecContext(ctx, `
		UPDATE programs
		SET disabled = $1, is_ignored = $2
		WHERE url = $3 OR rtrim(url, '/') = rtrim($3, '/')
	`, boolToInt(disabled), boolToInt(ignored), programURL)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no program found matching URL %s", programURL)
	}
	return nil
}

// GetIgnoredPrograms returns a map of program URLs that are marked as ignored for a specific platform.
func (d *DB) GetIgnoredPrograms(ctx context.Context, platform string) (map[string]bool, error) {
	names := platforms.MatchingNames(platform)
	rows, err := d.sql.QueryContext(ctx, "SELECT url FROM programs WHERE lower(platform) = ANY($1) AND is_ignored = 1", pgtype.FlatArray[string](names))
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
	names := platforms.MatchingNames(platform)
	err := d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM programs WHERE lower(platform) = ANY($1) AND disabled = 0 AND is_ignored = 0", pgtype.FlatArray[string](names)).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// SyncPlatformPrograms marks programs that are no longer returned by a platform's API as 'disabled'
// and logs their removal as a single event, preventing spam from individual target removals.
// Targets are retained (soft-disable only) so a bad poll cannot permanently wipe scope data.
//
// The returned changes have already been written to scope_changes as part of the
// same transaction; callers must not pass them to LogChanges.
func (d *DB) SyncPlatformPrograms(ctx context.Context, platform string, polledProgramURLs []string) ([]Change, error) {
	now := time.Now().UTC()
	changes := make([]Change, 0)
	if platforms.KnownPlatform(platform) {
		platform = platforms.CanonicalName(platform)
	}
	matchingPlatforms := platforms.MatchingNames(platform)

	// 1. Create a set of polled URLs for efficient lookup.
	polledURLSet := make(map[string]struct{}, len(polledProgramURLs))
	for _, u := range polledProgramURLs {
		if normalized := NormalizeProgramURL(u); normalized != "" {
			polledURLSet[normalized] = struct{}{}
		}
	}

	// 2. Read the active programs, disable the absent ones, and log the removals
	// in one transaction. Reading outside a transaction and then disabling in
	// per-program transactions left a window where a concurrent poll could
	// re-enable a program between the two, and a commit failure partway through
	// the loop returned change records for programs that were never disabled.
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("starting sync transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// FOR UPDATE holds the rows for the lifetime of the transaction so a
	// concurrent sync or upsert cannot act on the same snapshot.
	rows, err := tx.QueryContext(ctx, `
		SELECT p.id, p.url, p.handle
		FROM programs p
		WHERE lower(p.platform) = ANY($1) AND p.disabled = 0 AND p.is_ignored = 0
		ORDER BY p.id
		FOR UPDATE
	`, pgtype.FlatArray[string](matchingPlatforms))
	if err != nil {
		return nil, fmt.Errorf("querying for active programs: %w", err)
	}

	type programToRemove struct {
		ID     int64
		URL    string
		Handle string
	}
	var toRemove []programToRemove
	activeCount := 0

	for rows.Next() {
		var p programToRemove
		if err := rows.Scan(&p.ID, &p.URL, &p.Handle); err != nil {
			rows.Close()
			return nil, err
		}
		activeCount++
		if _, found := polledURLSet[NormalizeProgramURL(p.URL)]; !found {
			toRemove = append(toRemove, p)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if shouldAbortPartialSync(activeCount, len(toRemove)) {
		return nil, fmt.Errorf("%w: would disable %d of %d active %s programs",
			ErrAbortingPartialSync, len(toRemove), activeCount, platform)
	}

	if len(toRemove) == 0 {
		committed = true
		return changes, tx.Commit()
	}

	// 3. Disable every absent program in one statement.
	ids := make([]int64, len(toRemove))
	for i, p := range toRemove {
		ids[i] = p.ID
	}
	if _, err := tx.ExecContext(ctx, `UPDATE programs SET disabled = 1 WHERE id = ANY($1::bigint[])`, pgtype.FlatArray[int64](ids)); err != nil {
		return nil, fmt.Errorf("disabling %d programs: %w", len(ids), err)
	}

	// 4. One "removed" event per program, rather than per target, so a program
	// leaving a platform does not spam the change log.
	for _, p := range toRemove {
		normalizedProgramURL := NormalizeProgramURL(p.URL)
		changes = append(changes, Change{
			OccurredAt:       now,
			ProgramURL:       normalizedProgramURL,
			Platform:         platform,
			Handle:           p.Handle,
			TargetNormalized: normalizedProgramURL, // Use the program URL as the "target"
			TargetRaw:        p.URL,
			Category:         "program",
			InScope:          false,
			IsBBP:            false,
			ChangeType:       "removed",
		})
	}

	if err := logChangesTx(ctx, tx, changes); err != nil {
		return nil, fmt.Errorf("logging program removals: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing sync transaction: %w", err)
	}
	committed = true

	return changes, nil
}

// LogChanges records changes in their own transaction.
//
// UpsertProgramEntries and SyncPlatformPrograms already write their own audit
// rows inside the transaction that performs the mutation, so callers of those
// two must not pass the returned slice here as well — it would double-log.
// This remains available for callers that produce changes by other means.
func (d *DB) LogChanges(ctx context.Context, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := logChangesTx(ctx, tx, changes); err != nil {
		return err
	}

	return tx.Commit()
}

// logChangesTx appends audit rows using a caller-supplied transaction, so the
// scope mutation and its audit trail commit or roll back together. Logging in a
// separate transaction meant a crash in between left the scope updated with no
// record of what changed.
func logChangesTx(ctx context.Context, tx *sql.Tx, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO scope_changes(occurred_at, program_url, platform, handle, target_normalized, target_raw, target_ai_normalized, category, in_scope, is_bbp, change_type) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range changes {
		_, err := stmt.ExecContext(ctx, c.OccurredAt, c.ProgramURL, c.Platform, c.Handle, c.TargetNormalized, c.TargetRaw, c.TargetAINormalized, c.Category, boolToInt(c.InScope), boolToInt(c.IsBBP), c.ChangeType)
		if err != nil {
			return err
		}
	}

	return nil
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
	programID, err = d.getOrCreateProgramTx(ctx, tx, programURL, platform, handle)
	if err != nil {
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
