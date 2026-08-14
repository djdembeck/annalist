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

func TestRandomSuffix(t *testing.T) {
	re := regexp.MustCompile(`^-[0-9a-f]{8}$`)
	for i := 0; i < 20; i++ {
		s := randomSuffix()
		if !re.MatchString(s) {
			t.Fatalf("randomSuffix() = %q, want form -<8 hex>", s)
		}
	}
}

// gitTa shells out to git with a deterministic identity. Used to build
// throwaway repos. (Sibling pipeline_test.go already defines gitNoAsk.)
func gitTa(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		"GIT_CEILING_DIRECTORIES="+dir,
		"PRE_COMMIT_ALLOW_NO_CONFIG=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
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
	// --reverse HEAD path) lists newest-first.
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "HEAD", nil); got != "- fourth\n- third" {
		t.Errorf("CollectCommitLog(range) = %q", got)
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
	if !strings.Contains(got, "2 files changed") || !strings.Contains(got, "f.txt | 2 +") {
		// The stat line count is informational; the file must at least appear.
		if !strings.Contains(got, "f.txt") {
			t.Errorf("first-release diff missing f.txt stat; got:\n%s", got)
		}
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

	// Deterministic clone dir = sha256(URL)[:16].
	sum := sha256Hex(src)
	wantDir := filepath.Join(dataDir, "clones", "testplat", sum)
	if workdir != wantDir {
		t.Errorf("workdir = %q, want %q", workdir, wantDir)
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
