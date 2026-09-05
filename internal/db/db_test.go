package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewCreatesDatabaseFile(t *testing.T) {
	dataDir := t.TempDir()
	s, err := New(dataDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "app.db")); err != nil {
		t.Errorf("app.db not created: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// Double close is safe for *sql.DB.
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestNewCreatesNestedDataDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "nested", "deeper")
	s, err := New(dataDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Close()
	if _, err := os.Stat(dataDir); err != nil {
		t.Errorf("nested data dir not created: %v", err)
	}
}

func TestNewInvalidDataDir(t *testing.T) {
	// Put a regular file where a directory is required, so MkdirAll fails.
	file := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(file, "sub")

	if _, err := New(dataDir); err == nil {
		t.Fatal("New() should error when dataDir cannot be created")
	}
}

func TestNewReadOnlyDataDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only dirs do not block root")
	}
	dataDir := t.TempDir()
	if err := os.Chmod(dataDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dataDir, 0o700) })

	if _, err := New(dataDir); err == nil {
		t.Fatal("New() should error when app.db cannot be created in a read-only dir")
	}
}

func TestSettingsCRUD(t *testing.T) {
	s := newTestStore(t)

	t.Run("default absent row", func(t *testing.T) {
		got, err := s.GetSettings()
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if got.Tone != "" || got.Instructions != "" || got.Model != "" {
			t.Errorf("expected empty defaults, got %+v", got)
		}
		if got.Temperature != nil {
			t.Errorf("expected nil temperature, got %v", *got.Temperature)
		}
	})

	temp := 0.5
	if err := s.UpsertSettings(Settings{
		Tone: "custom", Instructions: "keep terse", Model: "m1",
		Temperature: &temp,
	}); err != nil {
		t.Fatalf("UpsertSettings: %v", err)
	}

	got, err := s.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.Tone != "custom" || got.Instructions != "keep terse" || got.Model != "m1" {
		t.Errorf("settings mismatch: %+v", got)
	}
	if got.Temperature == nil || *got.Temperature != 0.5 {
		t.Errorf("temperature = %v, want 0.5", got.Temperature)
	}

	// Upsert replaces values (singleton row id=1).
	if err := s.UpsertSettings(Settings{Tone: "replaced"}); err != nil {
		t.Fatalf("UpsertSettings replace: %v", err)
	}
	got, err = s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.Tone != "replaced" || got.Model != "" || got.Temperature != nil {
		t.Errorf("replace semantics wrong: %+v", got)
	}

	t.Run("base url and api key round-trip", func(t *testing.T) {
		if err := s.UpsertSettings(Settings{BaseURL: "https://b", APIKey: "k"}); err != nil {
			t.Fatalf("UpsertSettings: %v", err)
		}
		got, err := s.GetSettings()
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if got.BaseURL != "https://b" || got.APIKey != "k" {
			t.Errorf("endpoint = (%q, %q), want (https://b, k)", got.BaseURL, got.APIKey)
		}
		// Empty round-trips as empty (NULL -> ""), the inherit signal.
		if err := s.UpsertSettings(Settings{BaseURL: "", APIKey: ""}); err != nil {
			t.Fatalf("UpsertSettings clear: %v", err)
		}
		got, err = s.GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		if got.BaseURL != "" || got.APIKey != "" {
			t.Errorf("cleared endpoint = (%q, %q), want empty", got.BaseURL, got.APIKey)
		}
	})

	// Mode round-trips as a plain string ("" on the global row).
	if err := s.UpsertSettings(Settings{Mode: "deep"}); err != nil {
		t.Fatalf("UpsertSettings deep mode: %v", err)
	}
	got, err = s.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings mode: %v", err)
	}
	if got.Mode != "deep" {
		t.Errorf("mode = %q, want deep", got.Mode)
	}
	if err := s.UpsertSettings(Settings{Mode: ""}); err != nil {
		t.Fatalf("UpsertSettings empty mode: %v", err)
	}
	got, err = s.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings empty mode: %v", err)
	}
	if got.Mode != "" {
		t.Errorf("mode = %q, want empty", got.Mode)
	}

	// MaxTokens/ThinkingLevel round-trip; zero/empty read back unset.
	if err := s.UpsertSettings(Settings{MaxTokens: 4096, ThinkingLevel: "high"}); err != nil {
		t.Fatalf("UpsertSettings knobs: %v", err)
	}
	got, err = s.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings knobs: %v", err)
	}
	if got.MaxTokens != 4096 || got.ThinkingLevel != "high" {
		t.Errorf("knobs = (%d, %q), want (4096, high)", got.MaxTokens, got.ThinkingLevel)
	}
	if err := s.UpsertSettings(Settings{}); err != nil {
		t.Fatalf("UpsertSettings clear knobs: %v", err)
	}
	got, err = s.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings clear knobs: %v", err)
	}
	if got.MaxTokens != 0 || got.ThinkingLevel != "" {
		t.Errorf("cleared knobs = (%d, %q), want (0, empty)", got.MaxTokens, got.ThinkingLevel)
	}
}

func TestRepoSettingsCRUD(t *testing.T) {
	s := newTestStore(t)

	t.Run("missing row returns nil,nil", func(t *testing.T) {
		got, err := s.GetRepoSettings("github", "o", "nope")
		if err != nil {
			t.Fatalf("GetRepoSettings: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for missing row, got %+v", got)
		}
	})

	t.Run("empty list initially", func(t *testing.T) {
		rows, err := s.ListRepoSettings()
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Errorf("expected empty list, got %d", len(rows))
		}
	})

	temp := 0.8
	if err := s.UpsertRepoSettings(RepoSetting{
		Platform: "github", Owner: "djdembeck", Repo: "annalist",
		Enabled: true, Tone: "repo-tone", Instructions: "", Model: "repo-model",
		Temperature: &temp, Trigger: "manual",
	}); err != nil {
		t.Fatalf("UpsertRepoSettings: %v", err)
	}
	// Second row with empty strings + default trigger to exercise NULL/inherit + default.
	if err := s.UpsertRepoSettings(RepoSetting{
		Platform: "forgejo", Owner: "o", Repo: "r2", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := s.ListRepoSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListRepoSettings len = %d, want 2", len(rows))
	}

	got, err := s.GetRepoSettings("github", "djdembeck", "annalist")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected row")
	}
	if got.Platform != "github" || got.Owner != "djdembeck" || got.Repo != "annalist" {
		t.Errorf("key mismatch: %+v", got)
	}
	if got.Trigger != "manual" {
		t.Errorf("trigger = %q, want manual", got.Trigger)
	}
	if got.Temperature == nil || *got.Temperature != 0.8 {
		t.Errorf("temperature = %v, want 0.8", got.Temperature)
	}
	if !got.Enabled {
		t.Error("enabled should be true")
	}

	got2, err := s.GetRepoSettings("forgejo", "o", "r2")
	if err != nil {
		t.Fatal(err)
	}
	// Empty strings stored as NULL must read back as empty (inherit), and the
	// default trigger 'auto' applies when the stored value is empty.
	if got2.Tone != "" || got2.Model != "" || got2.Temperature != nil {
		t.Errorf("inherit semantics wrong: %+v", got2)
	}
	if got2.Trigger != "auto" {
		t.Errorf("default trigger = %q, want auto", got2.Trigger)
	}

	// Upsert on existing key updates rather than duplicating.
	if err := s.UpsertRepoSettings(RepoSetting{
		Platform: "github", Owner: "djdembeck", Repo: "annalist",
		Enabled: false, Tone: "updated", Trigger: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ListRepoSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("upsert should not add rows; len = %d", len(rows))
	}
	got, err = s.GetRepoSettings("github", "djdembeck", "annalist")
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.Tone != "updated" {
		t.Errorf("update semantics wrong: %+v", got)
	}
	if got.Trigger != "manual" {
		t.Errorf("trigger should persist across update, got %q", got.Trigger)
	}

	// Mode: explicit "deep" round-trips; empty (NULL) reads back as "" (inherit).
	if err := s.UpsertRepoSettings(RepoSetting{
		Platform: "forgejo", Owner: "o", Repo: "r2", Enabled: true, Mode: "deep",
	}); err != nil {
		t.Fatal(err)
	}
	got2, err = s.GetRepoSettings("forgejo", "o", "r2")
	if err != nil {
		t.Fatal(err)
	}
	if got2 == nil || got2.Mode != "deep" {
		t.Errorf("mode = %+v, want deep", got2)
	}
	if err := s.UpsertRepoSettings(RepoSetting{
		Platform: "forgejo", Owner: "o", Repo: "r2", Enabled: true, Mode: "",
	}); err != nil {
		t.Fatal(err)
	}
	got2, err = s.GetRepoSettings("forgejo", "o", "r2")
	if err != nil {
		t.Fatal(err)
	}
	if got2 == nil || got2.Mode != "" {
		t.Errorf("mode = %+v, want empty (inherit)", got2)
	}

	// MaxTokens/ThinkingLevel round-trip; zero/empty read back unset (inherit).
	if err := s.UpsertRepoSettings(RepoSetting{
		Platform: "forgejo", Owner: "o", Repo: "r2", Enabled: true,
		MaxTokens: 4096, ThinkingLevel: "high",
	}); err != nil {
		t.Fatal(err)
	}
	got2, err = s.GetRepoSettings("forgejo", "o", "r2")
	if err != nil {
		t.Fatal(err)
	}
	if got2 == nil || got2.MaxTokens != 4096 || got2.ThinkingLevel != "high" {
		t.Errorf("knobs = %+v, want (4096, high)", got2)
	}
	if err := s.UpsertRepoSettings(RepoSetting{
		Platform: "forgejo", Owner: "o", Repo: "r2", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	got2, err = s.GetRepoSettings("forgejo", "o", "r2")
	if err != nil {
		t.Fatal(err)
	}
	if got2 == nil || got2.MaxTokens != 0 || got2.ThinkingLevel != "" {
		t.Errorf("cleared knobs = %+v, want unset", got2)
	}
}

// TestOpenLegacySchemaVerifies ensureColumn backfills commit_types/mode on a
// database created before those columns existed: seeded rows survive the
// migration with empty values, and reopening is idempotent.
func TestOpenLegacySchema(t *testing.T) {
	dataDir := t.TempDir()

	conn, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := `
		CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK(id = 1),
			tone TEXT,
			instructions TEXT,
			model TEXT,
			temperature REAL
		);
		CREATE TABLE IF NOT EXISTS repo_settings (
			platform TEXT NOT NULL,
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			tone TEXT,
			instructions TEXT,
			model TEXT,
			temperature REAL,
			trigger TEXT NOT NULL DEFAULT 'auto',
			PRIMARY KEY (platform, owner, repo)
		);`
	if _, err := conn.Exec(legacy); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO settings (id, tone, model) VALUES (1, 'seed-tone', 'seed-model')`); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO repo_settings (platform, owner, repo, enabled, tone) VALUES ('github', 'o', 'r', 1, 'seed-repo-tone')`); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	// First open: migration must add the missing columns without losing data.
	s, err := New(dataDir)
	if err != nil {
		t.Fatalf("New on legacy db: %v", err)
	}
	got, err := s.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.Tone != "seed-tone" || got.Model != "seed-model" {
		t.Errorf("seeded settings lost: %+v", got)
	}
	if got.CommitTypes != "" || got.Mode != "" {
		t.Errorf("backfilled columns should be empty, got commit_types=%q mode=%q", got.CommitTypes, got.Mode)
	}
	if got.MaxTokens != 0 || got.ThinkingLevel != "" {
		t.Errorf("backfilled settings knobs should be unset, got (%d, %q)", got.MaxTokens, got.ThinkingLevel)
	}
	row, err := s.GetRepoSettings("github", "o", "r")
	if err != nil {
		t.Fatalf("GetRepoSettings: %v", err)
	}
	if row == nil {
		t.Fatal("expected seeded repo row, got nil")
	}
	if row.Tone != "seed-repo-tone" || row.CommitTypes != "" || row.Mode != "" {
		t.Errorf("seeded repo row mismatch: %+v", row)
	}
	if row.MaxTokens != 0 || row.ThinkingLevel != "" {
		t.Errorf("seeded repo row knobs should be unset, got (%d, %q)", row.MaxTokens, row.ThinkingLevel)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen: ensureColumn must be idempotent on an already-migrated db.
	s2, err := New(dataDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err = s2.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings after reopen: %v", err)
	}
	if got.Tone != "seed-tone" || got.Model != "seed-model" || got.CommitTypes != "" || got.Mode != "" {
		t.Errorf("settings after reopen: %+v", got)
	}
}

func TestGeneratedNotesRoundTrip(t *testing.T) {
	s := newTestStore(t)

	t.Run("missing key returns nil,nil", func(t *testing.T) {
		got, err := s.GetGenerated("cache-key-absent")
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	if err := s.MarkGenerated(GeneratedNote{
		CacheKey: "key-1", Platform: "github", Owner: "djdembeck", Repo: "annalist",
		ReleaseID: "rel-1", FromTag: "v0.9.0", ToTag: "v1.0.0",
		Profile: "customer", DisplayVersion: "1.0", ConfigDigest: "d1",
		Notes: "Notes one",
	}); err != nil {
		t.Fatalf("MarkGenerated: %v", err)
	}

	got, err := s.GetGenerated("key-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected a generated note")
	}
	if got.ReleaseID != "rel-1" || got.ToTag != "v1.0.0" || got.Notes != "Notes one" {
		t.Errorf("note mismatch: %+v", got)
	}
	if got.Owner != "djdembeck" || got.Repo != "annalist" || got.Platform != "github" {
		t.Errorf("note key fields mismatch: %+v", got)
	}
	if got.FromTag != "v0.9.0" || got.Profile != "customer" || got.DisplayVersion != "1.0" || got.ConfigDigest != "d1" {
		t.Errorf("note contract fields mismatch: %+v", got)
	}
	if got.CreatedAt == "" {
		t.Error("created_at should be populated")
	}

	// MarkGenerated is idempotent per cache key: overwrites the prior note.
	if err := s.MarkGenerated(GeneratedNote{
		CacheKey: "key-1", Platform: "github", Owner: "djdembeck", Repo: "annalist",
		ReleaseID: "rel-1", ToTag: "v2.0.0",
		Profile: "customer", DisplayVersion: "2.0", ConfigDigest: "d2",
		Notes: "Notes two",
	}); err != nil {
		t.Fatalf("MarkGenerated overwrite: %v", err)
	}
	got, err = s.GetGenerated("key-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ToTag != "v2.0.0" || got.Notes != "Notes two" || got.ConfigDigest != "d2" {
		t.Errorf("overwrite semantics wrong: %+v", got)
	}
	// A different cache key for the same release coexists.
	if err := s.MarkGenerated(GeneratedNote{
		CacheKey: "key-maintainer", Platform: "github", Owner: "djdembeck", Repo: "annalist",
		ReleaseID: "rel-1", ToTag: "v2.0.0", Profile: "maintainer",
		DisplayVersion: "2.0", Notes: "Maintainer notes",
	}); err != nil {
		t.Fatalf("MarkGenerated second key: %v", err)
	}
	if got, err := s.GetGenerated("key-maintainer"); err != nil || got == nil || got.Profile != "maintainer" {
		t.Errorf("second profile key missing: %+v (err=%v)", got, err)
	}
	if got, err := s.GetGenerated("key-1"); err != nil || got == nil || got.Notes != "Notes two" {
		t.Errorf("first key clobbered by second: %+v (err=%v)", got, err)
	}
}

// TestOpenLegacyGeneratedNotes verifies the generated_notes upgrade: a
// database created with the legacy release-id-keyed schema migrates to the
// contract-keyed schema in one go — every legacy row is preserved with legacy
// metadata (derived legacy cache key, empty from_tag/profile/config_digest,
// tag as to_tag and display_version), reopening is a no-op, and new
// profile rows coexist without collision.
func TestOpenLegacyGeneratedNotes(t *testing.T) {
	dataDir := t.TempDir()

	conn, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := `CREATE TABLE IF NOT EXISTS generated_notes (
		platform TEXT NOT NULL,
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		release_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		notes TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (release_id)
	)`
	if _, err := conn.Exec(legacy); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO generated_notes (platform, owner, repo, release_id, tag, notes, created_at)
		VALUES ('github', 'o1', 'r1', 'rel-a', 'v1.0.0', 'Note A', '2025-01-01 00:00:00'),
		       ('forgejo', 'o2', 'r2', 'rel-b', 'v2.0.0-beta.1', 'Note B', '2025-01-02 00:00:00')`); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	// First open: migration must preserve every legacy row with legacy
	// metadata.
	s, err := New(dataDir)
	if err != nil {
		t.Fatalf("New on legacy db: %v", err)
	}
	gotA, err := s.GetGenerated("legacy:github:rel-a")
	if err != nil || gotA == nil {
		t.Fatalf("legacy row A missing: %v (err=%v)", gotA, err)
	}
	if gotA.Notes != "Note A" || gotA.ToTag != "v1.0.0" || gotA.DisplayVersion != "v1.0.0" ||
		gotA.FromTag != "" || gotA.Profile != "" || gotA.ConfigDigest != "" ||
		gotA.CreatedAt != "2025-01-01 00:00:00" {
		t.Errorf("legacy row A metadata wrong: %+v", gotA)
	}
	gotB, err := s.GetGenerated("legacy:forgejo:rel-b")
	if err != nil || gotB == nil {
		t.Fatalf("legacy row B missing: %v (err=%v)", gotB, err)
	}
	if gotB.Notes != "Note B" || gotB.ToTag != "v2.0.0-beta.1" || gotB.DisplayVersion != "v2.0.0-beta.1" {
		t.Errorf("legacy row B metadata wrong: %+v", gotB)
	}
	// The legacy release-id key no longer resolves.
	if got, err := s.GetGenerated("rel-a"); err != nil || got != nil {
		t.Errorf("legacy release-id key should not resolve: %+v (err=%v)", got, err)
	}

	// A new profile row coexists without colliding with legacy keys.
	if err := s.MarkGenerated(GeneratedNote{
		CacheKey: "new-contract-key", Platform: "github", Owner: "o1", Repo: "r1",
		ReleaseID: "rel-a", ToTag: "v1.0.0", Profile: "maintainer",
		DisplayVersion: "1.0", Notes: "New note",
	}); err != nil {
		t.Fatalf("MarkGenerated new row: %v", err)
	}
	if got, err := s.GetGenerated("new-contract-key"); err != nil || got == nil || got.Notes != "New note" {
		t.Errorf("new row missing: %+v (err=%v)", got, err)
	}
	if got, err := s.GetGenerated("legacy:github:rel-a"); err != nil || got == nil || got.Notes != "Note A" {
		t.Errorf("legacy row clobbered by new row: %+v (err=%v)", got, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen: the migration must be a no-op on an already-migrated db.
	s2, err := New(dataDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if got, err := s2.GetGenerated("legacy:github:rel-a"); err != nil || got == nil || got.Notes != "Note A" {
		t.Errorf("legacy row lost after reopen: %+v (err=%v)", got, err)
	}
	if got, err := s2.GetGenerated("new-contract-key"); err != nil || got == nil || got.Notes != "New note" {
		t.Errorf("new row lost after reopen: %+v (err=%v)", got, err)
	}
}
