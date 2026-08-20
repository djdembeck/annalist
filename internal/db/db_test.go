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

	t.Run("missing release returns nil,nil", func(t *testing.T) {
		got, err := s.GetGenerated("github", "release-absent")
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	if err := s.MarkGenerated("github", "djdembeck", "annalist", "rel-1", "v1.0.0", "Notes one"); err != nil {
		t.Fatalf("MarkGenerated: %v", err)
	}

	got, err := s.GetGenerated("github", "rel-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected a generated note")
	}
	if got.ReleaseID != "rel-1" || got.Tag != "v1.0.0" || got.Notes != "Notes one" {
		t.Errorf("note mismatch: %+v", got)
	}
	if got.Owner != "djdembeck" || got.Repo != "annalist" || got.Platform != "github" {
		t.Errorf("note key fields mismatch: %+v", got)
	}
	if got.CreatedAt == "" {
		t.Error("created_at should be populated")
	}

	// MarkGenerated is idempotent per release_id: overwrites prior note.
	if err := s.MarkGenerated("github", "djdembeck", "annalist", "rel-1", "v2.0.0", "Notes two"); err != nil {
		t.Fatalf("MarkGenerated overwrite: %v", err)
	}
	got, err = s.GetGenerated("github", "rel-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Tag != "v2.0.0" || got.Notes != "Notes two" {
		t.Errorf("overwrite semantics wrong: %+v", got)
	}
}
