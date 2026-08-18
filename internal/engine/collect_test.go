package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
