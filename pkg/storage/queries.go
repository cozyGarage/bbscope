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

// ListAICoveredTargets returns a set of normalized target+category keys that already have AI enhancements.
func (d *DB) ListAIEnhancements(ctx context.Context, programURL string) (map[string][]TargetVariant, error) {
	result := make(map[string][]TargetVariant)
	if programURL == "" {
		return result, nil
	}

	// Programs are stored under NormalizeProgramURL, and callers pass the raw
	// poller URL. Match the canonical form plus the legacy trailing-slash
	// variants that getOrCreateProgramTx also accepts, otherwise existing
	// enhancements are missed and the AI work is redone every poll.
	normalizedURL := NormalizeProgramURL(programURL)

	var programID int64
	err := d.sql.QueryRowContext(ctx, `
		SELECT id FROM programs
		WHERE url = $1 OR url = $1 || '/' OR rtrim(url, '/') = rtrim($1, '/')
		ORDER BY id ASC
		LIMIT 1
	`, normalizedURL).Scan(&programID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := d.sql.QueryContext(ctx, `
		SELECT t.target, t.category, a.target_ai_normalized, a.in_scope, a.category
		FROM targets_ai_enhanced a
		JOIN targets_raw t ON a.target_id = t.id
		WHERE t.program_id = $1
	`, programID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			rawTarget    string
			category     string
			aiValue      string
			inScopeNS    sql.NullInt64
			aiCategoryNS sql.NullString
		)
		if err := rows.Scan(&rawTarget, &category, &aiValue, &inScopeNS, &aiCategoryNS); err != nil {
			return nil, err
		}

		key := BuildTargetCategoryKey(rawTarget, category)
		if key == "" {
			continue
		}

		variant := TargetVariant{
			Value:       aiValue,
			HasInScope:  inScopeNS.Valid,
			InScope:     inScopeNS.Int64 == 1,
			HasCategory: false,
		}
		if aiCategoryNS.Valid {
			cat := strings.ToLower(strings.TrimSpace(aiCategoryNS.String))
			if scope.IsUnifiedCategory(cat) {
				variant.HasCategory = true
				variant.Category = cat
			}
		}

		result[key] = append(result[key], variant)
	}

	return result, rows.Err()
}

// BuildTargetCategoryKey creates a normalized key for a target/category combination.
func BuildTargetCategoryKey(target, category string) string {
	normTarget := strings.ToLower(NormalizeTarget(target))
	if normTarget == "" {
		normTarget = strings.ToLower(strings.TrimSpace(target))
	}
	normCategory := scope.NormalizeCategory(category)
	return fmt.Sprintf("%s|%s", normTarget, normCategory)
}

// ListOptions controls selection when listing entries.
type ListOptions struct {
	Platform        string
	ProgramFilter   string
	Since           time.Time
	IncludeOOS      bool
	IncludeIgnored  bool
	IncludeDisabled bool
}

// ListEntries returns current entries matching filters.
func (d *DB) ListEntries(ctx context.Context, opts ListOptions) ([]Entry, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if opts.Platform != "" && opts.Platform != "all" {
		// splitPlatformList lowercases the filter so users can type any casing.
		// The stored column must be lowered to match, otherwise a program
		// recorded with uppercase characters is silently invisible to the filter.
		platformList := splitPlatformList(opts.Platform)
		if len(platformList) == 1 {
			where += fmt.Sprintf(" AND lower(p.platform) = $%d", argIdx)
			args = append(args, platformList[0])
			argIdx++
		} else if len(platformList) > 1 {
			where += fmt.Sprintf(" AND lower(p.platform) = ANY($%d)", argIdx)
			args = append(args, pgtype.FlatArray[string](platformList))
			argIdx++
		}
	}
	if opts.ProgramFilter != "" {
		filter := "%" + escapeLikePattern(opts.ProgramFilter) + "%"
		where += fmt.Sprintf(" AND (lower(p.url) LIKE lower($%d) ESCAPE '\\' OR lower(p.handle) LIKE lower($%d) ESCAPE '\\')", argIdx, argIdx)
		args = append(args, filter)
		argIdx++
	}
	if !opts.IncludeOOS {
		where += " AND COALESCE(a.in_scope, t.in_scope) = 1"
	}
	if !opts.IncludeIgnored {
		where += " AND p.is_ignored = 0"
	}
	if !opts.IncludeDisabled {
		where += " AND p.disabled = 0"
	}
	if !opts.Since.IsZero() {
		// Filter on the raw target's last_seen_at. Unchanged polls only bump
		// targets_raw; COALESCE(a.last_seen_at, ...) preferred a stale AI
		// timestamp and dropped every AI-joined row from --since listings.
		where += fmt.Sprintf(" AND t.last_seen_at >= $%d", argIdx)
		args = append(args, opts.Since.UTC())
	}

	//nolint:gosec // where clause is built from hardcoded strings with parameterised values only
	query := fmt.Sprintf(`
		SELECT 
			p.url,
			p.platform,
			p.handle,
			p.disabled,
			p.is_ignored,
			t.target,
			t.category,
			t.description,
			t.in_scope,
			t.is_bbp,
			a.target_ai_normalized,
			a.category,
			a.in_scope,
			a.id
		FROM targets_raw t
		JOIN programs p ON t.program_id = p.id
		LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
		%s
		ORDER BY p.url, COALESCE(a.target_ai_normalized, t.target)
	`, where)

	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var (
			programURL   string
			platform     string
			handle       string
			disabledInt  int
			ignoredInt   int
			rawTarget    string
			baseCategory string
			descNS       sql.NullString
			baseInScope  int
			isBBPInt     int
			aiTargetNS   sql.NullString
			aiCategoryNS sql.NullString
			aiInScopeNS  sql.NullInt64
			aiIDNS       sql.NullInt64
		)
		if err := rows.Scan(
			&programURL,
			&platform,
			&handle,
			&disabledInt,
			&ignoredInt,
			&rawTarget,
			&baseCategory,
			&descNS,
			&baseInScope,
			&isBBPInt,
			&aiTargetNS,
			&aiCategoryNS,
			&aiInScopeNS,
			&aiIDNS,
		); err != nil {
			return nil, err
		}

		baseNorm := NormalizeTarget(rawTarget)
		entry := Entry{
			ProgramURL:           programURL,
			Platform:             platform,
			Handle:               handle,
			BaseTargetRaw:        rawTarget,
			BaseTargetNormalized: baseNorm,
			TargetRaw:            rawTarget,
			Description:          descNS.String,
			IsBBP:                isBBPInt == 1,
			Category:             baseCategory,
			BaseCategory:         baseCategory,
			Source:               "raw",
			Disabled:             disabledInt == 1,
			IsIgnored:            ignoredInt == 1,
		}

		if aiIDNS.Valid {
			entry.Source = "ai"
			if aiTargetNS.Valid && aiTargetNS.String != "" {
				entry.TargetNormalized = aiTargetNS.String
			} else {
				entry.TargetNormalized = baseNorm
			}
			if aiCategoryNS.Valid {
				cat := strings.ToLower(strings.TrimSpace(aiCategoryNS.String))
				if scope.IsUnifiedCategory(cat) && !strings.EqualFold(cat, baseCategory) {
					entry.Category = cat
				}
			}
			if aiInScopeNS.Valid {
				entry.InScope = aiInScopeNS.Int64 == 1
			} else {
				entry.InScope = baseInScope == 1
			}
		} else {
			entry.TargetNormalized = baseNorm
			entry.InScope = baseInScope == 1
		}

		out = append(out, entry)
	}
	return out, rows.Err()
}

// ListRecentChanges returns the most recent N changes across all programs.
func (d *DB) ListRecentChanges(ctx context.Context, limit int) ([]Change, error) {
	if limit <= 0 {
		limit = 50
	}
	q := "SELECT occurred_at, program_url, platform, handle, target_normalized, target_raw, target_ai_normalized, category, in_scope, is_bbp, change_type FROM scope_changes ORDER BY occurred_at DESC LIMIT $1"
	rows, err := d.sql.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := []Change{}
	for rows.Next() {
		var c Change
		var inScopeInt, isBBPInt int
		if err := rows.Scan(&c.OccurredAt, &c.ProgramURL, &c.Platform, &c.Handle, &c.TargetNormalized, &c.TargetRaw, &c.TargetAINormalized, &c.Category, &inScopeInt, &isBBPInt, &c.ChangeType); err != nil {
			return nil, fmt.Errorf("scanning change row: %w", err)
		}
		c.InScope = inScopeInt == 1
		c.IsBBP = isBBPInt == 1
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

// GetChangesBetween returns changes between two timestamps
func (d *DB) GetChangesBetween(ctx context.Context, from, to time.Time, programURL string) ([]Change, error) {
	query := "SELECT occurred_at, program_url, platform, handle, target_normalized, target_raw, target_ai_normalized, category, in_scope, is_bbp, change_type FROM scope_changes WHERE occurred_at >= $1 AND occurred_at <= $2"
	args := []interface{}{from.UTC(), to.UTC()}

	if programURL != "" {
		query += " AND (lower(program_url) LIKE lower($3) ESCAPE '\\' OR lower(handle) LIKE lower($3) ESCAPE '\\')"
		args = append(args, "%"+escapeLikePattern(programURL)+"%")
	}

	query += " ORDER BY occurred_at ASC"

	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := []Change{}
	for rows.Next() {
		var c Change
		var inScopeInt, isBBPInt int
		if err := rows.Scan(&c.OccurredAt, &c.ProgramURL, &c.Platform, &c.Handle, &c.TargetNormalized, &c.TargetRaw, &c.TargetAINormalized, &c.Category, &inScopeInt, &isBBPInt, &c.ChangeType); err != nil {
			return nil, fmt.Errorf("scanning change row: %w", err)
		}
		c.InScope = inScopeInt == 1
		c.IsBBP = isBBPInt == 1
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

type PlatformStats struct {
	Platform        string
	ProgramCount    int
	InScopeCount    int
	OutOfScopeCount int
}

func (d *DB) GetStats(ctx context.Context) ([]PlatformStats, error) {
	// Count each raw target once using its own in_scope bit. AI variants used
	// to be joined and DISTINCT ON (t.id) ORDER BY a.id, which picked an
	// arbitrary override and could flip an in-scope target to out-of-scope.
	// The join to programs is a LEFT JOIN so platforms whose programs have no
	// targets yet still report a program count.
	query := `
		WITH effective_targets AS (
			SELECT t.id, t.program_id, t.in_scope
			FROM targets_raw t
		)
		SELECT
			p.platform,
			COUNT(DISTINCT p.id),
			COALESCE(SUM(CASE WHEN et.in_scope = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN et.in_scope = 0 THEN 1 ELSE 0 END), 0)
		FROM
			programs p LEFT JOIN effective_targets et ON p.id = et.program_id
		WHERE
			p.is_ignored = 0 AND p.disabled = 0
		GROUP BY
			p.platform
		ORDER BY
			p.platform;
	`
	rows, err := d.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PlatformStats
	for rows.Next() {
		var s PlatformStats
		if err := rows.Scan(&s.Platform, &s.ProgramCount, &s.InScopeCount, &s.OutOfScopeCount); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

func (d *DB) SearchTargets(ctx context.Context, searchTerm string) ([]Entry, error) {
	likeQuery := "%" + escapeLikePattern(searchTerm) + "%"

	query := `
		SELECT 
			p.url,
			p.platform,
			p.handle,
			t.target,
			t.category,
			t.description,
			t.in_scope,
			t.is_bbp,
			a.target_ai_normalized,
			a.category,
			a.in_scope,
			a.id,
			'current' AS source
		FROM targets_raw t
		JOIN programs p ON t.program_id = p.id
		LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
		WHERE p.is_ignored = 0 AND p.disabled = 0 AND (
			lower(COALESCE(a.target_ai_normalized, t.target)) LIKE lower($1) ESCAPE '\' OR
			lower(t.target) LIKE lower($1) ESCAPE '\' OR
			lower(t.description) LIKE lower($2) ESCAPE '\' OR
			lower(p.url) LIKE lower($3) ESCAPE '\'
		)

		UNION

		SELECT 
			c.program_url,
			c.platform,
			c.handle,
			c.target_raw,
			c.category,
			NULL as description,
			CASE WHEN c.in_scope = 1 THEN 1 ELSE 0 END as in_scope,
			CASE WHEN c.is_bbp = 1 THEN 1 ELSE 0 END as is_bbp,
			c.target_ai_normalized,
			c.category,
			CASE WHEN c.in_scope = 1 THEN 1 ELSE 0 END as ai_in_scope,
			NULL as ai_id,
			'historical' as source
		FROM scope_changes c
		LEFT JOIN programs p3 ON rtrim(p3.url, '/') = rtrim(c.program_url, '/')
		WHERE (lower(c.target_normalized) LIKE lower($4) ESCAPE '\' OR lower(c.target_ai_normalized) LIKE lower($5) ESCAPE '\' OR lower(c.target_raw) LIKE lower($4) ESCAPE '\' OR lower(c.program_url) LIKE lower($6) ESCAPE '\')
		AND (p3.id IS NULL OR (p3.is_ignored = 0 AND p3.disabled = 0))
		AND NOT EXISTS (
			SELECT 1 FROM targets_raw t2
			JOIN programs p2 ON t2.program_id = p2.id
			WHERE rtrim(p2.url, '/') = rtrim(c.program_url, '/')
			AND t2.target = c.target_raw
			AND t2.category = c.category
		);
	`

	rows, err := d.sql.QueryContext(ctx, query,
		likeQuery, likeQuery, likeQuery,
		likeQuery, likeQuery, likeQuery,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	seen := make(map[string]int)

	for rows.Next() {
		var (
			programURL   string
			platform     string
			handle       string
			rawTarget    string
			baseCategory string
			descNS       sql.NullString
			baseInScope  int
			isBBPInt     int
			aiTargetNS   sql.NullString
			aiCategoryNS sql.NullString
			aiInScopeNS  sql.NullInt64
			aiIDNS       sql.NullInt64
			source       string
		)
		if err := rows.Scan(
			&programURL,
			&platform,
			&handle,
			&rawTarget,
			&baseCategory,
			&descNS,
			&baseInScope,
			&isBBPInt,
			&aiTargetNS,
			&aiCategoryNS,
			&aiInScopeNS,
			&aiIDNS,
			&source,
		); err != nil {
			return nil, err
		}

		baseNorm := NormalizeTarget(rawTarget)
		entry := Entry{
			ProgramURL:           programURL,
			Platform:             platform,
			Handle:               handle,
			Category:             baseCategory,
			Description:          descNS.String,
			BaseTargetRaw:        rawTarget,
			BaseTargetNormalized: baseNorm,
			TargetRaw:            rawTarget,
			IsBBP:                isBBPInt == 1,
		}

		if source == "historical" {
			if aiTargetNS.Valid && aiTargetNS.String != "" {
				entry.TargetNormalized = aiTargetNS.String
			} else {
				entry.TargetNormalized = baseNorm
			}
			if aiCategoryNS.Valid && scope.IsUnifiedCategory(strings.ToLower(strings.TrimSpace(aiCategoryNS.String))) {
				entry.Category = strings.ToLower(strings.TrimSpace(aiCategoryNS.String))
			}
			entry.InScope = baseInScope == 1
			entry.IsHistorical = true
		} else {
			entry.IsHistorical = false
			if aiIDNS.Valid {
				if aiTargetNS.Valid && aiTargetNS.String != "" {
					entry.TargetNormalized = aiTargetNS.String
				} else {
					entry.TargetNormalized = baseNorm
				}
				if aiCategoryNS.Valid {
					cat := strings.ToLower(strings.TrimSpace(aiCategoryNS.String))
					if scope.IsUnifiedCategory(cat) && !strings.EqualFold(cat, baseCategory) {
						entry.Category = cat
					}
				}
				if aiInScopeNS.Valid {
					entry.InScope = aiInScopeNS.Int64 == 1
				} else {
					entry.InScope = baseInScope == 1
				}
			} else {
				entry.TargetNormalized = baseNorm
				entry.InScope = baseInScope == 1
			}
		}

		if source == "historical" {
			entry.Source = "historical"
		} else if aiIDNS.Valid {
			entry.Source = "ai"
		} else {
			entry.Source = "raw"
		}

		key := fmt.Sprintf("%s|%s|%s|%s", entry.ProgramURL, entry.TargetNormalized, entry.BaseTargetNormalized, entry.Category)
		if idx, ok := seen[key]; ok {
			// Always prefer current entries over historical ones
			if out[idx].IsHistorical && !entry.IsHistorical {
				out[idx] = entry
			}
			// If we already have a current entry, skip adding a historical one
		} else {
			out = append(out, entry)
			seen[key] = len(out) - 1
		}
	}
	return out, rows.Err()
}
