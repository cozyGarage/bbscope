package storage

import "time"

// Entry represents a single normalized scope entry for a program.
//
// The json tags matter for `db export`/`db import`: without them the encoder
// emitted Go field names in PascalCase while `db print --format json` emitted
// snake_case, so the two JSON shapes this tool produces disagreed with each
// other and neither round-tripped cleanly through external tooling.
type Entry struct {
	// Program info
	ProgramURL string `json:"program_url"`
	Platform   string `json:"platform"`
	Handle     string `json:"handle"`

	// Display target info (variant or raw)
	TargetNormalized string `json:"target_normalized"`
	TargetRaw        string `json:"target_raw"`

	// Base target info (always deterministic/raw)
	BaseTargetNormalized string `json:"base_target_normalized,omitempty"`
	BaseTargetRaw        string `json:"base_target_raw,omitempty"`

	Category     string `json:"category"`
	BaseCategory string `json:"base_category,omitempty"`
	Description  string `json:"description,omitempty"`
	InScope      bool   `json:"in_scope"`
	IsBBP        bool   `json:"is_bbp"`
	IsHistorical bool   `json:"is_historical,omitempty"`
	Source       string `json:"source,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
	IsIgnored    bool   `json:"is_ignored,omitempty"`
}

// Change captures a single change event for auditing or printing.
type Change struct {
	OccurredAt         time.Time
	ProgramURL         string
	Platform           string
	Handle             string
	TargetNormalized   string
	TargetRaw          string
	TargetAINormalized string
	Category           string
	InScope            bool
	IsBBP              bool
	ChangeType         string
}

// UpsertEntry represents the raw scope item along with its variants.
type UpsertEntry struct {
	ProgramURL       string
	Platform         string
	Handle           string
	TargetNormalized string
	TargetRaw        string
	Category         string
	Description      string
	InScope          bool
	IsBBP            bool
	Variants         []EntryVariant
}

// EntryVariant represents a derived/expanded target tied to a raw entry.
type EntryVariant struct {
	AINormalized string
	HasInScope   bool
	InScope      bool
	HasCategory  bool
	Category     string
}

// TargetItem is a light wrapper for building entries.
type TargetItem struct {
	URI         string
	Category    string
	Description string
	InScope     bool
	IsBBP       bool
	Variants    []TargetVariant
}

// TargetVariant captures a requested expansion for a target.
type TargetVariant struct {
	Value       string
	HasInScope  bool
	InScope     bool
	HasCategory bool
	Category    string
}
