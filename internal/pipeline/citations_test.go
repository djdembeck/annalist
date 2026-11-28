package pipeline

import (
	"context"
	"crypto/sha256"
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

func citationOrigin(t *testing.T) (origin, exportSHA, perfSHA string) {
	t.Helper()
	origin = t.TempDir()
	gitNoAsk(t, origin, "init", "-q")
	gitNoAsk(t, origin, "commit", "--allow-empty", "-m", "chore: baseline")
	gitNoAsk(t, origin, "tag", "v1.0.0")
	if err := os.WriteFile(filepath.Join(origin, "export.txt"), []byte("export"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitNoAsk(t, origin, "add", ".")
	gitNoAsk(t, origin, "commit", "-m", "feat: export verified archives", "-m", "Run annalist export --archive only for verified accounts. Archived records remain readable.")
	exportSHA = engine.TargetCommitSHA(context.Background(), origin, "HEAD")
	if err := os.WriteFile(filepath.Join(origin, "audio.txt"), []byte("parse once"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitNoAsk(t, origin, "add", ".")
	gitNoAsk(t, origin, "commit", "-m", "perf: parse audio headers once", "-m", "Imported recordings no longer parse the same header twice.")
	perfSHA = engine.TargetCommitSHA(context.Background(), origin, "HEAD")
	// A bookkeeping repeat must not replace the actual source commit's PR.
	gitNoAsk(t, origin, "commit", "--allow-empty", "-m", "perf: parse audio headers once", "-m", "Imported recordings no longer parse the same header twice.")
	gitNoAsk(t, origin, "tag", "v1.1.0")
	if exportSHA == "" || perfSHA == "" {
		t.Fatal("missing source commit identity")
	}
	return origin, exportSHA, perfSHA
}

func TestAutomaticCitationsPreserveMappingAndCachedLinks(t *testing.T) {
	origin, exportSHA, perfSHA := citationOrigin(t)
	const prompt = "Use the project's own voice. Keep source conditions and commands exact."
	for _, voice := range []struct {
		name, tone, instructions, profile string
		files                             map[string]string
	}{
		{name: "neutral"},
		{name: "engineer", tone: "engineer"},
		{name: "chronicler", tone: "chronicler"},
		{name: "launch", tone: "launch"},
		{name: "custom tone", tone: "Write for archive operators."},
		{name: "custom instructions", instructions: prompt},
		{name: "named profile", profile: "operator", files: map[string]string{
			profileManifestPath:     "version: 1\nprofiles:\n  operator:\n    prompt: .annalist/operator.md\n",
			".annalist/operator.md": prompt,
		}},
	} {
		for _, mode := range []string{engine.ModeLite, engine.ModeDeep} {
			t.Run(voice.name+"/"+mode, func(t *testing.T) {
				// Reorder the changes to prove that citations follow identity,
				// not the collector's order or a #number in the subject.
				raw := "Version 1.1.0 improves imported recordings.\n\n## Improvements\n- Parse imported audio headers once. " + engine.SourceToken(perfSHA) +
					"\n- Export archives only for verified accounts.\n  Archived records remain readable. " + engine.SourceToken(exportSHA) +
					"\n\n  Required actions:\n  - Run annalist export --archive."
				pip, stub, platform, store := fixtureWithStub(t, newStub(raw), voice.files, map[string]*Release{"v1.1.0": {ID: 1}})
				platform.origin = origin
				platform.references = map[string]CommitReference{
					exportSHA: {PullRequests: []PullRequestReference{{Number: 1754, URL: "https://github.com/djdembeck/annalist/pull/1754"}}},
					perfSHA:   {PullRequests: []PullRequestReference{{Number: 1779, URL: "https://github.com/djdembeck/annalist/pull/1779"}}},
				}
				if err := store.UpsertSettings(db.Settings{Tone: voice.tone, Instructions: voice.instructions, Mode: mode}); err != nil {
					t.Fatal(err)
				}
				spec := genSpec("v1.1.0", "automatic-citations")
				spec.Profile = voice.profile
				want := strings.ReplaceAll(raw, engine.SourceToken(perfSHA), "([#1779](https://github.com/djdembeck/annalist/pull/1779))")
				want = strings.ReplaceAll(want, engine.SourceToken(exportSHA), testCitation)
				result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
				if err != nil || result.Outcome != "published" || result.Notes != want || platform.edited != want+"\n"+marker || len(result.QualityIssues) != 0 {
					t.Fatalf("result=%+v edited=%q err=%v", result, platform.edited, err)
				}
				if !slices.Equal(platform.referenceSHAs, []string{perfSHA, exportSHA}) || strings.Contains(result.Notes, "annalist:") {
					t.Fatalf("wrong source identity or leaked token: sources=%v notes=%q", platform.referenceSHAs, result.Notes)
				}
				if (voice.instructions != "" || voice.profile != "") && stub.system() != prompt {
					t.Fatalf("custom system prompt changed: %q", stub.system())
				}
				platform.referenceErr = errors.New("cache hits must not refresh platform provenance")
				stub.errStatus = 500
				cached, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
				if err != nil || cached.Outcome != "cached" || cached.Notes != want || stub.calls != 1 || platform.referenceCalls != 1 {
					t.Fatalf("cached links changed or were re-resolved: result=%+v calls=%d lookups=%d err=%v", cached, stub.calls, platform.referenceCalls, err)
				}
			})
		}
	}
}

func TestInvalidAttributionCannotEscapePublishOrCache(t *testing.T) {
	sha := engine.TargetCommitSHA(context.Background(), sharedOrigin, "v0.2.0")
	token := engine.SourceToken(sha)
	unknown := engine.SourceToken(strings.Repeat("f", 40))
	for name, raw := range map[string]string{
		"missing identity":                  "- Export archives.",
		"empty change":                      "- " + token,
		"unknown identity":                  "- Export archives. " + unknown,
		"repeated identity in item":         "- Export archives. " + token + " " + token,
		"separated attribution groups":      "- Export archives. " + token + " More prose. " + token,
		"split attribution group":           "- Export archives. " + token + "\n  " + token,
		"prose after attribution":           "- Export archives. " + token + "\n  Another sentence follows.",
		"prose after nested action":         "- Export archives. " + token + "\n  - Required action: Stop writers.\n\n  Another sentence follows.",
		"unlinked primary item":             "- Export archives. " + token + "\n- Another change.",
		"identity in introduction":          "Version 2 exports archives. " + token + "\n\n- Export archives.",
		"identity in heading":               "## Export " + token + "\n- Export archives.",
		"identity in nested action":         "- Export archives.\n  - Run annalist export. " + token,
		"identity in code":                  "- Export archives. `" + token + "`",
		"identity in fence":                 "```\n- Export archives. " + token + "\n```",
		"identity hidden in comment":        "<!--\n- Export archives. " + token + "\n-->",
		"identity hidden in inline comment": "Version 2. <!--\n- Export archives. " + token + "\n-->",
		"identity hidden in HTML":           "<div>\n- Export archives. " + token + "\n</div>",
		"malformed identity":                "- Export archives. [[annalist:not-a-sha]]",
		"forged PR":                         "- Export archives. ([#1754](https://github.com/djdembeck/annalist/pull/1754)) " + token,
		"wrong repository link":             "- Export archives. [change](https://github.com/other/project/pull/9) " + token,
		"invented documentation":            "- Export archives. [guide](https://example.invalid/invented-guide) " + token,
		"relative target":                   "- Export archives. [guide](../invented-guide) " + token,
		"HTML target":                       "- Export archives. <a href='https://example.invalid'>guide</a> " + token,
	} {
		t.Run(name, func(t *testing.T) {
			pip, stub, platform, _ := fixtureWithStub(t, newStub(raw), nil, map[string]*Release{"v0.2.0": {ID: 1}})
			spec := genSpec("v0.2.0", "bad-attribution")
			preview, err := pip.GenerateNotes(context.Background(), spec, Options{})
			if err != nil || preview.Notes != "" || preview.WarningCode != "quality_review_required" || !slices.Equal(preview.QualityIssues, []string{"source_attribution_invalid"}) {
				t.Fatalf("unsafe preview=%+v err=%v", preview, err)
			}
			result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true, Force: true})
			requireQualityError(t, err, []string{"source_attribution_invalid"})
			if result.Notes != "" || platform.edited != "" || platform.referenceCalls != 0 || stub.calls != 4 || countGeneratedByReleaseID(t, pip.Cfg.Data.Dir, spec.ReleaseID) != 0 {
				t.Fatalf("invalid attribution escaped or retried: result=%+v model_calls=%d lookups=%d", result, stub.calls, platform.referenceCalls)
			}
		})
	}
}

// TestAttributionRetryRecoversOnce covers the single corrective regeneration
// after the model's own attribution fails: the second draft is validated from
// scratch, so a dropped token can recover without weakening the gate.
func TestAttributionRetryRecoversOnce(t *testing.T) {
	origin, exportSHA, perfSHA := citationOrigin(t)
	good := "- Export archives only for verified accounts. " + engine.SourceToken(exportSHA) + " " + engine.SourceToken(perfSHA)
	stub := newStub("- Export archives only for verified accounts.")
	stub.answers = []string{"- Export archives only for verified accounts.", good}
	pip, _, platform, _ := fixtureWithStub(t, stub, nil, map[string]*Release{"v1.1.0": {ID: 1}})
	platform.origin = origin
	platform.references = map[string]CommitReference{
		exportSHA: {PullRequests: []PullRequestReference{{Number: 1754, URL: "https://github.com/djdembeck/annalist/pull/1754"}}},
		perfSHA:   {PullRequests: []PullRequestReference{{Number: 1779, URL: "https://github.com/djdembeck/annalist/pull/1779"}}},
	}
	spec := genSpec("v1.1.0", "attribution-retry")
	spec.FromTag = "v1.0.0"
	result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
	if err != nil || result.Outcome != "published" || len(result.QualityIssues) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if stub.calls != 2 {
		t.Fatalf("model calls = %d, want the first draft plus one correction", stub.calls)
	}
	if !strings.Contains(result.Notes, "https://github.com/djdembeck/annalist/pull/1754") || !strings.Contains(result.Notes, "https://github.com/djdembeck/annalist/pull/1779") {
		t.Fatalf("recovered notes lack verified links: %q", result.Notes)
	}
	if strings.Contains(result.Notes, "annalist:") {
		t.Fatalf("source tokens leaked: %q", result.Notes)
	}
}

func TestConcernAttributionCoverage(t *testing.T) {
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	ta, tb := engine.SourceToken(a), engine.SourceToken(b)
	commits := engine.CommitLog{Commits: []engine.Commit{{SHA: a}, {SHA: b}}}
	for _, tc := range []struct {
		name, notes string
		groups      int
		valid       bool
	}{
		{"split source concerns", "- Prepare required indexes. " + ta + "\n- Add index inspection commands. " + ta + "\n- Restore stalled jobs. " + tb, 3, true},
		{"related grouped sources", "- Restore stalled jobs. " + ta + " " + tb, 1, true},
		{"missing grouped source", "- Restore stalled jobs. " + ta + "\n- Preserve recovery state. " + ta, 0, false},
		{"unknown grouped source", "- Restore stalled jobs. " + ta + " " + engine.SourceToken(strings.Repeat("f", 40)), 0, false},
		{"duplicate inside group", "- Restore stalled jobs. " + ta + " " + tb + " " + ta, 0, false},
		{"malformed group", "- Restore stalled jobs. " + ta + " [[annalist:bad]] " + tb, 0, false},
		{"comma separated tokens", "- Restore stalled jobs. " + ta + ", " + tb, 0, false},
		{"group in nested action", "- Restore stalled jobs.\n  - Restart the worker. " + ta + " " + tb, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uses, err := attributedSources(tc.notes, commits, false)
			if (err == nil) != tc.valid || tc.valid && len(uses) != tc.groups {
				t.Fatalf("uses=%+v err=%v, want valid=%v groups=%d", uses, err, tc.valid, tc.groups)
			}
		})
	}
}

func TestModelLinksWithGroupedSourceTokens(t *testing.T) {
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	group := engine.SourceToken(a) + " " + engine.SourceToken(b)
	commits := engine.CommitLog{Commits: []engine.Commit{{SHA: a}, {SHA: b}}}
	const guide = "https://example.invalid/upgrade"
	const pr = "https://github.com/djdembeck/annalist/pull/1754"
	for _, tc := range []struct {
		name, prose string
		valid       bool
	}{
		{"transport group", "Restore stalled jobs.", true},
		{"source-provided guide", "Restore stalled jobs. See [the guide](" + guide + ").", true},
		{"reference-style link", "Restore stalled jobs. See [the guide][upgrade].", false},
		{"inline HTML link", "Restore stalled jobs. See <a href='" + guide + "'>the guide</a>.", false},
		{"invented target", "Restore stalled jobs. See [the guide](https://evil.invalid/upgrade).", false},
		{"model-supplied PR target", "Restore stalled jobs. See [the change](" + pr + ").", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notes := "- " + tc.prose + " " + group
			if _, err := attributedSources(notes, commits, false); err != nil {
				t.Fatalf("valid source group rejected before link checking: %v", err)
			}
			if err := validateModelLinks(notes, guide+"\n"+pr); (err == nil) != tc.valid {
				t.Fatalf("link error=%v, want valid=%v", err, tc.valid)
			}
		})
	}
}

func TestConcernCitationsPreserveCoverageAndCachedLinks(t *testing.T) {
	origin, exportSHA, perfSHA := citationOrigin(t)
	a, b := engine.SourceToken(exportSHA), engine.SourceToken(perfSHA)
	pr := PullRequestReference{Number: 1754, URL: "https://github.com/djdembeck/annalist/pull/1754"}
	otherPR := PullRequestReference{Number: 1779, URL: "https://github.com/djdembeck/annalist/pull/1779"}
	commitA := "[" + exportSHA[:7] + "](https://github.com/djdembeck/annalist/commit/" + exportSHA + ")"
	commitB := "[" + perfSHA[:7] + "](https://github.com/djdembeck/annalist/commit/" + perfSHA + ")"
	fallbackGroup := "(" + commitA + ", " + commitB + ")"
	if exportSHA > perfSHA {
		fallbackGroup = "(" + commitB + ", " + commitA + ")"
	}
	for _, tc := range []struct {
		name, raw, want string
		refs            map[string]CommitReference
		groups          [][]string
	}{
		{
			name:   "one source supports distinct concerns",
			raw:    "- Export verified archives. " + a + "\n- Keep archived records readable. " + a + "\n- Parse imported audio headers once. " + b,
			want:   "- Export verified archives. " + testCitation + "\n- Keep archived records readable. " + testCitation + "\n- Parse imported audio headers once. ([#1779](" + otherPR.URL + "))",
			refs:   map[string]CommitReference{exportSHA: {PullRequests: []PullRequestReference{pr}}, perfSHA: {PullRequests: []PullRequestReference{otherPR}}},
			groups: [][]string{{exportSHA}, {exportSHA}, {perfSHA}},
		},
		{
			name:   "related sources share one PR",
			raw:    "- Process verified archive recordings. " + a + " " + b,
			want:   "- Process verified archive recordings. " + testCitation,
			refs:   map[string]CommitReference{exportSHA: {PullRequests: []PullRequestReference{pr}}, perfSHA: {PullRequests: []PullRequestReference{pr}}},
			groups: [][]string{{exportSHA, perfSHA}},
		},
		{
			name:   "grouped PR and commit fallback",
			raw:    "- Process verified archive recordings. " + a + " " + b,
			want:   "- Process verified archive recordings. ([#1754](" + pr.URL + "), " + commitB + ")",
			refs:   map[string]CommitReference{exportSHA: {PullRequests: []PullRequestReference{pr}}, perfSHA: {CommitURL: "https://github.com/djdembeck/annalist/commit/" + perfSHA}},
			groups: [][]string{{exportSHA, perfSHA}},
		},
		{
			name:   "grouped commit fallbacks",
			raw:    "- Process verified archive recordings. " + a + " " + b,
			want:   "- Process verified archive recordings. " + fallbackGroup,
			refs:   map[string]CommitReference{exportSHA: {CommitURL: "https://github.com/djdembeck/annalist/commit/" + exportSHA}, perfSHA: {CommitURL: "https://github.com/djdembeck/annalist/commit/" + perfSHA}},
			groups: [][]string{{exportSHA, perfSHA}},
		},
		{
			name:   "repeated commit fallback",
			raw:    "- Export verified archives. " + a + "\n- Keep archived records readable. " + a + "\n- Parse imported audio headers once. " + b,
			want:   "- Export verified archives. (" + commitA + ")\n- Keep archived records readable. (" + commitA + ")\n- Parse imported audio headers once. (" + commitB + ")",
			refs:   map[string]CommitReference{exportSHA: {CommitURL: "https://github.com/djdembeck/annalist/commit/" + exportSHA}, perfSHA: {CommitURL: "https://github.com/djdembeck/annalist/commit/" + perfSHA}},
			groups: [][]string{{exportSHA}, {exportSHA}, {perfSHA}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pip, stub, platform, store := fixtureWithStub(t, newStub(tc.raw), nil, map[string]*Release{"v1.1.0": {ID: 1}})
			platform.origin, platform.references = origin, tc.refs
			if err := store.UpsertSettings(db.Settings{Tone: "engineer"}); err != nil {
				t.Fatal(err)
			}
			spec := genSpec("v1.1.0", "concern-citations")
			result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
			if err != nil || result.Outcome != "published" || result.Notes != tc.want || len(result.QualityIssues) != 0 || !slices.Equal(platform.referenceSHAs, []string{exportSHA, perfSHA}) {
				t.Fatalf("result=%+v sources=%v err=%v", result, platform.referenceSHAs, err)
			}
			stored, err := store.GetGenerated(result.qualityReceipt.cacheKey)
			if err != nil || stored == nil {
				t.Fatalf("missing cached attribution: row=%+v err=%v", stored, err)
			}
			var attribution cachedAttribution
			if err := json.Unmarshal([]byte(stored.SourceAttributions), &attribution); err != nil || len(attribution.Groups) != len(tc.groups) {
				t.Fatalf("attribution=%+v err=%v", attribution, err)
			}
			for i, group := range attribution.Groups {
				if !slices.Equal(group.SHAs, tc.groups[i]) {
					t.Fatalf("group %d sources=%v, want %v", i, group.SHAs, tc.groups[i])
				}
			}
			platform.referenceErr = errors.New("cache must not refresh provenance")
			stub.errStatus = 500
			cached, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
			if err != nil || cached.Outcome != "cached" || cached.Notes != tc.want || stub.calls != 1 || platform.referenceCalls != 1 {
				t.Fatalf("cached=%+v model_calls=%d lookups=%d err=%v", cached, stub.calls, platform.referenceCalls, err)
			}
		})
	}
}

func TestCachedConcernAttributionRejectsCorruption(t *testing.T) {
	origin, a, b := citationOrigin(t)
	raw := "- Process verified archive recordings. " + engine.SourceToken(a) + " " + engine.SourceToken(b)
	for _, name := range []string{"missing metadata", "malformed metadata", "changed body", "missing source", "unknown source", "duplicate source", "invalid span", "missing group", "invalid placement", "foreign link"} {
		t.Run(name, func(t *testing.T) {
			pip, stub, platform, store := fixtureWithStub(t, newStub(raw), nil, map[string]*Release{"v1.1.0": {ID: 1}})
			platform.origin = origin
			ref := CommitReference{PullRequests: []PullRequestReference{{Number: 1754, URL: "https://github.com/djdembeck/annalist/pull/1754"}}}
			platform.references = map[string]CommitReference{a: ref, b: ref}
			spec := genSpec("v1.1.0", "cached-concern-corruption")
			result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
			if err != nil || result.Outcome != "published" {
				t.Fatalf("initial publication=%+v err=%v", result, err)
			}
			row, err := store.GetGenerated(result.qualityReceipt.cacheKey)
			if err != nil || row == nil {
				t.Fatalf("cached row=%+v err=%v", row, err)
			}
			var attribution cachedAttribution
			if err := json.Unmarshal([]byte(row.SourceAttributions), &attribution); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "missing metadata":
				row.SourceAttributions = ""
			case "malformed metadata":
				row.SourceAttributions = "{"
			case "changed body":
				row.Notes += " Changed after publication."
			case "missing source":
				attribution.Groups[0].SHAs = []string{a}
			case "unknown source":
				attribution.Groups[0].SHAs = append(attribution.Groups[0].SHAs, strings.Repeat("f", 40))
			case "duplicate source":
				attribution.Groups[0].SHAs = append(attribution.Groups[0].SHAs, a)
			case "invalid span":
				attribution.Groups[0].Start++
			case "missing group":
				attribution.Groups = nil
			case "invalid placement":
				row.Notes = "Version 1.1.0. " + testCitation + "\n\n- Process verified archive recordings."
			case "foreign link":
				row.Notes = strings.ReplaceAll(row.Notes, "https://github.com/djdembeck/annalist/pull/1754", "https://evil.invalid/djdembeck/annalist/pull/1754")
			}
			if name != "missing metadata" && name != "malformed metadata" {
				if name == "invalid placement" || name == "foreign link" {
					attribution.NotesHash = fmt.Sprintf("%x", sha256.Sum256([]byte(row.Notes)))
					span := cachedCitationRE.FindStringIndex(row.Notes)
					attribution.Groups[0].Start, attribution.Groups[0].End = span[0], span[1]
				}
				encoded, err := json.Marshal(attribution)
				if err != nil {
					t.Fatal(err)
				}
				row.SourceAttributions = string(encoded)
			}
			if err := store.MarkGenerated(*row); err != nil {
				t.Fatal(err)
			}
			platform.edited = ""
			platform.referenceErr = errors.New("cached corruption must not refresh provenance")
			stub.errStatus = 500
			preview, err := pip.GenerateNotes(context.Background(), spec, Options{})
			if err != nil || preview.Outcome != "cached" || preview.Notes != "" || preview.WarningCode != "quality_review_required" || !slices.Equal(preview.QualityIssues, []string{"source_attribution_invalid"}) {
				t.Fatalf("unsafe cached preview=%+v err=%v", preview, err)
			}
			_, err = pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
			requireQualityError(t, err, []string{"source_attribution_invalid"})
			if platform.edited != "" || stub.calls != 1 || platform.referenceCalls != 1 || countGeneratedByReleaseID(t, pip.Cfg.Data.Dir, spec.ReleaseID) != 1 {
				t.Fatal("corrupt cache reached publication, regeneration, or provider lookup")
			}
		})
	}
}

func TestScopedCitationsDoNotRequireOmittedSources(t *testing.T) {
	origin, exportSHA, _ := citationOrigin(t)
	gitNoAsk(t, origin, "tag", "archive/v1.1.0")
	for _, raw := range []string{"- Export verified archives. " + engine.SourceToken(exportSHA), "No changes documented for this artifact."} {
		pip, _, platform, _ := fixtureWithStub(t, newStub(raw), nil, map[string]*Release{"archive/v1.1.0": {ID: 1}})
		platform.origin = origin
		platform.references = map[string]CommitReference{exportSHA: {PullRequests: []PullRequestReference{{Number: 1754, URL: "https://github.com/djdembeck/annalist/pull/1754"}}}}
		spec := genSpec("archive/v1.1.0", "scoped")
		spec.FromTag = "v1.0.0"
		result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
		if err != nil || result.Outcome != "published" || len(result.QualityIssues) != 0 {
			t.Fatalf("valid scoped output rejected: result=%+v err=%v", result, err)
		}
		if strings.HasPrefix(raw, "- ") && !slices.Equal(platform.referenceSHAs, []string{exportSHA}) {
			t.Fatalf("omitted sources were looked up: %v", platform.referenceSHAs)
		}
		if !strings.HasPrefix(raw, "- ") && (platform.referenceCalls != 0 || result.Notes != raw) {
			t.Fatalf("empty artifact scope was altered: result=%+v lookups=%d", result, platform.referenceCalls)
		}
	}
}

func TestSourceFailuresStayExplicitAndCannotPublish(t *testing.T) {
	sha := engine.TargetCommitSHA(context.Background(), sharedOrigin, "v0.2.0")
	for _, tc := range []struct {
		name string
		err  error
		refs map[string]CommitReference
		code string
	}{
		{name: "forbidden", err: errors.New("403 Resource not accessible by integration private-provider-response"), code: "source_links"},
		{name: "timeout", err: context.DeadlineExceeded, code: "timeout"},
		{name: "omitted identity", refs: map[string]CommitReference{}, code: "source_links"},
		{name: "mismatched target", refs: map[string]CommitReference{sha: {PullRequests: []PullRequestReference{{Number: 1754, URL: "https://github.com/djdembeck/annalist/pull/1779"}}}}, code: "source_links"},
		{name: "foreign target", refs: map[string]CommitReference{sha: {PullRequests: []PullRequestReference{{Number: 1754, URL: "https://evil.invalid/djdembeck/annalist/pull/1754"}}}}, code: "source_links"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pip, stub, platform, store := fixtureWithStub(t, newStub("- Export archives. "+engine.SourceToken(sha)), nil, map[string]*Release{"v0.2.0": {ID: 1}})
			platform.referenceErr, platform.references = tc.err, tc.refs
			spec := genSpec("v0.2.0", "unresolved")
			result, err := pip.GenerateNotes(context.Background(), spec, Options{Source: "manual", Publish: true, Force: true})
			var sourceErr *SourceReferenceError
			if !errors.As(err, &sourceErr) || result.Notes != "" || strings.Contains(err.Error(), "private-provider-response") || platform.edited != "" || stub.calls != 1 || countGeneratedByReleaseID(t, pip.Cfg.Data.Dir, spec.ReleaseID) != 0 {
				t.Fatalf("source lookup failure escaped: result=%+v err=%v calls=%d", result, err, stub.calls)
			}
			run := onlyRun(t, store)
			encoded, _ := json.Marshal(run)
			if run.Status != "failed" || run.ErrorCode == nil || *run.ErrorCode != tc.code || strings.Contains(string(encoded), "private-provider-response") || strings.Contains(string(encoded), "annalist:") {
				t.Fatalf("unsafe history: %s", encoded)
			}
		})
	}
}

func TestVerifiedMultipleAssociationsAndCommitFallback(t *testing.T) {
	sha := engine.TargetCommitSHA(context.Background(), sharedOrigin, "v0.2.0")
	for _, tc := range []struct {
		name string
		ref  CommitReference
		want string
	}{
		{name: "multiple merged associations", ref: CommitReference{PullRequests: []PullRequestReference{
			{Number: 1779, URL: "https://github.com/djdembeck/annalist/pull/1779"},
			{Number: 1754, URL: "https://github.com/djdembeck/annalist/pull/1754"},
		}}, want: "([#1754](https://github.com/djdembeck/annalist/pull/1754), [#1779](https://github.com/djdembeck/annalist/pull/1779))"},
		{name: "authoritative no association", ref: CommitReference{CommitURL: "https://github.com/djdembeck/annalist/commit/" + sha}, want: "([" + sha[:7] + "](https://github.com/djdembeck/annalist/commit/" + sha + "))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pip, _, platform, _ := fixtureWithStub(t, newStub("- Export archives. "+engine.SourceToken(sha)), nil, map[string]*Release{"v0.2.0": {ID: 1}})
			platform.references = map[string]CommitReference{sha: tc.ref}
			spec := genSpec("v0.2.0", "verified-fallback")
			result, err := pip.GenerateNotes(context.Background(), spec, Options{Publish: true})
			if err != nil || result.Notes != "- Export archives. "+tc.want || result.Outcome != "published" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			platform.referenceErr = errors.New("no cache refresh")
			cached, err := pip.GenerateNotes(context.Background(), spec, Options{})
			if err != nil || cached.Notes != result.Notes || cached.Outcome != "cached" || platform.referenceCalls != 1 {
				t.Fatalf("cached provenance failed: result=%+v err=%v", cached, err)
			}
		})
	}
}

// TestSourceTokenPrefixResolution: a model that copies an eight-character
// object-name prefix is accepted when the prefix names exactly one selected
// commit, and rejected when it is ambiguous or unknown.
func TestSourceTokenPrefixResolution(t *testing.T) {
	// Crafted identities keep the ambiguity case deterministic.
	const a = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const b = "aaaaaaaabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	commits := engine.CommitLog{Commits: []engine.Commit{{SHA: a}, {SHA: b}}}
	unique := engine.CommitLog{Commits: []engine.Commit{{SHA: a}, {SHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}
	for _, tc := range []struct {
		name    string
		log     engine.CommitLog
		notes   string
		wantErr bool
	}{
		{name: "full token", log: commits, notes: "- One change. [[annalist:" + a + "]]\n- Another change. [[annalist:" + b + "]]"},
		{name: "unique prefix", log: unique, notes: "- One change. [[annalist:aaaaaaaa]]\n- Another change. [[annalist:bbbbbbbb]]"},
		{name: "ambiguous prefix", log: commits, notes: "- One change. [[annalist:aaaaaaaa]]\n- Another change. [[annalist:aaaaaaaa]]", wantErr: true},
		{name: "unknown prefix", log: commits, notes: "- One change. [[annalist:ffffffff]]\n- Another change. [[annalist:aaaaaaaa]]", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := attributedSources(tc.notes, tc.log, false)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error=%v", err, tc.wantErr)
			}
		})
	}
}
