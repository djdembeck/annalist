package db

import (
	"database/sql"
	"fmt"
)

// Settings is the global (singleton) release-note configuration row.
// Temperature is nil when unset so downstream can distinguish inherit from a
// literal zero.
type Settings struct {
	Tone         string
	Instructions string
	Model        string
	Temperature  *float64
	CommitTypes  string
	// BaseURL and APIKey override the env/config LLM endpoint when non-empty;
	// empty means "inherit from env/config".
	BaseURL string
	APIKey  string
	// Mode is "lite" or "deep". Empty means "inherit" on a repo row, "lite"
	// default on the global row (resolved downstream).
	Mode          string
	MaxTokens     int    // 0 = unset → inherit from config/env
	ThinkingLevel string // "off" | "low" | "medium" | "high"; empty = inherit from config/env
}

// RepoSetting is a per-repo override of the global settings. Empty string /
// nil temperature mean "inherit from global".
type RepoSetting struct {
	Platform     string
	Owner        string
	Repo         string
	Trigger      string
	Enabled      bool
	Tone         string
	Instructions string
	Model        string
	Temperature  *float64
	CommitTypes  string
	// Mode is "lite" or "deep". Empty means "inherit" on a repo row, "lite"
	// default on the global row (resolved downstream).
	Mode          string
	MaxTokens     int    // 0 = unset → inherit from config/env
	ThinkingLevel string // "off" | "low" | "medium" | "high"; empty = inherit from global
}

// GeneratedNote records a previously generated release note for idempotency.
type GeneratedNote struct {
	Platform  string
	Owner     string
	Repo      string
	ReleaseID string
	Tag       string
	Notes     string
	CreatedAt string
}

// migrations are applied in order at Store construction.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS settings (
		id INTEGER PRIMARY KEY CHECK(id = 1),
		tone TEXT,
		instructions TEXT,
		model TEXT,
		temperature REAL,
		commit_types TEXT,
		base_url TEXT,
		api_key TEXT,
		mode TEXT,
		max_tokens INTEGER,
		thinking_level TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS repo_settings (
		platform TEXT NOT NULL,
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		tone TEXT,
		instructions TEXT,
		model TEXT,
		temperature REAL,
		trigger TEXT NOT NULL DEFAULT 'auto',
		commit_types TEXT,
		mode TEXT,
		max_tokens INTEGER,
		thinking_level TEXT,
		PRIMARY KEY (platform, owner, repo)
	)`,
	`CREATE TABLE IF NOT EXISTS generated_notes (
		platform TEXT NOT NULL,
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		release_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		notes TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (release_id)
	)`,
}

func (s *Store) migrate() error {
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("running migration: %w", err)
		}
	}
	// Backfill columns added after the table was first created.
	// CREATE TABLE IF NOT EXISTS is a no-op on existing tables, so the column
	// wouldn't appear for databases created before this migration.
	for _, col := range []struct {
		table, name, ddl string
	}{
		{"settings", "commit_types", "ALTER TABLE settings ADD COLUMN commit_types TEXT"},
		{"repo_settings", "commit_types", "ALTER TABLE repo_settings ADD COLUMN commit_types TEXT"},
		{"settings", "base_url", "ALTER TABLE settings ADD COLUMN base_url TEXT"},
		{"settings", "api_key", "ALTER TABLE settings ADD COLUMN api_key TEXT"},
		{"settings", "mode", "ALTER TABLE settings ADD COLUMN mode TEXT"},
		{"repo_settings", "mode", "ALTER TABLE repo_settings ADD COLUMN mode TEXT"},
		{"settings", "max_tokens", "ALTER TABLE settings ADD COLUMN max_tokens INTEGER"},
		{"settings", "thinking_level", "ALTER TABLE settings ADD COLUMN thinking_level TEXT"},
		{"repo_settings", "max_tokens", "ALTER TABLE repo_settings ADD COLUMN max_tokens INTEGER"},
		{"repo_settings", "thinking_level", "ALTER TABLE repo_settings ADD COLUMN thinking_level TEXT"},
	} {
		if err := s.ensureColumn(col.table, col.name, col.ddl); err != nil {
			return fmt.Errorf("ensureColumn(%s.%s): %w", col.table, col.name, err)
		}
	}
	return nil
}

// ensureColumn adds a column to a table if it does not already exist.
// Idempotent: safe to call on every startup.
func (s *Store) ensureColumn(table, column, ddl string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(ddl)
	return err
}

// strOrNil maps an empty string to NULL so absent values round-trip as
// "inherit" rather than as an explicit empty override.
func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// GetSettings returns the singleton global settings row. When the row is
// absent it returns a zero Settings (empty tone/instructions/model, nil
// temperature) with a nil error.
func (s *Store) GetSettings() (Settings, error) {
	var out Settings
	var tone, instructions, model, commitTypes, baseURL, apiKey, mode sql.NullString
	var temp sql.NullFloat64
	var maxTokens sql.NullInt64
	var thinkingLevel sql.NullString

	err := s.db.QueryRow(
		`SELECT tone, instructions, model, temperature, commit_types, base_url, api_key, mode, max_tokens, thinking_level
		 FROM settings WHERE id = 1`,
	).Scan(&tone, &instructions, &model, &temp, &commitTypes, &baseURL, &apiKey, &mode, &maxTokens, &thinkingLevel)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return out, err
	}

	out.Tone = tone.String
	out.Instructions = instructions.String
	out.Model = model.String
	out.CommitTypes = commitTypes.String
	out.BaseURL = baseURL.String
	out.APIKey = apiKey.String
	out.Mode = mode.String
	out.MaxTokens = int(maxTokens.Int64) // invalid/NULL → 0 = unset
	out.ThinkingLevel = thinkingLevel.String
	if temp.Valid {
		v := temp.Float64
		out.Temperature = &v
	}
	return out, nil
}

// UpsertSettings writes the singleton settings row (id=1), replacing any
// existing values.
func (s *Store) UpsertSettings(settings Settings) error {
	var temp any
	if settings.Temperature != nil {
		temp = *settings.Temperature
	}

	_, err := s.db.Exec(
		`INSERT INTO settings (id, tone, instructions, model, temperature, commit_types, base_url, api_key, mode, max_tokens, thinking_level)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			tone = excluded.tone,
			instructions = excluded.instructions,
			model = excluded.model,
			temperature = excluded.temperature,
			commit_types = excluded.commit_types,
			base_url = excluded.base_url,
			api_key = excluded.api_key,
			mode = excluded.mode,
			max_tokens = excluded.max_tokens,
			thinking_level = excluded.thinking_level`,
		settings.Tone, settings.Instructions, settings.Model, temp, strOrNil(settings.CommitTypes),
		strOrNil(settings.BaseURL), strOrNil(settings.APIKey), strOrNil(settings.Mode),
		settings.MaxTokens, strOrNil(settings.ThinkingLevel),
	)
	return err
}

// ListRepoSettings returns every repo_settings row.
func (s *Store) ListRepoSettings() ([]RepoSetting, error) {
	rows, err := s.db.Query(
		`SELECT platform, owner, repo, enabled, tone, instructions, model, temperature, trigger, commit_types, mode, max_tokens, thinking_level
		 FROM repo_settings`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RepoSetting
	for rows.Next() {
		var r RepoSetting
		if err := scanRepoSetting(rows, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRepoSettings returns the settings row for a repo, or (nil, nil) when no
// row exists (downstream treats a missing row as all-inherit / enabled).
func (s *Store) GetRepoSettings(platform, owner, repo string) (*RepoSetting, error) {
	row := s.db.QueryRow(
		`SELECT platform, owner, repo, enabled, tone, instructions, model, temperature, trigger, commit_types, mode, max_tokens, thinking_level
		 FROM repo_settings WHERE platform = ? AND owner = ? AND repo = ?`,
		platform, owner, repo,
	)

	var r RepoSetting
	err := scanRepoSetting(row, &r)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// UpsertRepoSettings inserts or updates the settings row keyed by
// (platform, owner, repo). Empty tone/instructions/model are stored as NULL to
// signal "inherit"; a nil Temperature is stored as NULL.
func (s *Store) UpsertRepoSettings(r RepoSetting) error {
	var temp any
	if r.Temperature != nil {
		temp = *r.Temperature
	}
	enabled := 0
	if r.Enabled {
		enabled = 1
	}

	_, err := s.db.Exec(
		`INSERT INTO repo_settings (platform, owner, repo, enabled, tone, instructions, model, temperature, trigger, commit_types, mode, max_tokens, thinking_level)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform, owner, repo) DO UPDATE SET
			enabled = excluded.enabled,
			tone = excluded.tone,
			instructions = excluded.instructions,
			model = excluded.model,
			temperature = excluded.temperature,
			trigger = excluded.trigger,
			commit_types = excluded.commit_types,
			mode = excluded.mode,
			max_tokens = excluded.max_tokens,
			thinking_level = excluded.thinking_level`,
		r.Platform, r.Owner, r.Repo, enabled,
		strOrNil(r.Tone), strOrNil(r.Instructions), strOrNil(r.Model),
		temp, r.Trigger, strOrNil(r.CommitTypes), strOrNil(r.Mode),
		r.MaxTokens, strOrNil(r.ThinkingLevel),
	)
	return err
}

// MarkGenerated records a generated release note, overwriting any prior note
// for the same release_id (idempotent for retries and force regeneration).
func (s *Store) MarkGenerated(platform, owner, repo, releaseID, tag, notes string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO generated_notes (platform, owner, repo, release_id, tag, notes)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		platform, owner, repo, releaseID, tag, notes,
	)
	return err
}

// GetGenerated returns the stored generated note for a release, or
// (nil, nil) when none exists.
func (s *Store) GetGenerated(platform, releaseID string) (*GeneratedNote, error) {
	var n GeneratedNote
	err := s.db.QueryRow(
		`SELECT platform, owner, repo, release_id, tag, notes, created_at
		 FROM generated_notes WHERE release_id = ?`,
		releaseID,
	).Scan(&n.Platform, &n.Owner, &n.Repo, &n.ReleaseID, &n.Tag, &n.Notes, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRepoSetting(row rowScanner, r *RepoSetting) error {
	var enabled int
	var tone, instructions, model, trigger, commitTypes, mode sql.NullString
	var temp sql.NullFloat64
	var maxTokens sql.NullInt64
	var thinkingLevel sql.NullString

	err := row.Scan(&r.Platform, &r.Owner, &r.Repo, &enabled, &tone, &instructions, &model, &temp, &trigger, &commitTypes, &mode, &maxTokens, &thinkingLevel)
	if err != nil {
		return err
	}

	r.Enabled = enabled != 0
	r.Tone = tone.String
	r.Instructions = instructions.String
	r.Model = model.String
	r.Trigger = trigger.String
	r.CommitTypes = commitTypes.String
	r.Mode = mode.String
	r.MaxTokens = int(maxTokens.Int64)
	r.ThinkingLevel = thinkingLevel.String
	if trigger.String == "" {
		r.Trigger = "auto"
	}
	if temp.Valid {
		v := temp.Float64
		r.Temperature = &v
	}
	return nil
}
