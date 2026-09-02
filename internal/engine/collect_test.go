package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		tag  string
		want version
		ok   bool
	}{
		{tag: "v1.2.3", want: version{1, 2, 3}, ok: true},
		{tag: "1.2.3", want: version{1, 2, 3}, ok: true},    // missing v prefix is fine
		{tag: "v1.10.0", want: version{1, 10, 0}, ok: true}, // multi-digit components
		{tag: "v0.0.0", want: version{0, 0, 0}, ok: true},
		{tag: "v1.2.3-rc.1", want: version{1, 2, 3}, ok: true},   // prerelease suffix ignored
		{tag: "v1.2.3+build5", want: version{1, 2, 3}, ok: true}, // build suffix ignored
		{tag: "v1.2", ok: false},                                 // only two components
		{tag: "v1", ok: false},
		{tag: "abc", ok: false},
		{tag: "v", ok: false},
		{tag: "", ok: false},
		{tag: "v1.2.x", ok: false}, // non-numeric patch
	}
	for _, tc := range cases {
		got, ok := parseVersion(tc.tag)
		if ok != tc.ok {
			t.Errorf("parseVersion(%q) ok = %v, want %v", tc.tag, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("parseVersion(%q) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b version
		want int
	}{
		{version{1, 0, 0}, version{1, 0, 0}, 0},
		{version{2, 0, 0}, version{1, 99, 99}, 1}, // major dominates minor/patch
		{version{1, 1, 0}, version{1, 10, 0}, -1},
		{version{1, 10, 0}, version{1, 2, 0}, 1},
		{version{1, 0, 5}, version{1, 0, 3}, 1},
		{version{1, 0, 2}, version{1, 0, 10}, -1},
	}
	for _, tc := range cases {
		if got := tc.a.compare(tc.b); got != tc.want {
			t.Errorf("(%+v).compare(%+v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCmpInt(t *testing.T) {
	if got := cmpInt(1, 2); got != -1 {
		t.Errorf("cmpInt(1,2) = %d, want -1", got)
	}
	if got := cmpInt(3, 2); got != 1 {
		t.Errorf("cmpInt(3,2) = %d, want 1", got)
	}
	if got := cmpInt(2, 2); got != 0 {
		t.Errorf("cmpInt(2,2) = %d, want 0", got)
	}
	if got := cmpInt(-2, 1); got != -1 {
		t.Errorf("cmpInt(-2,1) = %d, want -1", got)
	}
}

func TestValidGitRef(t *testing.T) {
	cases := []struct {
		tag  string
		want bool
	}{
		{tag: "v1.0.0", want: true},
		{tag: "v0.9.0-beta.1", want: true},
		{tag: "feature/x", want: true},
		{tag: "", want: false},
		{tag: "-v1.0.0", want: false},
		{tag: "--output=evil", want: false},
		{tag: "-q", want: false},
		{tag: "a..b", want: false},
		{tag: "HEAD@{1}", want: false},
		{tag: "a b", want: false},
		{tag: "a\\b", want: false},
		{tag: "a?", want: false},
		{tag: "a*", want: false},
		{tag: "a:", want: false},
		{tag: "a[b]", want: false},
		{tag: "/abs", want: false},
		{tag: ".hidden", want: false},
		{tag: "trail/", want: false},
		{tag: "trailing.", want: false},
		{tag: "a//b", want: false},
		{tag: "v1\x00.0", want: false},
		{tag: "a~b", want: false},
		{tag: "a^b", want: false},
		{tag: "v1\x7f.0", want: false},
	}
	for _, tc := range cases {
		if got := validGitRef(tc.tag); got != tc.want {
			t.Errorf("validGitRef(%q) = %v, want %v", tc.tag, got, tc.want)
		}
	}
}

func TestRandomSuffix(t *testing.T) {
	re := regexp.MustCompile(`^-[0-9a-f]{8}$`)
	for i := 0; i < 20; i++ {
		s := randomSuffix()
		if !re.MatchString(s) {
			t.Fatalf("randomSuffix() = %q, want form -<8 hex>", s)
		}
	}
}

// gitTaEnv returns the deterministic git environment for throwaway repos.
func gitTaEnv(dir string) []string {
	return append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		"GIT_CEILING_DIRECTORIES="+dir,
		"PRE_COMMIT_ALLOW_NO_CONFIG=1")
}

// gitTa shells out to git with a deterministic identity. Used to build
// throwaway repos. (Sibling pipeline_test.go already defines gitNoAsk.)
func gitTa(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitTaEnv(dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitTaOut is like gitTa but returns trimmed stdout.
func gitTaOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitTaEnv(dir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// sha256Hex returns the first 16 hex chars of sha256(s), matching cloneDir.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

// makeTaggedRepo builds a local repo committing subjects in order, tagging
// specific subjects with the given tags.
func makeTaggedRepo(t *testing.T, dir string, plan []struct {
	subject string
	tag     string
}) string {
	t.Helper()
	gitTa(t, dir, "init", "-q")
	for _, step := range plan {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(step.subject), 0o644); err != nil {
			t.Fatal(err)
		}
		gitTa(t, dir, "add", "-A")
		gitTa(t, dir, "commit", "-q", "-m", step.subject)
		if step.tag != "" {
			gitTa(t, dir, "tag", step.tag)
		}
	}
	return dir
}

func TestResolvePrevTag(t *testing.T) {
	dir := t.TempDir()
	// Order of creation: v1.10.0 is committed BEFORE v1.9.0 so recency differs
	// from version order. ResolvePrevTag must pick the greatest STRICTLY-less
	// version, i.e. v1.9.0 for current v1.10.0, and v1.10.0 for current v1.11.0.
	plan := []struct {
		subject string
		tag     string
	}{
		{subject: "zero", tag: "v0.1.0"},
		{subject: "one", tag: "v1.0.0"},
		{subject: "one ten", tag: "v1.10.0"},
		{subject: "one nine", tag: "v1.9.0"}, // created last, lower version
	}
	makeTaggedRepo(t, dir, plan)

	ctx := context.Background()
	cases := []struct {
		name    string
		current string
		want    string
	}{
		{name: "numeric not recency", current: "v1.10.0", want: "v1.9.0"},
		{name: "highest", current: "v1.11.0", want: "v1.10.0"},
		{name: "first release no less", current: "v0.1.0", want: ""},
		{name: "current equal to a tag", current: "v1.10.0-dup", want: "v1.9.0"},
		{name: "current not in tags", current: "v1.5.0", want: "v1.0.0"},
		{name: "invalid current falls back to greatest", current: "main", want: "v1.10.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolvePrevTag(ctx, dir, tc.current); got != tc.want {
				t.Errorf("ResolvePrevTag(%q) = %q, want %q", tc.current, got, tc.want)
			}
		})
	}
}

func TestResolvePrevTagNoTags(t *testing.T) {
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	if got := ResolvePrevTag(context.Background(), dir, "v1.0.0"); got != "" {
		t.Errorf("ResolvePrevTag on untagged repo = %q, want empty", got)
	}
}

func TestCollectCommitLog(t *testing.T) {
	dir := t.TempDir()
	plan := []struct {
		subject string
		tag     string
	}{
		{subject: "first", tag: ""},
		{subject: "second", tag: "v0.1.0"},
		{subject: "third", tag: ""},
		{subject: "fourth", tag: ""},
	}
	makeTaggedRepo(t, dir, plan)
	ctx := context.Background()

	// Reverse chronological from HEAD yields all subjects oldest-first.
	if got := CollectCommitLog(ctx, dir, "", "", nil); got != "- first\n- second\n- third\n- fourth" {
		t.Errorf("CollectCommitLog(all) = %q", got)
	}
	// Range v0.1.0..HEAD excludes the tagged commit itself and (unlike the
	// --reverse HEAD path) lists newest-first. HEAD is a rev name the
	// validGitRef guard accepts (the fixture's only tag, v0.1.0, is the
	// from-bound, so HEAD is load-bearing here).
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "HEAD", nil); got != "- fourth\n- third" {
		t.Errorf("CollectCommitLog(range) = %q", got)
	}
	// Implicit HEAD: an empty toTag makes git resolve the missing range
	// endpoint to HEAD, so the output must match the explicit v0.1.0..HEAD
	// range above.
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "", nil); got != "- fourth\n- third" {
		t.Errorf("CollectCommitLog(implicit HEAD) = %q", got)
	}
	// Equal bounds yield no commits.
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "v0.1.0", nil); got != "" {
		t.Errorf("CollectCommitLog(equal) = %q, want empty", got)
	}
}

func TestParseCommitTypes(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{in: "", want: nil},
		{in: "feat", want: []string{"feat"}},
		{in: "fix,feat", want: []string{"fix", "feat"}},
		{in: " fix , feat , refactor ", want: []string{"fix", "feat", "refactor"}},
		{in: ",,,", want: nil},
		{in: "feat,", want: []string{"feat"}},
		{in: ",feat", want: []string{"feat"}},
	}
	for _, tc := range cases {
		got := ParseCommitTypes(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("ParseCommitTypes(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("ParseCommitTypes(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestFilterCommitLog(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		types []string
		want  string
	}{
		{
			name:  "include matches kept",
			raw:   "- feat: add login\x00- fix: patch bug\x00",
			types: []string{"feat"},
			want:  "- feat: add login",
		},
		{
			name:  "non-include typed dropped",
			raw:   "- chore: tidy\x00- feat: add login\x00",
			types: []string{"feat"},
			want:  "- feat: add login",
		},
		{
			name:  "bang shorthand kept",
			raw:   "- feat!: break api\x00- chore: tidy\x00",
			types: []string{"fix"},
			want:  "- feat!: break api",
		},
		{
			name:  "breaking change trailer kept with body",
			raw:   "- docs: update readme\n\nBREAKING CHANGE: api removed\x00- chore: tidy\x00",
			types: []string{"fix"},
			want:  "- docs: update readme\n\nBREAKING CHANGE: api removed",
		},
		{
			name:  "typed non-breaking body kept",
			raw:   "- feat: add login\n\nAdds OAuth login with refresh token rotation.\x00- chore: tidy\x00",
			types: []string{"feat"},
			want:  "- feat: add login\n\nAdds OAuth login with refresh token rotation.",
		},
		{
			name:  "typed non-breaking body kept no filter",
			raw:   "- fix: patch bug\n\nFixes the crash on empty input.\x00",
			types: nil,
			want:  "- fix: patch bug\n\nFixes the crash on empty input.",
		},
		{
			name:  "untyped body kept",
			raw:   "- Merge pull request #1\n\nImports the new dashboard.\x00- feat: add login\x00",
			types: []string{"feat"},
			want:  "- Merge pull request #1\n\nImports the new dashboard.\n- feat: add login",
		},
		{
			name:  "untyped kept",
			raw:   "- Merge pull request #1\x00- feat: add login\x00",
			types: []string{"feat"},
			want:  "- Merge pull request #1\n- feat: add login",
		},
		{
			name:  "nil include keeps all",
			raw:   "- feat: add login\x00- chore: tidy\x00",
			types: nil,
			want:  "- feat: add login\n- chore: tidy",
		},
		{
			name:  "empty include keeps all",
			raw:   "- feat: add login\x00- chore: tidy\x00",
			types: []string{},
			want:  "- feat: add login\n- chore: tidy",
		},
		{
			name:  "scope parsing",
			raw:   "- feat(api): new endpoint\x00- chore: tidy\x00",
			types: []string{"feat"},
			want:  "- feat(api): new endpoint",
		},
		{
			name:  "uppercase subject lowercase includeTypes kept",
			raw:   "- FIX: patch bug\x00- chore: tidy\x00",
			types: []string{"fix"},
			want:  "- FIX: patch bug",
		},
		{
			name:  "lowercase subject uppercase includeTypes kept",
			raw:   "- fix: patch bug\x00- chore: tidy\x00",
			types: []string{"FIX"},
			want:  "- fix: patch bug",
		},
		{
			name:  "mixed case drop",
			raw:   "- docs: update\x00- feat: add login\x00",
			types: []string{"fix", "feat"},
			want:  "- feat: add login",
		},
		{
			name:  "hyphen BREAKING-CHANGE variant kept with body",
			raw:   "- chore: cleanup\n\nBREAKING-CHANGE: removed flag\x00",
			types: []string{"feat"},
			want:  "- chore: cleanup\n\nBREAKING-CHANGE: removed flag",
		},
		{
			name:  "untyped breaking with body plus matching type",
			raw:   "- Removed old flag\n\nBREAKING CHANGE: old flag gone\x00- feat: x\x00",
			types: []string{"feat"},
			want:  "- Removed old flag\n\nBREAKING CHANGE: old flag gone\n- feat: x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterCommitLog(tc.raw, tc.types)
			if got != tc.want {
				t.Errorf("FilterCommitLog() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSelectHunks(t *testing.T) {
	// Each unit costs header + hunk = 25 + 15 = 40 bytes on first emit
	// per file; a second hunk of the same file costs only its 15 bytes.
	a := diffUnit{class: "source", filePath: "a.go", fileHeader: "diff --git a/a.go b/a.go\n", hunkText: strings.Repeat("a", 15), origIndex: 0}
	b := diffUnit{class: "source", filePath: "b.go", fileHeader: "diff --git a/b.go b/b.go\n", hunkText: strings.Repeat("b", 15), origIndex: 1}
	d := diffUnit{class: "docs", filePath: "x.md", fileHeader: "diff --git a/x.md b/x.md\n", hunkText: strings.Repeat("x", 15), origIndex: 2}

	// Case A: budget 90 fits both source hunks (80) but not the docs hunk.
	out, skipped := selectHunks([]diffUnit{a, b, d}, 90)
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if !strings.Contains(out, "a.go") || !strings.Contains(out, strings.Repeat("a", 15)) {
		t.Errorf("output missing a.go hunk; got:\n%s", out)
	}
	if !strings.Contains(out, "b.go") || !strings.Contains(out, strings.Repeat("b", 15)) {
		t.Errorf("output missing b.go hunk; got:\n%s", out)
	}
	if strings.Contains(out, "x.md") || strings.Contains(out, strings.Repeat("x", 15)) {
		t.Errorf("output must exclude docs hunk; got:\n%s", out)
	}
	// Within the source class, file path orders a.go before b.go.
	if i, j := strings.Index(out, strings.Repeat("a", 15)), strings.Index(out, strings.Repeat("b", 15)); i > j {
		t.Errorf("a.go hunk must precede b.go hunk; got:\n%s", out)
	}

	// Case B: one source hunk and two docs hunks share a budget that fits
	// the source hunk plus exactly one docs hunk (40 + 40 = 80). Input is
	// deliberately unsorted; output order must be (class priority, file
	// path, origIndex), so the source hunk leads and x.md beats y.md.
	src := diffUnit{class: "source", filePath: "c.go", fileHeader: "diff --git a/c.go b/c.go\n", hunkText: strings.Repeat("s", 15), origIndex: 2}
	dx := diffUnit{class: "docs", filePath: "x.md", fileHeader: "diff --git a/x.md b/x.md\n", hunkText: strings.Repeat("x", 15), origIndex: 0}
	dy := diffUnit{class: "docs", filePath: "y.md", fileHeader: "diff --git a/y.md b/y.md\n", hunkText: strings.Repeat("y", 15), origIndex: 1}
	out2, skipped2 := selectHunks([]diffUnit{dy, src, dx}, 80)
	if skipped2 != 1 {
		t.Errorf("skipped = %d, want 1", skipped2)
	}
	want2 := "diff --git a/c.go b/c.go\n" + strings.Repeat("s", 15) +
		"diff --git a/x.md b/x.md\n" + strings.Repeat("x", 15)
	if out2 != want2 {
		t.Errorf("output = %q, want %q", out2, want2)
	}

	// Case C: class priority must beat lexicographic path order. The docs
	// path "a.md" precedes the source path "z.go", so a class-blind
	// comparator (path, then index) would pick a.md under a budget that
	// fits only one unit; the real comparator must pick z.go.
	docA := diffUnit{class: "docs", filePath: "a.md", fileHeader: "diff --git a/a.md b/a.md\n", hunkText: strings.Repeat("A", 15), origIndex: 0}
	srcZ := diffUnit{class: "source", filePath: "z.go", fileHeader: "diff --git a/z.go b/z.go\n", hunkText: strings.Repeat("Z", 15), origIndex: 1}
	out4, skipped4 := selectHunks([]diffUnit{docA, srcZ}, 40)
	if skipped4 != 1 {
		t.Errorf("class vs path: skipped = %d, want 1", skipped4)
	}
	want4 := "diff --git a/z.go b/z.go\n" + strings.Repeat("Z", 15)
	if out4 != want4 {
		t.Errorf("class vs path: output = %q, want %q (source must beat docs even though a.md < z.go)", out4, want4)
	}

	// Header is charged only once per file: two 35-byte hunks with a 10-byte
	// header cost 45 + 35 = 80 (not 45 + 45 = 90), so a 80-byte budget fits
	// both.
	h := diffUnit{class: "source", filePath: "m.go", fileHeader: "H012345678", hunkText: strings.Repeat("m", 35), origIndex: 0}
	h2 := diffUnit{class: "source", filePath: "m.go", fileHeader: "H012345678", hunkText: strings.Repeat("n", 35), origIndex: 1}
	out3, skipped3 := selectHunks([]diffUnit{h, h2}, 80)
	if skipped3 != 0 || strings.Count(out3, "diff --git") != 0 ||
		!strings.Contains(out3, strings.Repeat("m", 35)) || !strings.Contains(out3, strings.Repeat("n", 35)) {
		t.Errorf("same-file hunks: skipped = %d, out = %q (want both hunks, one header)", skipped3, out3)
	}
}

func TestParseDiffBinary(t *testing.T) {
	patch := "diff --git a/img.png b/img.png\n" +
		"Binary files a/img.png and b/img.png differ\n" +
		"diff --git a/main.go b/main.go\n" +
		"--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -1 +1,2 @@\n" +
		"-old\n" +
		"+new\n" +
		"+extra\n"
	units := parseDiff(patch)
	if len(units) != 1 {
		t.Fatalf("len(units) = %d, want 1 (binary section must contribute nothing)", len(units))
	}
	u := units[0]
	if u.filePath != "main.go" || u.class != "source" {
		t.Errorf("unit = (%q, %q), want (main.go, source)", u.filePath, u.class)
	}
	if !strings.Contains(u.hunkText, "-old") || !strings.Contains(u.hunkText, "+extra") {
		t.Errorf("hunk text missing content; got:\n%s", u.hunkText)
	}
	if strings.Contains(u.fileHeader, "Binary files") {
		t.Errorf("binary marker leaked into header; got:\n%s", u.fileHeader)
	}

	// A literal "diff --git " line INSIDE hunk content (e.g. a committed
	// patch file) must not split the patch into a phantom section. Only the
	// line-anchored section start counts, and the literal must survive in
	// the hunk text.
	embedded := "+diff --git a/fake.txt b/fake.txt\n"
	patch2 := "diff --git a/notes.patch b/notes.patch\n" +
		"--- a/notes.patch\n" +
		"+++ b/notes.patch\n" +
		"@@ -0,0 +1,2 @@\n" +
		"++- commit a\n" +
		embedded +
		"diff --git a/real.go b/real.go\n" +
		"--- a/real.go\n" +
		"+++ b/real.go\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n"
	units2 := parseDiff(patch2)
	if len(units2) != 2 {
		t.Fatalf("len(units) = %d, want 2 (no phantom section from embedded literal)", len(units2))
	}
	if units2[0].filePath != "notes.patch" {
		t.Errorf("first unit = %q, want notes.patch", units2[0].filePath)
	}
	if !strings.Contains(units2[0].hunkText, embedded) {
		t.Errorf("embedded literal must stay in hunk content; got:\n%s", units2[0].hunkText)
	}
	if strings.Contains(units2[0].hunkText, "real.go") {
		t.Errorf("next section leaked into first hunk; got:\n%s", units2[0].hunkText)
	}
	if units2[1].filePath != "real.go" {
		t.Errorf("second unit = %q, want real.go", units2[1].filePath)
	}
}

func TestDiffFilePath(t *testing.T) {
	cases := []struct {
		name    string
		section string
		want    string
	}{
		{
			name:    "rename prefers b path",
			section: "diff --git a/old.txt b/new.txt\n--- a/old.txt\n+++ b/new.txt\n@@ -1 +1 @@\n-x\n+y\n",
			want:    "new.txt",
		},
		{
			name:    "deletion falls back to a path",
			section: "diff --git a/old.txt b/dev/null\n--- a/old.txt\n+++ /dev/null\n@@ -1 +0 @@\n-x\n",
			want:    "old.txt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := diffFilePath(tc.section); got != tc.want {
				t.Errorf("diffFilePath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCollectCommitLogBreakingChange(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Build a repo with a breaking change commit (body via -m -m)
	gitTa(t, dir, "init", "-q")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "feat: baseline")
	gitTa(t, dir, "tag", "v1.0.0")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "feat!: break api", "-m", "BREAKING CHANGE: the old api is removed")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "chore: tidy")

	// Breaking commit body should survive even when chore is filtered
	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"feat"})
	if !strings.Contains(got, "feat!: break api") {
		t.Errorf("breaking commit missing; got %q", got)
	}
	if !strings.Contains(got, "BREAKING CHANGE: the old api is removed") {
		t.Errorf("breaking change body missing; got %q", got)
	}
	if strings.Contains(got, "chore: tidy") {
		t.Errorf("chore should be filtered; got %q", got)
	}
}

func TestCollectCommitLogExcludesMerges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	//   P (base, tagged v1.0.0)
	//    ├── main:
	//    └── feature: fix: branch work
	//    Merge of feature into main: subject "Merge branch 'feature' into
	//    main" (no conventional type) with a body carrying a non-selected
	//    type ("chore: bump deps"). The untyped merge subject would be kept
	//    by FilterCommitLog's untyped rule, so without --no-merges the merge
	//    subject AND its chore body leak into the filtered log.
	gitTa(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: base")
	gitTa(t, dir, "tag", "v1.0.0")
	baseBranch := gitTaOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD")

	gitTa(t, dir, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "fix: branch work")

	gitTa(t, dir, "checkout", "-q", baseBranch)
	gitTa(t, dir, "merge", "-q", "--no-commit", "--no-ff", "feature")
	gitTa(t, dir, "commit", "-q", "-m", "Merge branch 'feature' into main", "-m", "chore: bump deps")

	// Guard the premise: HEAD must be a real two-parent merge.
	if parents := strings.Fields(gitTaOut(t, dir, "rev-list", "--parents", "-n", "1", "HEAD")); len(parents) != 3 {
		t.Fatalf("fixture broken: HEAD is not a two-parent merge (rev-list: %q)", strings.Join(parents, " "))
	}

	// With a "fix" type filter in effect: branch work (fix) is retained, the
	// merge subject and its chore body are absent.
	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"fix"})
	if got != "- fix: branch work" {
		t.Fatalf("CollectCommitLog = %q, want exactly %q (merge commit must be omitted)", got, "- fix: branch work")
	}
}

func TestClassifyFile(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"src/main.go", "source"},
		{"pkg/util/foo.py", "source"},
		{"readme.md", "docs"},
		{"docs/guide.md", "docs"},
		{"doc/notes.md", "docs"},
		{"CHANGELOG.md", "docs"},
		{"src/test/thing.go", "test"},
		{"tests/suite/test_x.go", "test"},
		{"js/__tests__/comp.test.ts", "test"},
		{"test_foo.py", "test"},
		{"foo_test.go", "test"},
		{"bar_test.py", "test"},
		{"comp.test.ts", "test"},
		{"comp.spec.js", "test"},
		{"package.json", "config"},
		{"go.mod", "config"},
		{"go.sum", "config"},
		{".github/workflows/ci.yml", "config"},
		{".forgejo/actions/build.yml", "config"},
		{"infra/main.tf", "config"},
		{"terraform/prod.tf", "config"},
		{"k8s/deploy.yaml", "config"},
		{"config/app.txt", "source"}, // extension outside the config set falls through
	}
	for _, tc := range cases {
		if got := ClassifyFile(tc.path); got != tc.want {
			t.Errorf("ClassifyFile(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestCollectDiff(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	plan := []struct {
		subject string
		tag     string
	}{
		{subject: "one", tag: "v0.1.0"},
		{subject: "two", tag: "v0.2.0"},
	}
	makeTaggedRepo(t, dir, plan)

	got := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", DiffBudgetBytes)
	if got == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(got, "f.txt") {
		t.Errorf("diff missing f.txt; got:\n%s", got)
	}
	// The --stat summary is always included.
	if !strings.Contains(got, "1 file changed") {
		t.Errorf("diff missing --stat summary; got:\n%s", got)
	}
	// The patch hunk is present when the budget is generous.
	if !strings.Contains(got, "@@") {
		t.Errorf("diff missing hunks; got:\n%s", got)
	}
}

func TestCollectDiffThreeDotNonLinear(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// Non-linear history where merge-base(v0.1.0, v0.2.0) != v0.1.0:
	//   P (base)
	//    ├── rel:    A (file-a.txt), tagged v0.1.0
	//    └── hotfix: C (file-c.txt), tagged v0.2.0
	// v0.2.0 does NOT contain v0.1.0 as an ancestor, so the merge-base is P,
	// the parent of A. The three-dot diff (v0.1.0...v0.2.0 = P..C) shows only
	// file-c.txt added; the two-dot diff (v0.2.0 vs v0.1.0 = A..C) also shows
	// file-a.txt as deleted (it exists at A but not at C). On linear history
	// the two ranges are byte-identical, so the two-dot mutation is only
	// caught when the merge base differs from the from-tag.
	gitTa(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: base")
	p := gitTaOut(t, dir, "rev-parse", "HEAD")

	gitTa(t, dir, "checkout", "-q", "-b", "rel")
	if err := os.WriteFile(filepath.Join(dir, "file-a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "feat: add file a")
	gitTa(t, dir, "tag", "v0.1.0")
	a := gitTaOut(t, dir, "rev-parse", "HEAD")

	gitTa(t, dir, "checkout", "-q", "-b", "hotfix", p)
	if err := os.WriteFile(filepath.Join(dir, "file-c.txt"), []byte("c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "fix: add file c")
	gitTa(t, dir, "tag", "v0.2.0")

	// Guard the premise: merge-base must be strictly below v0.1.0.
	if mb := gitTaOut(t, dir, "merge-base", "v0.1.0", "v0.2.0"); mb == a {
		t.Fatal("fixture broken: merge-base(v0.1.0, v0.2.0) must not equal v0.1.0")
	}

	// The two range shapes really differ on this history — pin that so the
	// test fails loudly if git behavior or the fixture ever changes.
	three := gitTaOut(t, dir, "diff", "--patch", "-U3", "v0.1.0...v0.2.0")
	two := gitTaOut(t, dir, "diff", "--patch", "-U3", "v0.1.0..v0.2.0")
	if three == two {
		t.Fatalf("fixture broken: three-dot and two-dot diffs are identical; test cannot discriminate")
	}
	if !strings.Contains(three, "file-c.txt") || strings.Contains(three, "file-a.txt") {
		t.Fatalf("fixture broken: expected three-dot diff to contain only file-c.txt; got:\n%s", three)
	}

	diff := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", DiffBudgetBytes)
	if !strings.Contains(diff, "file-c.txt") || !strings.Contains(diff, "+c") {
		t.Errorf("three-dot diff missing file-c.txt; got:\n%s", diff)
	}
	if strings.Contains(diff, "file-a.txt") {
		t.Errorf("three-dot diff must not show file-a.txt (merge base is below v0.1.0); got:\n%s", diff)
	}
}

func TestCollectDiffBudget(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// Two large, distinguishable files so a tight budget must skip some hunks.
	gitTa(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "first")
	gitTa(t, dir, "tag", "v0.1.0")

	var bigA, bigB strings.Builder
	for i := range 40 {
		fmt.Fprintf(&bigA, "line %d of a\n", i)
		fmt.Fprintf(&bigB, "line %d of b\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(bigA.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte(bigB.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "second")
	gitTa(t, dir, "tag", "v0.2.0")

	full := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", DiffBudgetBytes)
	if !strings.Contains(full, "a.go") || !strings.Contains(full, "b.md") {
		t.Fatalf("full diff missing files; got:\n%s", full)
	}
	if strings.Contains(full, "[diff truncated") {
		t.Errorf("unbudgeted diff should not truncate; got:\n%s", full)
	}

	// A 1-byte budget can emit at most one tiny hunk header's worth; most
	// hunks must be skipped and the truncation note emitted.
	tiny := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", 1)
	if !strings.Contains(tiny, "[diff truncated") {
		t.Errorf("tight budget should note truncation; got:\n%s", tiny)
	}
	// The --stat summary is always present, regardless of budget.
	if !strings.Contains(tiny, "2 files changed") {
		t.Errorf("stat summary must be present under budget; got:\n%s", tiny)
	}
	// With a 1-byte budget no hunk body can fit, so no @@ content is emitted.
	if strings.Contains(tiny, "@@") {
		t.Errorf("1-byte budget should skip all hunks; got:\n%s", tiny)
	}
}

func TestCollectDiffFirstRelease(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	plan := []struct {
		subject string
		tag     string
	}{
		{subject: "one", tag: ""},
		{subject: "two", tag: "v0.2.0"},
	}
	makeTaggedRepo(t, dir, plan)

	// fromTag == "" diffs against the empty tree: the whole history up to v0.2.0.
	got := CollectDiff(ctx, dir, "", "v0.2.0", DiffBudgetBytes)
	if got == "" {
		t.Fatal("expected non-empty first-release diff")
	}
	if !strings.Contains(got, "f.txt") {
		t.Errorf("first-release diff missing f.txt; got:\n%s", got)
	}
	// The fixture commits a single file with a single line, so the stat
	// summary is exactly "1 file changed, 1 insertion(+)" / "f.txt | 1 +".
	if !strings.Contains(got, "1 file changed") || !strings.Contains(got, "f.txt | 1 +") {
		t.Errorf("first-release stat summary wrong; got:\n%s", got)
	}
	if !strings.Contains(got, "two") {
		t.Errorf("first-release diff should contain the final file content; got:\n%s", got)
	}
}

func TestCollectDiffGitFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// No git repo here: git diff must fail and CollectDiff returns "".
	if got := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", DiffBudgetBytes); got != "" {
		t.Errorf("expected empty diff on git failure, got:\n%s", got)
	}
}

// fakeGitDir creates a temp dir containing a `git` shell script that records
// any invocation by touching a marker file (path in $FAKE_GIT_MARKER) and
// exits 128. Prepending the dir to PATH proves whether a git exec happened at
// all: a bad tag that slips past the guard would leave the marker behind,
// even though the function returns "" either way.
func fakeGitDir(t *testing.T) (dir, marker string) {
	t.Helper()
	dir = t.TempDir()
	marker = filepath.Join(dir, "executed")
	script := "#!/bin/sh\ntouch \"$FAKE_GIT_MARKER\"\nexit 128\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_GIT_MARKER", marker)
	return dir, marker
}

func TestCollectDiffRejectsBadTags(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	plan := []struct {
		subject string
		tag     string
	}{
		{subject: "one", tag: "v0.1.0"},
		{subject: "two", tag: "v0.2.0"},
	}
	makeTaggedRepo(t, dir, plan)

	// Option-injection attempts must be rejected before any git exec.
	t.Run("badFromTag", func(t *testing.T) {
		fakeDir, marker := fakeGitDir(t)
		t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		if got := CollectDiff(ctx, dir, "--output=/tmp/evil", "v0.2.0", DiffBudgetBytes); got != "" {
			t.Errorf("expected empty diff for bad fromTag, got:\n%s", got)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("git was executed for bad fromTag; guard must reject before exec")
		}
	})
	t.Run("badToTag", func(t *testing.T) {
		fakeDir, marker := fakeGitDir(t)
		t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		if got := CollectDiff(ctx, dir, "v0.1.0", "-q", DiffBudgetBytes); got != "" {
			t.Errorf("expected empty diff for bad toTag, got:\n%s", got)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("git was executed for bad toTag; guard must reject before exec")
		}
	})
	// Control: valid tags still work (the 1-byte budget forces a truncation
	// note, but the output must still be non-empty). Runs under the real PATH
	// (t.Setenv restored it when the subtests above ended).
	if got := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", 1); got == "" {
		t.Error("expected non-empty diff for valid tags")
	}
	// Control: implicit HEAD — an empty toTag makes git resolve the missing
	// range endpoint to HEAD, so the diff covers the commit(s) after v0.1.0
	// (here the "two" commit, which rewrites f.txt).
	if got := CollectDiff(ctx, dir, "v0.1.0", "", DiffBudgetBytes); got == "" || !strings.Contains(got, "f.txt") {
		t.Errorf("expected non-empty implicit-HEAD diff mentioning f.txt; got:\n%s", got)
	}
}

func TestCollectCommitLogRejectsBadTags(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	plan := []struct {
		subject string
		tag     string
	}{
		{subject: "one", tag: "v0.1.0"},
		{subject: "two", tag: "v0.2.0"},
	}
	makeTaggedRepo(t, dir, plan)

	// Option-injection attempts must be rejected before any git exec.
	t.Run("badFromTag", func(t *testing.T) {
		fakeDir, marker := fakeGitDir(t)
		t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		if got := CollectCommitLog(ctx, dir, "--output=/tmp/evil", "v0.2.0", nil); got != "" {
			t.Errorf("expected empty log for bad fromTag, got:\n%s", got)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("git was executed for bad fromTag; guard must reject before exec")
		}
	})
	t.Run("badToTag", func(t *testing.T) {
		fakeDir, marker := fakeGitDir(t)
		t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		if got := CollectCommitLog(ctx, dir, "v0.1.0", "--output=/tmp/evil", nil); got != "" {
			t.Errorf("expected empty log for bad toTag, got:\n%s", got)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("git was executed for bad toTag; guard must reject before exec")
		}
	})
	// Control: valid tags still work. Runs under the real PATH (t.Setenv
	// restored it when the subtests above ended).
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "v0.2.0", nil); got == "" {
		t.Error("expected non-empty log for valid tags")
	}
}

func TestCollectCommitLogBodies(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	gitTa(t, dir, "init", "-q")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "feat: baseline")
	gitTa(t, dir, "tag", "v1.0.0")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "feat: add import queue", "-m", "Imports queue behind an explicit worker pool so large libraries do not block the UI thread.")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "fix: correct chapter offset drift", "-m", "Offsets now come from container timestamps instead of the codec header.")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "chore: bump deps")

	// Bodies of kept typed non-breaking commits survive filtering.
	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"feat", "fix"})
	want := "- fix: correct chapter offset drift\n\nOffsets now come from container timestamps instead of the codec header.\n- feat: add import queue\n\nImports queue behind an explicit worker pool so large libraries do not block the UI thread."
	if got != want {
		t.Errorf("CollectCommitLog(bodies) = %q, want %q", got, want)
	}
	if strings.Contains(got, "chore: bump deps") {
		t.Errorf("chore should be filtered; got %q", got)
	}
}

func TestCloneToLocal(t *testing.T) {
	// Local-path clone: fixture repo built directly (Git accepts a plain path
	// as a clone source; no HTTP involved, so the injected header is inert).
	src := t.TempDir()
	plan := []struct {
		subject string
		tag     string
	}{
		{subject: "first", tag: ""},
		{subject: "second", tag: ""},
	}
	makeTaggedRepo(t, src, plan)

	dataDir := t.TempDir()
	workdir, cleanup, err := CloneTo(context.Background(), dataDir, "testplat", src, "Bearer sekret")
	if err != nil {
		t.Fatalf("CloneTo: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workdir, "f.txt")); statErr != nil {
		t.Errorf("cloned workdir missing f.txt: %v", statErr)
	}

	// Clone dir = sha256(URL)[:16] plus a unique random suffix (always
	// present so concurrent clones of the same URL never share a workdir).
	sum := sha256Hex(src)
	wantPrefix := filepath.Join(dataDir, "clones", "testplat", sum+"-")
	if !strings.HasPrefix(workdir, wantPrefix) {
		t.Errorf("workdir = %q, want prefix %q", workdir, wantPrefix)
	}

	cleanup()
	if _, statErr := os.Stat(workdir); !os.IsNotExist(statErr) {
		t.Errorf("cleanup did not remove %q", workdir)
	}
}

func TestCloneToCollision(t *testing.T) {
	src := t.TempDir()
	plan := []struct {
		subject string
		tag     string
	}{{subject: "only", tag: ""}}
	makeTaggedRepo(t, src, plan)

	dataDir := t.TempDir()
	sum := sha256Hex(src)
	// Pre-create the deterministic clone dir plus a marker file so cloneDir
	// appends a random suffix instead of colliding.
	stale := filepath.Join(dataDir, "clones", "testplat", sum)
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	workdir, cleanup, err := CloneTo(context.Background(), dataDir, "testplat", src, "Bearer x")
	if err != nil {
		t.Fatalf("CloneTo: %v", err)
	}
	if workdir == stale {
		t.Error("clone collided with pre-existing dir, expected random suffix")
	}
	if !strings.HasPrefix(filepath.Base(workdir), sum+"-") {
		t.Errorf("workdir base = %q, want prefix %q-", filepath.Base(workdir), sum)
	}
	cleanup()
}
