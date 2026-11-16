package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/engine"
)

func truncatedQualityOrigin(t *testing.T) string {
	t.Helper()
	origin := makeOrigin(t, t.TempDir())
	// One invented, indivisible source hunk exceeds the fixed context budget.
	content := "package sample\n\n" + strings.Repeat("// Fern export context keeps the archived record available for review.\n", 5000)
	if err := os.WriteFile(filepath.Join(origin, "export.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitNoAsk(t, origin, "add", "export.go")
	gitNoAsk(t, origin, "commit", "-q", "-m", "feat: add Fern archive export", "-m", "The export keeps the prior record available for review.")
	gitNoAsk(t, origin, "tag", "v0.3.0")
	return origin
}

func requireQualityError(t *testing.T, err error, issues []string) {
	t.Helper()
	var qualityErr *QualityError
	if !errors.As(err, &qualityErr) || !slices.Equal(qualityErr.Issues, issues) {
		t.Fatalf("err=%v, want QualityError with issues %v", err, issues)
	}
	if err.Error() != "pipeline: notes need quality review before publication" {
		t.Fatalf("unsafe quality error: %q", err.Error())
	}
}

func TestQualityPreviewAndRejectedPublish(t *testing.T) {
	for _, tc := range []struct {
		name, answer, mode, instructions string
		issues                           []string
	}{
		{"strict prose", "Fern export is available; archived records remain readable.", engine.ModeLite, "", []string{"strict_prose_semicolon"}},
		{"overloaded primary", "Prepare required indexes before startup. Add inspection commands. Repair accepted history updates.", engine.ModeLite, "", []string{"primary_bullet_overloaded"}},
		{"incomplete custom context", "Fern export is available; archived records remain readable.", engine.ModeDeep, "Keep the project's punctuation; preserve source facts.", []string{"source_context_incomplete"}},
		{"both required checks", "Fern export is available; archived records remain readable.", engine.ModeDeep, "", []string{"strict_prose_semicolon", "source_context_incomplete"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pip, stub, platform, store := fixtureWithStub(t, newAttributedStub(tc.answer), nil, map[string]*Release{"v0.2.0": {ID: 1, Body: "existing release"}})
			if err := store.UpsertSettings(db.Settings{Tone: "engineer", Instructions: tc.instructions, Mode: tc.mode}); err != nil {
				t.Fatal(err)
			}
			spec := genSpec("v0.2.0", "quality-preview")
			if tc.mode == engine.ModeDeep {
				platform.origin = truncatedQualityOrigin(t)
				spec.ToTag, spec.FromTag = "v0.3.0", "v0.2.0"
			}
			platform.releaseErr = errors.New("publication must not be reached")

			preview, err := pip.GenerateNotes(context.Background(), spec, Options{Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if preview.Notes != linkedTestNotes(tc.answer) || preview.Outcome != "generated" || preview.WarningCode != "quality_review_required" || !slices.Equal(preview.QualityIssues, tc.issues) {
				t.Fatalf("preview=%+v", preview)
			}
			run := onlyRun(t, store)
			if run.Status != "generated" || run.ErrorCode != nil || run.WarningCode == nil || *run.WarningCode != "quality_review_required" {
				t.Fatalf("preview run=%+v", run)
			}

			rejected, err := pip.GenerateNotes(context.Background(), spec, Options{Source: "webhook", Publish: true, Force: true})
			requireQualityError(t, err, tc.issues)
			if rejected.Notes != linkedTestNotes(tc.answer) || !slices.Equal(rejected.QualityIssues, tc.issues) || rejected.WarningCode != "quality_review_required" {
				t.Fatalf("rejected result=%+v", rejected)
			}
			if platform.edited != "" || platform.releases["v0.2.0"].Body != "existing release" || countGeneratedByReleaseID(t, pip.Cfg.Data.Dir, spec.ReleaseID) != 0 {
				t.Fatal("quality failure changed a release or wrote a cache entry")
			}
			if stub.calls != 2 {
				t.Fatalf("LLM calls=%d, want one per explicit request and no retry", stub.calls)
			}
			run = onlyRun(t, store)
			if run.Status != "failed" || run.ErrorCode == nil || *run.ErrorCode != "quality" || run.WarningCode == nil || *run.WarningCode != "quality_review_required" {
				t.Fatalf("rejected publication run=%+v", run)
			}
			encoded, err := json.Marshal(run)
			if err != nil || strings.Contains(string(encoded), tc.answer) || strings.Contains(string(encoded), "Fern export context") {
				t.Fatalf("history leaked generated notes or source: %s err=%v", encoded, err)
			}
		})
	}
}

func TestQualityPreservesAuthoritativePunctuation(t *testing.T) {
	const notes = "Fern export is available; archived records remain readable."
	for _, tc := range []struct {
		name, tone, instructions, profile string
		files                             map[string]string
	}{
		{name: "global instructions", tone: "engineer", instructions: "Keep the project's punctuation."},
		{name: "repository instructions", tone: "engineer", files: map[string]string{".github/release-notes-instructions.md": "Keep the project's punctuation."}},
		{name: "named profile", tone: "engineer", profile: "maintainer", files: map[string]string{
			profileManifestPath:               "version: 1\nprofiles:\n  maintainer:\n    prompt: .annalist/prompts/maintainer.md\n",
			".annalist/prompts/maintainer.md": "Keep the project's punctuation.",
		}},
		{name: "custom tone", tone: "Use semicolons where useful."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pip, stub, platform, store := fixtureWithStub(t, newAttributedStub(notes), tc.files, map[string]*Release{"v0.2.0": {ID: 1}})
			if err := store.UpsertSettings(db.Settings{Tone: tc.tone, Instructions: tc.instructions}); err != nil {
				t.Fatal(err)
			}
			spec := genSpec("v0.2.0", "authoritative")
			spec.Profile = tc.profile
			result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
			if err != nil || result.Outcome != "published" || result.Notes != linkedTestNotes(notes) || result.WarningCode != "" || len(result.QualityIssues) != 0 || platform.edited != linkedTestNotes(notes)+"\n"+marker {
				t.Fatalf("result=%+v edited=%q err=%v", result, platform.edited, err)
			}
			if tc.name != "custom tone" && stub.system() != "Keep the project's punctuation." {
				t.Fatalf("authoritative prompt changed: %q", stub.system())
			}
			stored, err := generatedByReleaseID(t, pip.Cfg.Data.Dir, spec.ReleaseID)
			if err != nil || stored == nil || stored.Notes != linkedTestNotes(notes) {
				t.Fatalf("stored=%+v err=%v", stored, err)
			}
		})
	}
}

func TestQualityPreservesProtectedTechnicalText(t *testing.T) {
	for name, notes := range map[string]string{
		"command":   "curl https://example.invalid/export; git status",
		"URL":       "Use https://example.invalid/export;mode=archive for archived records.",
		"code":      "Use `archive;mode` when opening the export.",
		"quotation": "The warning reads \"archive; retry later\".",
	} {
		t.Run(name, func(t *testing.T) {
			pip, _, platform, store := fixtureWithStub(t, newAttributedStub(notes), nil, map[string]*Release{"v0.3.0": {ID: 1}})
			platform.origin = makeOrigin(t, t.TempDir())
			gitNoAsk(t, platform.origin, "commit", "--allow-empty", "-m", "feat: add Fern archive export", "-m", notes)
			gitNoAsk(t, platform.origin, "tag", "v0.3.0")
			if err := store.UpsertSettings(db.Settings{Tone: "engineer"}); err != nil {
				t.Fatal(err)
			}
			spec := genSpec("v0.3.0", "protected")
			spec.FromTag = "v0.2.0"
			result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
			if err != nil || result.Outcome != "published" || result.Notes != linkedTestNotes(notes) || platform.edited != linkedTestNotes(notes)+"\n"+marker {
				t.Fatalf("technical text changed or rejected: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestQualityRevalidatesCachedOutputAndSource(t *testing.T) {
	for _, incomplete := range []bool{false, true} {
		t.Run(map[bool]string{false: "cached punctuation", true: "cached incomplete source"}[incomplete], func(t *testing.T) {
			pip, stub, platform, store := fixture(t, nil, nil)
			settings := db.Settings{Tone: "engineer", Mode: engine.ModeLite}
			spec := genSpec("v0.2.0", "cached-quality")
			notes, issue := "Fern export is available; archived records remain readable.", "strict_prose_semicolon"
			if incomplete {
				platform.origin = truncatedQualityOrigin(t)
				spec.ToTag, spec.FromTag = "v0.3.0", "v0.2.0"
				settings.Mode = engine.ModeDeep
				notes, issue = "Fern export is available.", "source_context_incomplete"
			} else {
				spec.FromTag = "v0.1.0"
			}
			if err := store.UpsertSettings(settings); err != nil {
				t.Fatal(err)
			}
			_, effective, resolved, err := pip.Resolve(context.Background(), spec.Platform, spec.Owner, spec.Repo)
			if err != nil {
				t.Fatal(err)
			}
			digest := pip.configDigest(effective.BaseURL, resolved)
			key := pip.cacheKey(spec, spec.FromTag, digest)
			linked := linkedTestNotes(notes)
			groups := cachedCitationRE.FindAllStringIndex(linked, -1)
			sha := engine.TargetCommitSHA(context.Background(), platform.origin, spec.ToTag)
			attribution, err := json.Marshal(cachedAttribution{NotesHash: fmt.Sprintf("%x", sha256.Sum256([]byte(linked))), Groups: []sourceUse{{SHAs: []string{sha}, Start: groups[0][0], End: groups[0][1]}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.MarkGenerated(db.GeneratedNote{CacheKey: key, Platform: spec.Platform, Owner: spec.Owner, Repo: spec.Repo, ReleaseID: spec.ReleaseID, FromTag: spec.FromTag, ToTag: spec.ToTag, DisplayVersion: spec.ToTag, ConfigDigest: digest, Notes: linked, SourceAttributions: string(attribution)}); err != nil {
				t.Fatal(err)
			}
			platform.releaseErr = errors.New("cached quality failure must not reach the release API")
			preview, err := pip.GenerateNotes(context.Background(), spec, Options{})
			if err != nil || preview.Outcome != "cached" || preview.Notes != linkedTestNotes(notes) || preview.WarningCode != "quality_review_required" || !slices.Equal(preview.QualityIssues, []string{issue}) {
				t.Fatalf("cached preview=%+v err=%v", preview, err)
			}
			_, err = pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
			requireQualityError(t, err, []string{issue})
			if stub.calls != 0 || platform.edited != "" || countGeneratedByReleaseID(t, pip.Cfg.Data.Dir, spec.ReleaseID) != 1 {
				t.Fatalf("cached rejection called model, edited remote, or wrote cache: calls=%d", stub.calls)
			}
		})
	}
}

func TestGenerationContractInvalidatesAuthoritativePromptCache(t *testing.T) {
	const prompt = "Keep this complete prompt unchanged; retain the source facts."
	const answer = "Fern export is available; archived records remain readable."
	pip, stub, _, store := fixtureWithStub(t, newAttributedStub(answer), nil, map[string]*Release{"v0.2.0": {ID: 1}})
	if err := store.UpsertSettings(db.Settings{Tone: "engineer", Instructions: prompt}); err != nil {
		t.Fatal(err)
	}
	spec := genSpec("v0.2.0", "old-cache")
	spec.FromTag = "v0.1.0"
	_, effective, resolved, err := pip.Resolve(context.Background(), spec.Platform, spec.Owner, spec.Repo)
	if err != nil {
		t.Fatal(err)
	}
	// The previous contract used commit-sized bullets and had no private
	// attribution groups. Unchanged custom prompts must also miss those rows.
	legacy := configDigestInput{
		ContractVersion: "source3-generation3-quality2-citations1",
		SystemPrompt:    prompt,
		Model:           resolved.Model,
		Temperature:     resolved.Temperature,
		CommitTypes:     resolved.CommitTypes,
		Mode:            resolved.Mode,
		MaxTokens:       resolved.MaxTokens,
		ThinkingLevel:   resolved.ThinkingLevel,
		BaseURL:         effective.BaseURL,
	}
	if legacy.CommitTypes == nil {
		legacy.CommitTypes = []string{}
	}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	oldDigest := hex.EncodeToString(sum[:])
	if err := store.MarkGenerated(db.GeneratedNote{CacheKey: pip.cacheKey(spec, spec.FromTag, oldDigest), Platform: spec.Platform, Owner: spec.Owner, Repo: spec.Repo, ReleaseID: spec.ReleaseID, FromTag: spec.FromTag, ToTag: spec.ToTag, DisplayVersion: spec.ToTag, ConfigDigest: oldDigest, Notes: "OLD UNCHECKED NOTES"}); err != nil {
		t.Fatal(err)
	}
	result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
	if err != nil || result.Outcome != "published" || result.Notes != linkedTestNotes(answer) || result.ConfigDigest == oldDigest || stub.calls != 1 || stub.system() != prompt {
		t.Fatalf("old custom-prompt cache was reused: result=%+v calls=%d err=%v", result, stub.calls, err)
	}
	cached, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
	if err != nil || cached.Outcome != "cached" || cached.Notes != linkedTestNotes(answer) || stub.calls != 1 {
		t.Fatalf("current cache not reusable: result=%+v calls=%d err=%v", cached, stub.calls, err)
	}
}

func TestPublishRequiresCurrentQualityReceipt(t *testing.T) {
	pip, _, platform, _ := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1}})
	spec := genSpec("v0.2.0", "receipt")
	checked, err := pip.GenerateNotes(context.Background(), spec, Options{})
	if err != nil {
		t.Fatal(err)
	}
	platform.releaseErr = errors.New("unchecked notes must not reach release API")
	for _, name := range []string{"unchecked", "changed notes", "changed contract", "known issue"} {
		t.Run(name, func(t *testing.T) {
			result, target := checked, spec
			issue := "quality_check_required"
			switch name {
			case "unchecked":
				result.qualityReceipt = qualityReceipt{}
			case "changed notes":
				result.Notes += " Changed after review."
			case "changed contract":
				target.ReleaseID = "other-release-contract"
			case "known issue":
				result.QualityIssues = []string{"source_context_incomplete"}
				issue = "source_context_incomplete"
			}
			published, err := pip.Publish(context.Background(), result, target, platform, true)
			requireQualityError(t, err, []string{issue})
			if published || platform.edited != "" || countGeneratedByReleaseID(t, pip.Cfg.Data.Dir, target.ReleaseID) != 0 {
				t.Fatal("unchecked result was published or cached")
			}
		})
	}
}
