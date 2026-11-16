package engine

import (
	"strings"
	"testing"
)

func TestCheckNotesStrictProseSemicolon(t *testing.T) {
	strict := Resolved{Tone: "engineer"}
	cases := []struct {
		name  string
		notes string
		want  bool
	}{
		{"substantive prose rejected", "Upgrade the database; then restart the service.", true},
		{"unrelated prose beside command rejected", "Run go migrate; this keeps old writes safe.", true},
		{"command line with unrelated prose rejected", "go migrate; this keeps old writes safe.", true},
		{"command accepted", "go migrate --dry-run; go migrate", false},
		{"URL accepted", "See https://example.invalid/upgrade?a=1;b=2 for details.", false},
		{"identifier punctuation remains prose", "Set legacy_mode; then restart the service.", true},
		{"single quoted phrase accepted", "The tool reports 'ready; waiting' before startup.", false},
		{"double quoted phrase accepted", "The setting is called \"old; mode\".", false},
		{"quoted span does not excuse prose", "The option is \"safe\"; restart the service.", true},
		{"inline code accepted", "Use `migrate; --dry-run` before deployment.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := CheckNotes(strict, tc.notes)
			if got := len(issues) != 0; got != tc.want {
				t.Fatalf("CheckNotes(%q) issues=%v, want rejection=%v", tc.notes, issues, tc.want)
			}
			if tc.want && issues[0] != "strict_prose_semicolon" {
				t.Fatalf("unexpected issue: %v", issues)
			}
		})
	}
}

func TestCheckNotesPrimaryBulletReadability(t *testing.T) {
	// This paragraph follows the published Audnexus v1.18.0 index-management
	// bullet. Short individual sentences still form an overloaded primary item.
	const overloaded = "Prepare a shared index catalog before the API or worker becomes ready. " +
		"The catalog adopts equivalent indexes and rejects incompatible definitions without dropping constraints. " +
		"DB-only indexes check and indexes apply commands inspect and apply the catalog. " +
		"DB_INDEX_MODE=verify makes API and worker startup verify indexes without creating them, but history prepare and indexes apply still apply missing indexes. " +
		"DB_INDEX_BUILD_WAIT_MS limits the wait for each index, so total startup time can exceed that limit. " +
		"Book history still requires a transaction-capable replica set."
	for _, tone := range []string{"", "engineer", "chronicler", "launch"} {
		t.Run("overloaded/"+tone, func(t *testing.T) {
			issues := CheckNotes(Resolved{Tone: tone}, "## Features\n- "+overloaded)
			if len(issues) != 1 || issues[0] != "primary_bullet_overloaded" {
				t.Fatalf("issues=%v, want primary_bullet_overloaded", issues)
			}
		})
	}
	for _, tc := range []struct {
		name, notes string
		want        bool
	}{
		{"two short sentences", "- Prepare required indexes before startup. Equivalent definitions remain valid.", false},
		{"three short sentences", "- Prepare required indexes. Wait for completion. Reject incompatible definitions.", true},
		{"wrapped primary paragraph", "- Prepare required indexes.\n  Wait for completion.\n  Reject incompatible definitions.", true},
		{"later primary paragraph", "- Prepare required indexes.\n\n  Wait for completion. Keep old constraints. Reject incompatible definitions.", true},
		{"nested action continuation", "- Prepare required indexes.\n\n  - Required action: Stop active writers.\n\n    Run the migration check. Confirm no pending writes. Start the coordinator.", false},
		{"oversized two sentence paragraph", "- " + strings.Repeat("Readers retain compatible records ", 16) + ". Startup remains available.", true},
		{"nested required actions", "- Prepare required indexes before startup.\n  - Required action: Wait for each index to complete.\n  - Required action: Run indexes check before indexes apply.\n  - Caveat: DB_INDEX_MODE=verify does not change history prepare.\n  - Caveat: Book history requires a transaction-capable replica set.", false},
		{"ordered nested actions", "1. Prepare required indexes before startup.\n   1. Wait for completion. Run indexes check. Keep old constraints.\n2. Repair accepted history updates.", false},
		{"command", "- go run ./cmd/indexes.go --mode=check; go run ./cmd/indexes.go --mode=apply", false},
		{"URLs and identifiers", "- See https://example.invalid/v1.2.3/indexes. Use DB_INDEX_MODE=verify. Read schema.prepare.indexes.", false},
		{"protected spans", "- Use `schema.prepare. indexes.apply. history.prepare.` before startup.", false},
		{"separate concerns", "- Prepare required indexes.\n- Repair accepted history updates.\n- Restore stalled jobs.", false},
		{"no total note limit", strings.Repeat("- Restore stalled jobs.\n", 100), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := CheckNotes(Resolved{Tone: "chronicler"}, tc.notes)
			if got := len(issues) != 0; got != tc.want {
				t.Fatalf("issues=%v, want rejection=%v", issues, tc.want)
			}
		})
	}
	for _, r := range []Resolved{
		{Tone: "engineer", Instructions: "Use the project's complete format."},
		{Tone: "Write detailed records for operators."},
	} {
		if got := CheckNotes(r, "- "+overloaded); got != nil {
			t.Errorf("custom prose contract changed: %v", got)
		}
	}
}

func TestCheckNotesBypassesOverridesAndOtherVoices(t *testing.T) {
	for _, r := range []Resolved{
		{Tone: "chronicler"},
		{Tone: "engineer", Instructions: "Use semicolons; preserve this override."},
		{Tone: "", Instructions: "Custom complete prompt; authoritative."},
	} {
		if got := CheckNotes(r, "This prose has a semicolon; keep it."); got != nil {
			t.Errorf("CheckNotes(%+v) = %v, want nil", r, got)
		}
	}
}
