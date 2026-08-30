package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cozyGarage/bbscope/v2/pkg/scope"
)

// getOrCreateProgramTx upserts a program row inside an existing transaction.
// programURL is canonicalized via NormalizeProgramURL. Legacy trailing-slash
// variants are reused when present so we do not create duplicate programs.
func (d *DB) getOrCreateProgramTx(ctx context.Context, tx *sql.Tx, programURL, platform, handle string) (int64, error) {
	programURL = NormalizeProgramURL(programURL)
	var programID int64

	err := tx.QueryRowContext(ctx, `
		SELECT id FROM programs
		WHERE platform = $1 AND (
			url = $2 OR
			url = $2 || '/' OR
			rtrim(url, '/') = rtrim($2, '/')
		)
		ORDER BY id ASC
		LIMIT 1
	`, platform, programURL).Scan(&programID)
	if err == nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE programs
			SET platform = $1,
			    handle = $2,
			    url = $3,
			    last_seen_at = CURRENT_TIMESTAMP,
			    disabled = 0
			WHERE id = $4
		`, platform, handle, programURL, programID); err != nil {
			// URL rewrite can fail if another row already owns the canonical URL.
			// Fall back to updating metadata without changing url.
			if _, err2 := tx.ExecContext(ctx, `
				UPDATE programs
				SET platform = $1,
				    handle = $2,
				    last_seen_at = CURRENT_TIMESTAMP,
				    disabled = 0
				WHERE id = $3
			`, platform, handle, programID); err2 != nil {
				return 0, fmt.Errorf("updating program: %w", err2)
			}
		}
		return programID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("looking up program: %w", err)
	}

	row := tx.QueryRowContext(ctx, `
		INSERT INTO programs(platform, handle, url, first_seen_at, last_seen_at)
		VALUES($1,$2,$3,CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(url) DO UPDATE SET
			platform = excluded.platform,
			handle = excluded.handle,
			last_seen_at = CURRENT_TIMESTAMP,
			disabled = 0
		RETURNING id
	`, platform, handle, programURL)
	if err := row.Scan(&programID); err != nil {
		return 0, fmt.Errorf("upserting program: %w", err)
	}
	return programID, nil
}

// UpsertOptions controls optional behavior of an upsert.
type UpsertOptions struct {
	// SkipChangeLog suppresses the scope_changes rows for this upsert. A
	// platform's first poll sets it: every target would otherwise be recorded as
	// an addition, burying later real changes under the initial import.
	SkipChangeLog bool
}

// UpsertProgramEntries reconciles a program's scope against the entries a poll
// produced and returns the detected changes.
//
// The returned changes have already been written to scope_changes as part of the
// same transaction; callers must not pass them to LogChanges. They are returned
// so callers can display or forward them.
func (d *DB) UpsertProgramEntries(ctx context.Context, programURL, platform, handle string, entries []UpsertEntry) ([]Change, error) {
	return d.UpsertProgramEntriesWithOptions(ctx, programURL, platform, handle, entries, UpsertOptions{})
}

// UpsertProgramEntriesWithOptions is UpsertProgramEntries with explicit options.
func (d *DB) UpsertProgramEntriesWithOptions(ctx context.Context, programURL, platform, handle string, entries []UpsertEntry, opts UpsertOptions) ([]Change, error) {
	now := time.Now().UTC()
	programURL = NormalizeProgramURL(programURL)

	// Hold one transaction across read/diff/write so concurrent upserts for the
	// same program cannot compute conflicting diffs from a stale snapshot.
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("beginning upsert transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 1. Get or create program
	programID, err := d.getOrCreateProgramTx(ctx, tx, programURL, platform, handle)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create program: %w", err)
	}
	var lockedProgramID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM programs WHERE id = $1 FOR UPDATE`, programID).Scan(&lockedProgramID); err != nil {
		return nil, fmt.Errorf("locking program %d: %w", programID, err)
	}

	type existingVariant struct {
		ID          int64
		Norm        string
		Category    string
		HasCategory bool
		HasInScope  bool
		InScope     bool
	}

	type existingTarget struct {
		ID       int64
		Raw      string
		Cat      string
		Desc     string
		InScope  bool
		IsBBP    bool
		Variants map[string]existingVariant
	}

	// Helper functions for creating change records
	computeChangeCategoryForEntry := func(entry *UpsertEntry, variant *EntryVariant) string {
		if variant.HasCategory && !strings.EqualFold(variant.Category, entry.Category) {
			return variant.Category
		}
		return entry.Category
	}

	computeChangeCategoryForExisting := func(entry *UpsertEntry, variant *existingVariant) string {
		if variant.HasCategory && !strings.EqualFold(variant.Category, entry.Category) {
			return variant.Category
		}
		return entry.Category
	}

	computeChangeInScopeForEntry := func(entry *UpsertEntry, variant *EntryVariant) bool {
		if variant.HasInScope && variant.InScope != entry.InScope {
			return variant.InScope
		}
		return entry.InScope
	}

	computeChangeInScopeForExisting := func(entry *UpsertEntry, variant *existingVariant) bool {
		if variant.HasInScope && variant.InScope != entry.InScope {
			return variant.InScope
		}
		return entry.InScope
	}

	createChangeWithEntry := func(entry *UpsertEntry, variant *EntryVariant, changeType string) Change {
		return Change{
			OccurredAt:         now,
			ProgramURL:         programURL,
			Platform:           platform,
			Handle:             handle,
			TargetRaw:          entry.TargetRaw,
			TargetNormalized:   entry.TargetNormalized,
			TargetAINormalized: variant.AINormalized,
			Category:           computeChangeCategoryForEntry(entry, variant),
			InScope:            computeChangeInScopeForEntry(entry, variant),
			IsBBP:              entry.IsBBP,
			ChangeType:         changeType,
		}
	}

	createChangeWithExisting := func(entry *UpsertEntry, variant *existingVariant, changeType string) Change {
		return Change{
			OccurredAt:         now,
			ProgramURL:         programURL,
			Platform:           platform,
			Handle:             handle,
			TargetRaw:          entry.TargetRaw,
			TargetNormalized:   entry.TargetNormalized,
			TargetAINormalized: variant.Norm,
			Category:           computeChangeCategoryForExisting(entry, variant),
			InScope:            computeChangeInScopeForExisting(entry, variant),
			IsBBP:              entry.IsBBP,
			ChangeType:         changeType,
		}
	}

	needsVariantUpdate := func(existing *existingVariant, desired *EntryVariant) bool {
		if desired.HasInScope != existing.HasInScope {
			return true
		}
		if desired.HasInScope && existing.InScope != desired.InScope {
			return true
		}
		if desired.HasCategory != existing.HasCategory {
			return true
		}
		if desired.HasCategory && !strings.EqualFold(existing.Category, desired.Category) {
			return true
		}
		return false
	}

	// 2. Load existing targets for this program
	rows, err := tx.QueryContext(ctx, `
		SELECT id, target, category, in_scope, description, is_bbp
		FROM targets_raw
		WHERE program_id = $1
		FOR UPDATE
	`, programID)
	if err != nil {
		return nil, err
	}

	existingMap := make(map[string]*existingTarget)
	existingByID := make(map[int64]*existingTarget)

	for rows.Next() {
		var (
			id, inScope, isBBP int64
			raw, cat           string
			desc               sql.NullString
		)
		if err = rows.Scan(&id, &raw, &cat, &inScope, &desc, &isBBP); err != nil {
			_ = rows.Close()
			return nil, err
		}
		key := identityKey(raw, cat)
		ex := &existingTarget{
			ID:       id,
			Raw:      raw,
			Cat:      cat,
			Desc:     desc.String,
			InScope:  inScope == 1,
			IsBBP:    isBBP == 1,
			Variants: make(map[string]existingVariant),
		}
		existingMap[key] = ex
		existingByID[id] = ex
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	// 3. Load existing AI enhancements tied to those targets
	variantRows, err := tx.QueryContext(ctx, `
		SELECT v.id, v.target_id, v.target_ai_normalized, v.category, v.in_scope
		FROM targets_ai_enhanced v
		JOIN targets_raw t ON v.target_id = t.id
		WHERE t.program_id = $1
	`, programID)
	if err != nil {
		return nil, err
	}

	for variantRows.Next() {
		var (
			id, targetID int64
			normAI       string
			catNS        sql.NullString
			inScopeNS    sql.NullInt64
		)
		if err := variantRows.Scan(&id, &targetID, &normAI, &catNS, &inScopeNS); err != nil {
			_ = variantRows.Close()
			return nil, err
		}
		if target, ok := existingByID[targetID]; ok {
			if normAI != "" {
				if target.Variants == nil {
					target.Variants = make(map[string]existingVariant)
				}
				target.Variants[normAI] = existingVariant{
					ID:          id,
					Norm:        normAI,
					Category:    strings.ToLower(catNS.String),
					HasCategory: catNS.Valid,
					HasInScope:  inScopeNS.Valid,
					InScope:     inScopeNS.Int64 == 1,
				}
			}
		}
	}
	if err := variantRows.Err(); err != nil {
		_ = variantRows.Close()
		return nil, err
	}
	if err := variantRows.Close(); err != nil {
		return nil, err
	}

	// SAFETY CHECK
	//
	// Count entries that yield a usable identity key, not raw entries. Entries
	// whose target normalizes to "" are skipped by the diff loop below, so a
	// poller that starts returning blank targets — the shape of a platform markup
	// change — would otherwise clear the guard and have every existing target
	// collected into toRemove and deleted.
	usableEntries := 0
	for _, e := range entries {
		if identityKey(e.TargetRaw, e.Category) != "" {
			usableEntries++
		}
	}
	if usableEntries == 0 && len(existingMap) > 0 {
		return nil, ErrAbortingScopeWipe
	}

	// 4. Compare incoming data against existing state
	// Pre-allocate slices with estimated capacity to reduce allocations.
	// These estimates are based on typical update patterns:
	// - Changes (additions + updates): ~50% (25% new + 25% updates)
	// - Unchanged (touches only): ~50%
	const (
		estimatedChangeRatio = 0.5  // ~50% of entries result in changes (25% new + 25% updates)
		estimatedNewRatio    = 0.25 // ~25% are new entries
		estimatedUpdateRatio = 0.25 // ~25% are updates
	)

	changes := make([]Change, 0, int(float64(len(entries))*estimatedChangeRatio))
	processedKeys := make(map[string]bool)
	entryByKey := make(map[string]UpsertEntry)
	targetIDs := make(map[string]int64, len(existingMap))
	for key, ex := range existingMap {
		targetIDs[key] = ex.ID
	}

	toAdd := make([]UpsertEntry, 0, int(float64(len(entries))*estimatedNewRatio))
	toUpdate := make([]struct {
		entry UpsertEntry
		id    int64
	}, 0, int(float64(len(entries))*estimatedUpdateRatio))
	toTouch := make([]int64, 0, int(float64(len(entries))*(1.0-estimatedChangeRatio)))

	for _, e := range entries {
		key := identityKey(e.TargetRaw, e.Category)
		if key == "" {
			continue
		}
		if processedKeys[key] {
			continue
		}
		processedKeys[key] = true
		entryByKey[key] = e

		ex, existed := existingMap[key]
		if !existed {
			toAdd = append(toAdd, e)
			changes = append(changes, Change{
				OccurredAt:       now,
				ProgramURL:       programURL,
				Platform:         platform,
				Handle:           handle,
				TargetRaw:        e.TargetRaw,
				TargetNormalized: e.TargetNormalized,
				Category:         e.Category,
				InScope:          e.InScope,
				IsBBP:            e.IsBBP,
				ChangeType:       "added",
			})
		} else {
			if ex.Desc != e.Description || ex.InScope != e.InScope || ex.IsBBP != e.IsBBP {
				toUpdate = append(toUpdate, struct {
					entry UpsertEntry
					id    int64
				}{entry: e, id: ex.ID})
				changes = append(changes, Change{
					OccurredAt:       now,
					ProgramURL:       programURL,
					Platform:         platform,
					Handle:           handle,
					TargetRaw:        e.TargetRaw,
					TargetNormalized: e.TargetNormalized,
					Category:         e.Category,
					InScope:          e.InScope,
					IsBBP:            e.IsBBP,
					ChangeType:       "updated",
				})
			} else {
				toTouch = append(toTouch, ex.ID)
			}
		}
	}

	var toRemove []*existingTarget
	for key, ex := range existingMap {
		if !processedKeys[key] {
			copied := *ex
			toRemove = append(toRemove, &copied)
			normalized := NormalizeTarget(ex.Raw)
			changes = append(changes, Change{
				OccurredAt:       now,
				ProgramURL:       programURL,
				Platform:         platform,
				Handle:           handle,
				TargetRaw:        ex.Raw,
				TargetNormalized: normalized,
				Category:         ex.Cat,
				InScope:          ex.InScope,
				IsBBP:            ex.IsBBP,
				ChangeType:       "removed",
			})
		}
	}

	// 5. Apply every write below in the same transaction opened for read/diff.
	if len(toAdd) > 0 {
		// Prepare arrays for bulk insert using UNNEST
		targets := make([]string, len(toAdd))
		categories := make([]string, len(toAdd))
		descriptions := make([]sql.NullString, len(toAdd))
		inScopes := make([]int, len(toAdd))
		isBBPs := make([]int, len(toAdd))

		// Build a lookup map for matching returned rows
		addEntryByKey := make(map[string]UpsertEntry, len(toAdd))
		for i, e := range toAdd {
			targets[i] = e.TargetRaw
			categories[i] = e.Category
			if e.Description != "" {
				descriptions[i] = sql.NullString{String: e.Description, Valid: true}
			}
			inScopes[i] = boolToInt(e.InScope)
			isBBPs[i] = boolToInt(e.IsBBP)
			addEntryByKey[identityKey(e.TargetRaw, e.Category)] = e
		}

		// Bulk insert using UNNEST - returns id, target, category to match back
		rows, err := tx.QueryContext(ctx, `
			INSERT INTO targets_raw(program_id, target, category, description, in_scope, is_bbp, first_seen_at, last_seen_at)
			SELECT $1, t.target, t.category, t.description, t.in_scope, t.is_bbp, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			FROM UNNEST($2::text[], $3::text[], $4::text[], $5::int[], $6::int[]) AS t(target, category, description, in_scope, is_bbp)
			ON CONFLICT(program_id, category, target) DO UPDATE SET
				description = excluded.description,
				in_scope = excluded.in_scope,
				is_bbp = excluded.is_bbp,
				last_seen_at = CURRENT_TIMESTAMP
			RETURNING id, target, category
		`, programID, pgtype.FlatArray[string](targets), pgtype.FlatArray[string](categories), pgtype.FlatArray[sql.NullString](descriptions), pgtype.FlatArray[int](inScopes), pgtype.FlatArray[int](isBBPs))
		if err != nil {
			return nil, fmt.Errorf("bulk inserting targets: %w", err)
		}

		for rows.Next() {
			var id int64
			var target, category string
			if err := rows.Scan(&id, &target, &category); err != nil {
				rows.Close()
				return nil, err
			}
			key := identityKey(target, category)
			targetIDs[key] = id
			if e, ok := addEntryByKey[key]; ok {
				ex := &existingTarget{
					ID:       id,
					Raw:      e.TargetRaw,
					Cat:      e.Category,
					Desc:     e.Description,
					InScope:  e.InScope,
					IsBBP:    e.IsBBP,
					Variants: make(map[string]existingVariant),
				}
				existingMap[key] = ex
			}
		}
		rows.Close()
	}

	// Batch Updates
	if len(toUpdate) > 0 {
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets_raw SET description = $1, in_scope = $2, is_bbp = $3, last_seen_at = CURRENT_TIMESTAMP WHERE id = $4`)
		if err != nil {
			return nil, err
		}
		for _, u := range toUpdate {
			if _, err := stmt.ExecContext(ctx, nullIfEmpty(u.entry.Description), boolToInt(u.entry.InScope), boolToInt(u.entry.IsBBP), u.id); err != nil {
				stmt.Close()
				return nil, err
			}
		}
		stmt.Close()
	}

	// Batch Touches (update last_seen_at) - single query using ANY
	if len(toTouch) > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE targets_raw SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ANY($1::bigint[])`, pgtype.FlatArray[int64](toTouch)); err != nil {
			return nil, fmt.Errorf("batch touching targets: %w", err)
		}
	}

	// Synchronize AI enhancements for remaining entries
	type variantAddOp struct {
		targetID int64
		key      string
		entry    UpsertEntry
		variant  EntryVariant
	}
	type variantUpdateOp struct {
		id      int64
		key     string
		entry   UpsertEntry
		variant EntryVariant
	}
	type variantDeleteOp struct {
		id      int64
		key     string
		entry   UpsertEntry
		variant existingVariant
	}

	// Pre-allocate variant operation slices with estimated capacity.
	// Typical entry has ~2 variants (normalized + AI-normalized).
	// We estimate ~25% will be added, updated, or deleted.
	const (
		avgVariantsPerEntry   = 2
		variantOperationRatio = 0.25 // 1/4 of variants will need operations
	)
	estimatedVariants := len(entryByKey) * avgVariantsPerEntry
	variantOpCapacity := int(float64(estimatedVariants) * variantOperationRatio)

	var (
		variantAdds    = make([]variantAddOp, 0, variantOpCapacity)
		variantUpdates = make([]variantUpdateOp, 0, variantOpCapacity)
		variantDeletes = make([]variantDeleteOp, 0, variantOpCapacity)
	)

	for key, entry := range entryByKey {
		targetID, ok := targetIDs[key]
		if !ok || targetID == 0 {
			continue
		}
		existing := existingMap[key]
		if existing == nil {
			continue
		}
		if existing.Variants == nil {
			existing.Variants = make(map[string]existingVariant)
		}

		desired := make(map[string]EntryVariant, len(entry.Variants))
		for _, variant := range entry.Variants {
			if variant.AINormalized == "" {
				continue
			}
			desired[variant.AINormalized] = variant
		}

		for norm, variant := range desired {
			if ev, found := existing.Variants[norm]; !found {
				variantAdds = append(variantAdds, variantAddOp{
					targetID: targetID,
					key:      key,
					entry:    entry,
					variant:  variant,
				})

				changes = append(changes, createChangeWithEntry(&entry, &variant, "added"))
			} else {
				// Use helper function to check if update is needed
				if !needsVariantUpdate(&ev, &variant) {
					continue
				}

				variantUpdates = append(variantUpdates, variantUpdateOp{
					id:      ev.ID,
					key:     key,
					entry:   entry,
					variant: variant,
				})

				changes = append(changes, createChangeWithEntry(&entry, &variant, "updated"))
			}
		}

		for norm, ev := range existing.Variants {
			if _, desiredExists := desired[norm]; desiredExists {
				continue
			}
			variantDeletes = append(variantDeletes, variantDeleteOp{
				id:      ev.ID,
				key:     key,
				entry:   entry,
				variant: ev,
			})

			changes = append(changes, createChangeWithExisting(&entry, &ev, "removed"))
		}
	}

	if len(variantAdds) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO targets_ai_enhanced(target_id, target_ai_normalized, category, in_scope, first_seen_at, last_seen_at)
			VALUES($1,$2,$3,$4,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
			ON CONFLICT(target_id, target_ai_normalized) DO UPDATE SET
				category = COALESCE(excluded.category, targets_ai_enhanced.category),
				in_scope = COALESCE(excluded.in_scope, targets_ai_enhanced.in_scope),
				last_seen_at = CURRENT_TIMESTAMP
			RETURNING id
		`)
		if err != nil {
			return nil, err
		}
		for _, add := range variantAdds {
			var catVal interface{}
			if add.variant.HasCategory && !strings.EqualFold(add.variant.Category, add.entry.Category) {
				catVal = add.variant.Category
			} else {
				add.variant.HasCategory = false
			}
			var inScopeVal interface{}
			if add.variant.HasInScope && add.variant.InScope != add.entry.InScope {
				inScopeVal = boolToInt(add.variant.InScope)
			} else {
				add.variant.HasInScope = false
			}
			var id int64
			if err := stmt.QueryRowContext(ctx, add.targetID, add.variant.AINormalized, catVal, inScopeVal).Scan(&id); err != nil {
				stmt.Close()
				return nil, err
			}
			if existing := existingMap[add.key]; existing != nil {
				existing.Variants[add.variant.AINormalized] = existingVariant{
					ID:          id,
					Norm:        add.variant.AINormalized,
					Category:    add.variant.Category,
					HasCategory: add.variant.HasCategory,
					HasInScope:  add.variant.HasInScope,
					InScope:     add.variant.InScope,
				}
			}
		}
		stmt.Close()
	}

	if len(variantUpdates) > 0 {
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets_ai_enhanced SET target_ai_normalized = $1, category = $2, in_scope = $3, last_seen_at = CURRENT_TIMESTAMP WHERE id = $4`)
		if err != nil {
			return nil, err
		}
		for _, upd := range variantUpdates {
			var catVal interface{}
			if upd.variant.HasCategory && !strings.EqualFold(upd.variant.Category, upd.entry.Category) {
				catVal = upd.variant.Category
			} else {
				upd.variant.HasCategory = false
			}
			var inScopeVal interface{}
			if upd.variant.HasInScope && upd.variant.InScope != upd.entry.InScope {
				inScopeVal = boolToInt(upd.variant.InScope)
			} else {
				upd.variant.HasInScope = false
			}
			if _, err := stmt.ExecContext(ctx, upd.variant.AINormalized, catVal, inScopeVal, upd.id); err != nil {
				stmt.Close()
				return nil, err
			}
			if existing := existingMap[upd.key]; existing != nil {
				existing.Variants[upd.variant.AINormalized] = existingVariant{
					ID:          upd.id,
					Norm:        upd.variant.AINormalized,
					Category:    upd.variant.Category,
					HasCategory: upd.variant.HasCategory,
					HasInScope:  upd.variant.HasInScope,
					InScope:     upd.variant.InScope,
				}
			}
		}
		stmt.Close()
	}

	if len(variantDeletes) > 0 {
		stmt, err := tx.PrepareContext(ctx, `DELETE FROM targets_ai_enhanced WHERE id = $1`)
		if err != nil {
			return nil, err
		}
		for _, del := range variantDeletes {
			if _, err := stmt.ExecContext(ctx, del.id); err != nil {
				stmt.Close()
				return nil, err
			}
			if existing := existingMap[del.key]; existing != nil {
				delete(existing.Variants, del.variant.Norm)
			}
		}
		stmt.Close()
	}

	// Batch Deletes - single query using ANY
	if len(toRemove) > 0 {
		ids := make([]int64, len(toRemove))
		for i, ex := range toRemove {
			ids[i] = ex.ID
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM targets_raw WHERE id = ANY($1::bigint[])`, pgtype.FlatArray[int64](ids)); err != nil {
			return nil, fmt.Errorf("batch deleting targets: %w", err)
		}
	}

	// Audit rows go in the same transaction as the mutation they describe, so a
	// failure here rolls the scope change back rather than leaving the database
	// updated with no record of what happened.
	if !opts.SkipChangeLog {
		if err := logChangesTx(ctx, tx, changes); err != nil {
			return nil, fmt.Errorf("logging scope changes: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing upsert transaction: %w", err)
	}
	committed = true

	return changes, nil
}

// BuildEntries canonicalizes raw items into upsert entries with optional variants.
func BuildEntries(programURL, platform, handle string, items []TargetItem) ([]UpsertEntry, error) {
	if programURL == "" || platform == "" {
		return nil, errors.New("invalid program identifiers")
	}
	out := make([]UpsertEntry, 0, len(items))
	for _, it := range items {
		normalized := NormalizeTarget(it.URI)
		normalizedCategory := scope.NormalizeCategory(it.Category)

		entry := UpsertEntry{
			ProgramURL:       NormalizeProgramURL(programURL),
			Platform:         platform,
			Handle:           handle,
			TargetNormalized: normalized,
			TargetRaw:        it.URI,
			Category:         normalizedCategory,
			Description:      it.Description,
			InScope:          it.InScope,
			IsBBP:            it.IsBBP,
		}

		if len(it.Variants) > 0 {
			entry.Variants = make([]EntryVariant, 0, len(it.Variants))
			for _, variant := range it.Variants {
				rawValue := strings.TrimSpace(variant.Value)
				if rawValue == "" {
					continue
				}
				variantNorm := NormalizeTarget(rawValue)
				inScope := entry.InScope
				hasInScope := false
				if variant.HasInScope {
					hasInScope = true
					inScope = variant.InScope
				}
				var cat string
				var hasCat bool
				if variant.HasCategory && scope.IsUnifiedCategory(strings.ToLower(strings.TrimSpace(variant.Category))) {
					cat = strings.ToLower(strings.TrimSpace(variant.Category))
					hasCat = true
				}
				entry.Variants = append(entry.Variants, EntryVariant{
					AINormalized: variantNorm,
					HasInScope:   hasInScope,
					InScope:      inScope,
					HasCategory:  hasCat,
					Category:     cat,
				})
			}
		}

		out = append(out, entry)
	}
	return out, nil
}
