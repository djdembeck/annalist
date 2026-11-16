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
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		tag  string
		want Version
		ok   bool
	}{
		{tag: "v1.2.3", want: Version{1, 2, 3}, ok: true},
		{tag: "1.2.3", want: Version{1, 2, 3}, ok: true},    // missing v prefix is fine
		{tag: "v1.10.0", want: Version{1, 10, 0}, ok: true}, // multi-digit components
		{tag: "v0.0.0", want: Version{0, 0, 0}, ok: true},
		{tag: "v1.2.3-rc.1", want: Version{1, 2, 3}, ok: true},   // prerelease suffix ignored
		{tag: "v1.2.3+build5", want: Version{1, 2, 3}, ok: true}, // build suffix ignored
		{tag: "v1.2", ok: false},                                 // only two components
		{tag: "1.2", ok: false},
		{tag: "v1", ok: false},
		{tag: "abc", ok: false},
		{tag: "dev", ok: false},
		{tag: "v", ok: false},
		{tag: "", ok: false},
		{tag: "v1.2.x", ok: false},                          // non-numeric patch
		{tag: "v1.2.3.4", want: Version{1, 2, 3}, ok: true}, // trailing extra components ignored
		{tag: "v1.0.0-extra.2", want: Version{1, 0, 0}, ok: true},
		{tag: "v01.2.3", want: Version{1, 2, 3}, ok: true}, // leading zeros parse
	}
	for _, tc := range cases {
		got, ok := ParseTagVersion(tc.tag)
		if ok != tc.ok {
			t.Errorf("ParseTagVersion(%q) ok = %v, want %v", tc.tag, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("ParseTagVersion(%q) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b Version
		want int
	}{
		{Version{1, 0, 0}, Version{1, 0, 0}, 0},
		{Version{2, 0, 0}, Version{1, 99, 99}, 1}, // major dominates minor/patch
		{Version{1, 1, 0}, Version{1, 10, 0}, -1},
		{Version{1, 10, 0}, Version{1, 2, 0}, 1},
		{Version{1, 0, 5}, Version{1, 0, 3}, 1},
		{Version{1, 0, 2}, Version{1, 0, 10}, -1},
	}
	for _, tc := range cases {
		if got := tc.a.Compare(tc.b); got != tc.want {
			t.Errorf("(%+v).Compare(%+v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// The prefixed-tag boundary the release flow meets: split/prefix/accept for
// each case is pinned to the accepted legacy semantics, independent of any
// consumer reimplementing the split.
func TestSplitPrefixedTag(t *testing.T) {
	cases := []struct {
		tag        string
		wantPrefix string
		wantVer    string
		ok         bool
	}{
		{tag: "docs/v1.0.2", wantPrefix: "docs/", wantVer: "v1.0.2", ok: true},
		{tag: "ggsimpleads/v1.0.2", wantPrefix: "ggsimpleads/", wantVer: "v1.0.2", ok: true},
		{tag: "docs/1.0.2", wantPrefix: "docs/", wantVer: "1.0.2", ok: true}, // no v in the version is fine
		{tag: "docs/v1.0", ok: false},                                  // version segment too short
		{tag: "/v1.2.3", wantPrefix: "/", wantVer: "v1.2.3", ok: true}, // leading-slash prefix accepted
		{tag: "docs/", ok: false},                                      // empty version
		{tag: "//", ok: false},
		{tag: "v1.2.3", ok: false}, // plain tag: no prefix to split
		{tag: "docs/v1.2", ok: false},
	}
	for _, tc := range cases {
		prefix, ver, ok := SplitPrefixedTag(tc.tag)
		if ok != tc.ok {
			t.Errorf("SplitPrefixedTag(%q) ok = %v, want %v", tc.tag, ok, tc.ok)
			continue
		}
		if ok && (prefix != tc.wantPrefix || ver != tc.wantVer) {
			t.Errorf("SplitPrefixedTag(%q) = (%q, %q), want (%q, %q)",
				tc.tag, prefix, ver, tc.wantPrefix, tc.wantVer)
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

// TestResolvePrevTagPrefixed covers per-artifact release streams: a prefixed
// tag (<prefix>/vX.Y.Z) resolves the previous tag of the SAME prefix, never an
// unrelated stream or the repo-wide stream, and a prefix's first tag resolves
// to "" (its stream's first release).
func TestResolvePrevTagPrefixed(t *testing.T) {
	dir := t.TempDir()
	plan := []struct {
		subject string
		tag     string
	}{
		{subject: "repo zero", tag: "v0.1.0"},
		{subject: "ads first", tag: "ggsimpleads/v1.0.0"},
		{subject: "block first", tag: "customfblock/v1.0.0"},
		{subject: "ads second", tag: "ggsimpleads/v1.0.1"},
		{subject: "ads tenth", tag: "ggsimpleads/v1.0.10"}, // numeric order beats recency
		{subject: "repo nine", tag: "v9.9.9"},
	}
	makeTaggedRepo(t, dir, plan)

	ctx := context.Background()
	cases := []struct {
		name    string
		current string
		want    string
	}{
		{name: "previous of same stream", current: "ggsimpleads/v1.0.1", want: "ggsimpleads/v1.0.0"},
		{name: "numeric order within stream", current: "ggsimpleads/v1.0.10", want: "ggsimpleads/v1.0.1"},
		{name: "first of stream ignores repo-wide tags", current: "ggsimpleads/v1.0.0", want: ""},
		{name: "first of another stream", current: "customfblock/v1.0.0", want: ""},
		{name: "next of another stream", current: "customfblock/v1.0.1", want: "customfblock/v1.0.0"},
		{name: "unknown stream has no predecessor", current: "luascan/v1.0.0", want: ""},
		{name: "unprefixed unchanged", current: "v9.9.9", want: "v0.1.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolvePrevTag(ctx, dir, tc.current); got != tc.want {
				t.Errorf("ResolvePrevTag(%q) = %q, want %q", tc.current, got, tc.want)
			}
		})
	}
}

func TestTagCommitSHA(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
	// Lightly tagged: the commit itself.
	gitTa(t, dir, "tag", "light")
	// Annotated: must unwrap to the same commit.
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "second")
	gitTa(t, dir, "tag", "-a", "annot", "-m", "tag message")

	firstSHA := gitTaOut(t, dir, "rev-parse", "HEAD~1")
	secondSHA := gitTaOut(t, dir, "rev-parse", "HEAD")

	if got := TagCommitSHA(ctx, dir, "light"); got != firstSHA {
		t.Errorf("TagCommitSHA(light) = %q, want %q", got, firstSHA)
	}
	if got := TagCommitSHA(ctx, dir, "annot"); got != secondSHA {
		t.Errorf("TagCommitSHA(annot) = %q, want %q (annotated unwrap)", got, secondSHA)
	}
	if got := TagCommitSHA(ctx, dir, "missing"); got != "" {
		t.Errorf("TagCommitSHA(missing) = %q, want empty", got)
	}
	if got := TagCommitSHA(ctx, dir, "ref/../x"); got != "" {
		t.Errorf("TagCommitSHA(bad ref) = %q, want empty", got)
	}
}

func TestIsAncestorCommit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "a")
	a := gitTaOut(t, dir, "rev-parse", "HEAD")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "b")
	b := gitTaOut(t, dir, "rev-parse", "HEAD")
	// A detached side branch: c is not an ancestor of b.
	base := gitTaOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
	gitTa(t, dir, "checkout", "-q", "--orphan", "side")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "c")
	c := gitTaOut(t, dir, "rev-parse", "HEAD")
	gitTa(t, dir, "checkout", "-q", base)

	if !IsAncestorCommit(ctx, dir, a, a) {
		t.Error("a commit is its own ancestor: want true")
	}
	if !IsAncestorCommit(ctx, dir, a, b) {
		t.Error("a is ancestor of b: want true")
	}
	if IsAncestorCommit(ctx, dir, b, a) {
		t.Error("b is NOT ancestor of a: want false")
	}
	if IsAncestorCommit(ctx, dir, c, b) {
		t.Error("unrelated c is NOT ancestor of b: want false")
	}
	if IsAncestorCommit(ctx, dir, "not-a-sha", b) {
		t.Error("malformed sha: want false (fail closed)")
	}
	if IsAncestorCommit(ctx, dir, a, "0000000000000000000000000000000000000000") {
		t.Error("unborn object: want false (fail closed)")
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
	if got := CollectCommitLog(ctx, dir, "", "", nil).Text(); got != "- first\n- second\n- third\n- fourth" {
		t.Errorf("CollectCommitLog(all) = %q", got)
	}
	// Range v0.1.0..HEAD excludes the tagged commit itself and (unlike the
	// --reverse HEAD path) lists newest-first. HEAD is a rev name the
	// validGitRef guard accepts (the fixture's only tag, v0.1.0, is the
	// from-bound, so HEAD is load-bearing here).
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "HEAD", nil).Text(); got != "- fourth\n- third" {
		t.Errorf("CollectCommitLog(range) = %q", got)
	}
	// Implicit HEAD: an empty toTag makes git resolve the missing range
	// endpoint to HEAD, so the output must match the explicit v0.1.0..HEAD
	// range above.
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "", nil).Text(); got != "- fourth\n- third" {
		t.Errorf("CollectCommitLog(implicit HEAD) = %q", got)
	}
	// Equal bounds yield no commits.
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "v0.1.0", nil).Text(); got != "" {
		t.Errorf("CollectCommitLog(equal) = %q, want empty", got)
	}
	// First release (empty from): the range is bounded at the release's own
	// tag, not at the clone's HEAD, so commits that landed after the tag are
	// excluded.
	if got := CollectCommitLog(ctx, dir, "", "v0.1.0", nil).Text(); got != "- first\n- second" {
		t.Errorf("CollectCommitLog(first release) = %q, want bounded-at-tag history", got)
	}
}

// TestCollectCommitLogEmptyRepeat pins the zero-tree-delta empty-repeat
// exclusion: a commit whose tree equals its first parent's tree is dropped
// only when its full message (subject plus body) duplicates an already
// retained change.
func TestCollectCommitLogEmptyRepeat(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	writeRepoFile(t, dir, "v1")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: base")
	gitTa(t, dir, "tag", "v1.0.0")
	// A real, tree-changing commit, then its zero-tree-delta repeat.
	writeRepoFile(t, dir, "v2")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "fix: apply change", "-m", "Changes the thing.")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "fix: apply change", "-m", "Changes the thing.")
	// A distinct zero-delta commit (different full message): must stay.
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "chore: bump")
	// A second zero-delta commit identical to the first: must be dropped
	// (its twin is retained and the twin is zero-delta, so the first in
	// log order wins).
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "chore: bump")

	// Guard the premise: the last chore: bump (HEAD) is zero-delta with its
	// parent (HEAD~1), which is the first chore: bump.
	if headTree := gitTaOut(t, dir, "rev-parse", "HEAD^{tree}"); headTree != gitTaOut(t, dir, "rev-parse", "HEAD~1^{tree}") {
		t.Fatal("fixture broken: last chore: bump is not zero-delta")
	}
	// And the fix twin (HEAD~2) is zero-delta with the real fix commit.
	if twinTree := gitTaOut(t, dir, "rev-parse", "HEAD~2^{tree}"); twinTree != gitTaOut(t, dir, "rev-parse", "HEAD~3^{tree}") {
		t.Fatal("fixture broken: fix twin is not zero-delta")
	}

	// Range logs are newest-first, so the expected order is reversed
	// relative to commit creation order.
	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", nil).Text()
	want := "- chore: bump\n- fix: apply change\n\nChanges the thing."
	if got != want {
		t.Errorf("CollectCommitLog(empty repeat) = %q, want %q", got, want)
	}

	// Type filter: the filtered-out zero-delta "chore: bump" commits never
	// count as retained, and the fix twin stays with its body.
	got = CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"fix"}).Text()
	if got != "- fix: apply change\n\nChanges the thing." {
		t.Errorf("CollectCommitLog(filter fix) = %q", got)
	}

	// "fix"+"chore": "chore: bump" is retained, so its zero-delta twin is
	// dropped as an empty repeat; the fix-only case above proves the
	// converse — when the twin is filtered out it is not counted as
	// retained and the fix message is kept exactly once.
	got = CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"fix", "chore"}).Text()
	want = "- chore: bump\n- fix: apply change\n\nChanges the thing."
	if got != want {
		t.Errorf("CollectCommitLog(fix+chore) = %q, want %q", got, want)
	}
}

// writeRepoFile writes a single tracked file with the given content, used
// by log fixtures that need a real tree change between commits.
func writeRepoFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCollectCommitLogGitFailure pins the failure contract: on a git
// failure (no repository) the log is empty, which is indistinguishable
// from a genuinely empty range — both are "" and no metadata is invented.
func TestCollectCommitLogGitFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "v0.2.0", nil).Text(); got != "" {
		t.Errorf("expected empty log on git failure, got:\n%s", got)
	}
	if got := CollectCommitLog(ctx, dir, "", "HEAD", nil).Text(); got != "" {
		t.Errorf("expected empty log on git failure (first-release shape), got:\n%s", got)
	}
}

// TestCatFileCommitTreesLargeBatch is a bounded regression for the
// cat-file stdin deadlock: 2000 commit SHAs are about 80 KiB of batch
// input — above the typical 64 KiB pipe capacity — so an implementation
// that fills a StdinPipe before the process starts blocks forever here.
// A plain context deadline on the inner call cannot fail fast against
// that deadlock (git has not started to honor it), so the outer test
// runs the real assertions in a child test-binary process and enforces
// a hard 45 s parent deadline that kills the child even when the inner
// cat-file never started. The repo is built with a single `git
// fast-import` (one process, not 2000 commits' worth).
const largeBatchChildMarker = "ANNALIST_CATFILE_LARGE_BATCH_CHILD"

func TestCatFileCommitTreesLargeBatch(t *testing.T) {
	if os.Getenv(largeBatchChildMarker) != "" {
		runCatFileCommitTreesLargeBatch(t)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.CommandContext(ctx, executable,
		"-test.run", `^TestCatFileCommitTreesLargeBatch$`)
	// Per-child env; the marker selects the child branch without global
	// mutation.
	cmd.Env = append(append([]string{}, os.Environ()...), largeBatchChildMarker+"=1")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("large-batch cat-file child exceeded 45s parent deadline (stdin deadlock?):\n%s", out)
	}
	if err != nil {
		t.Fatalf("large-batch cat-file child failed: %v\n%s", err, out)
	}
}

func runCatFileCommitTreesLargeBatch(t *testing.T) {
	const n = 2000
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")

	var fi strings.Builder
	for i := range n {
		fi.WriteString("commit refs/heads/main\nmark :")
		fmt.Fprintf(&fi, "%d", i+1)
		fi.WriteString("\ncommitter t <t@e.i> 0 +0000\ndata 4\n")
		fmt.Fprintf(&fi, "%04d", i) // exactly 4 bytes, as declared
		if i > 0 {
			fi.WriteString("\nfrom :")
			fmt.Fprintf(&fi, "%d\n", i)
		}
	}
	cmd := exec.Command("git", "fast-import")
	cmd.Dir = dir
	cmd.Env = gitTaEnv(dir)
	cmd.Stdin = strings.NewReader(fi.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git fast-import: %v\n%s", err, out)
	}

	shas := strings.Fields(gitTaOut(t, dir, "rev-list", "--reverse", "main"))
	if len(shas) != n {
		t.Fatalf("fixture broken: %d commits, want %d", len(shas), n)
	}
	if input := n * 41; input <= 64*1024 {
		t.Fatalf("fixture broken: batch input %d bytes must exceed the 64 KiB pipe capacity", input)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	trees := catFileCommitTrees(ctx, dir, shas)
	if len(trees) != n {
		t.Fatalf("catFileCommitTrees resolved %d of %d commits (deadline %v)", len(trees), n, ctx.Err())
	}
	// Spot-check both ends: resolved tree must be the real commit tree.
	for _, i := range []int{0, n - 1} {
		want := gitTaOut(t, dir, "rev-parse", shas[i]+"^{tree}")
		if trees[shas[i]] != want {
			t.Errorf("tree[%d] = %q, want %q", i, trees[shas[i]], want)
		}
	}
}

// TestCollectCommitLogNonLinearFirstParent exercises zero-delta detection
// on non-linear history where the range's from-bound is NOT the first
// parent of an in-log commit: the batched first-parent tree lookup must
// resolve the parent from below the range, and the zero-tree-delta twin
// must still be excluded.
//
//	P (base)
//	 ├── rel:    A (feat: real), E (empty twin of A), tagged v0.1.0
//	 └── hot:    B (fix: real), tagged v0.2.0
//
// Range v0.1.0..v0.2.0 is only B: its first parent P is below the range
// and must be resolved by the batched lookup. The whole-history log
// bounded at the rel tag v0.1.0 covers A and E (the rel side is not
// reachable from v0.2.0), so the twin exclusion is asserted there.
func TestCollectCommitLogNonLinearFirstParent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: base")
	p := gitTaOut(t, dir, "rev-parse", "HEAD")

	gitTa(t, dir, "checkout", "-q", "-b", "rel")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "feat: real")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "feat: real") // zero-delta twin
	gitTa(t, dir, "tag", "v0.1.0")

	gitTa(t, dir, "checkout", "-q", "-b", "hot", p)
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "fix: real")
	gitTa(t, dir, "tag", "v0.2.0")

	// Range log: only the hotfix commit, and it is retained with its
	// tree-delta status intact.
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "v0.2.0", nil).Text(); got != "- fix: real" {
		t.Errorf("CollectCommitLog(non-linear range) = %q, want %q", got, "- fix: real")
	}

	// Whole-history (empty from) log bounded at the rel tag: oldest-first
	// via --reverse, so the zero-delta twin between the two rel commits is
	// excluded and only one "feat: real" survives.
	got := CollectCommitLog(ctx, dir, "", "v0.1.0", nil).Text()
	want := "- chore: base\n- feat: real"
	if got != want {
		t.Errorf("CollectCommitLog(non-linear whole) = %q, want %q", got, want)
	}
}

// TestCollectCommitLogEmptyTwinAfterDelta pins order-independence: the
// retained tree-changing twin lands LATER in the range than the zero-delta
// repeat, so a naive "first in log order wins" implementation would drop
// the real commit and keep the empty one.
func TestCollectCommitLogEmptyTwinAfterDelta(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	writeRepoFile(t, dir, "v1")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: base")
	gitTa(t, dir, "tag", "v1.0.0")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "fix: apply change", "-m", "Changes the thing.")
	writeRepoFile(t, dir, "v2")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "fix: apply change", "-m", "Changes the thing.")

	// Guard: the delta twin (HEAD) must really change the tree, and the
	// empty twin (HEAD~1) must not.
	if a, b := gitTaOut(t, dir, "rev-parse", "HEAD~1^{tree}"), gitTaOut(t, dir, "rev-parse", "HEAD^{tree}"); a == b {
		t.Fatal("fixture broken: delta twin is not tree-changing")
	}

	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", nil).Text()
	want := "- fix: apply change\n\nChanges the thing."
	if got != want {
		t.Errorf("CollectCommitLog(twin after delta) = %q, want %q (exactly one retained)", got, want)
	}
}

// TestCollectCommitLogRetainsDistinctEmpty proves distinct empty commits
// and nonempty same-subject commits are never excluded.
func TestCollectCommitLogRetainsDistinctEmpty(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	writeRepoFile(t, dir, "v1")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: base")
	gitTa(t, dir, "tag", "v1.0.0")
	writeRepoFile(t, dir, "v2")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "fix: apply change")
	writeRepoFile(t, dir, "v3")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "fix: apply change")                  // same subject, new tree
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "fix: apply change") // zero-delta twin of BOTH
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "chore: first empty")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "chore: second empty")
	writeRepoFile(t, dir, "v4")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: version only") // tree differs: metadata-only change

	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", nil).Text()
	// Range logs are newest-first.
	want := "- chore: version only\n- chore: second empty\n- chore: first empty\n" +
		"- fix: apply change\n- fix: apply change"
	if got != want {
		t.Errorf("CollectCommitLog(distinct) = %q, want %q", got, want)
	}
}

// TestCollectCommitLogKeepAll pins the wildcard/empty include-set contract
// through the metadata-aware collector: with keepAll in effect, EVERY typed
// commit is retained (the per-record decision must not fall through to the
// type match when keepAll is true), including zero-delta empty repeats of
// distinct messages.
func TestCollectCommitLogKeepAll(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	writeRepoFile(t, dir, "v1")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: base")
	gitTa(t, dir, "tag", "v1.0.0")
	writeRepoFile(t, dir, "v2")
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "fix: apply change")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "docs: note one")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "docs: note two")
	gitTa(t, dir, "commit", "-q", "--allow-empty", "-m", "build(deps): bump dep")

	// Wildcard: all typed commits retained, newest-first.
	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"*"}).Text()
	want := "- build(deps): bump dep\n- docs: note two\n- docs: note one\n- fix: apply change"
	if got != want {
		t.Errorf("CollectCommitLog(wildcard) = %q, want %q", got, want)
	}

	// Empty include set: same keep-all behavior.
	got = CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", nil).Text()
	if got != want {
		t.Errorf("CollectCommitLog(nil) = %q, want %q", got, want)
	}

	// A narrow filter must drop the non-matching types; the zero-delta
	// distinct empty commits prove keep-all is not the only path to
	// retention.
	got = CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"fix"}).Text()
	if got != "- fix: apply change" {
		t.Errorf("CollectCommitLog(fix only) = %q, want exactly the fix commit", got)
	}
}

// TestFilterCommitLogNoDedup pins the raw-message contract: FilterCommitLog
// has no tree/parent metadata, so identical full messages are never
// deduplicated there — only CollectCommitLog's metadata-aware selection
// excludes zero-tree-delta empty repeats.
func TestFilterCommitLogNoDedup(t *testing.T) {
	raw := "- fix: apply change\n\nChanges the thing.\x00- fix: apply change\n\nChanges the thing.\x00"
	if got := FilterCommitLog(raw, nil); got != "- fix: apply change\n\nChanges the thing.\n- fix: apply change\n\nChanges the thing." {
		t.Errorf("FilterCommitLog must keep both identical records, got:\n%q", got)
	}
	if got := FilterCommitLog(raw, []string{"fix"}); got != "- fix: apply change\n\nChanges the thing.\n- fix: apply change\n\nChanges the thing." {
		t.Errorf("FilterCommitLog (typed) must keep both identical records, got:\n%q", got)
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

func TestFilterCommitLogWildcard(t *testing.T) {
	raw := "- docs: update readme\x00- build(deps): bump dep\x00- fix: patch bug\x00"

	t.Run("wildcard keeps all types", func(t *testing.T) {
		got := FilterCommitLog(raw, ParseCommitTypes("*"))
		for _, subject := range []string{"docs: update readme", "build(deps): bump dep", "fix: patch bug"} {
			if !strings.Contains(got, subject) {
				t.Errorf("wildcard output missing %q; got:\n%s", subject, got)
			}
		}
	})

	t.Run("default types keep fix, drop docs and build", func(t *testing.T) {
		got := FilterCommitLog(raw, ParseCommitTypes("fix,feat,refactor,perf"))
		if !strings.Contains(got, "fix: patch bug") {
			t.Errorf("output missing fix subject; got:\n%s", got)
		}
		if strings.Contains(got, "docs: update readme") {
			t.Errorf("output should not contain docs subject; got:\n%s", got)
		}
		if strings.Contains(got, "build(deps): bump dep") {
			t.Errorf("output should not contain build subject; got:\n%s", got)
		}
	})
}

func TestSelectHunks(t *testing.T) {
	// Each unit costs header + hunk = 25 + 15 = 40 bytes on first emit
	// per file; a second hunk of the same file costs only its 15 bytes.
	a := diffUnit{class: "source", filePath: "a.go", fileHeader: "diff --git a/a.go b/a.go\n", hunkText: strings.Repeat("a", 15), origIndex: 0}
	b := diffUnit{class: "source", filePath: "b.go", fileHeader: "diff --git a/b.go b/b.go\n", hunkText: strings.Repeat("b", 15), origIndex: 1}
	d := diffUnit{class: "docs", filePath: "x.md", fileHeader: "diff --git a/x.md b/x.md\n", hunkText: strings.Repeat("x", 15), origIndex: 2}

	// Case A: budget 90 fits both source hunks (80) but not the docs hunk.
	out, skipped, _ := selectHunks([]diffUnit{a, b, d}, 90)
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
	out2, skipped2, _ := selectHunks([]diffUnit{dy, src, dx}, 80)
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
	out4, skipped4, _ := selectHunks([]diffUnit{docA, srcZ}, 40)
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
	out3, skipped3, _ := selectHunks([]diffUnit{h, h2}, 80)
	// A hunk that cannot fit whole is clipped at a line boundary instead of
	// disappearing: the header line and the first whole lines survive, and the
	// clipped count reports it.
	big := diffUnit{class: "source", filePath: "big.go", fileHeader: "H\n", hunkText: "@@ -1,2 +1,2 @@\n keep one\n keep two\n drop this line\n", origIndex: 0}
	out5, skipped5, clipped5 := selectHunks([]diffUnit{big}, len("H\n")+len("@@ -1,2 +1,2 @@\n")+len(" keep one\n"))
	if skipped5 != 0 || clipped5 != 1 {
		t.Errorf("clip: skipped = %d, clipped = %d, want 0/1", skipped5, clipped5)
	}
	if !strings.Contains(out5, "@@ -1,2 +1,2 @@") || !strings.Contains(out5, "keep one") || strings.Contains(out5, "drop this line") {
		t.Errorf("clip output = %q, want header and first line only", out5)
	}
	if !strings.Contains(out5, "[hunk truncated]") {
		t.Errorf("clip output must mark the elision: %q", out5)
	}

	if skipped3 != 0 || strings.Count(out3, "diff --git") != 0 ||
		!strings.Contains(out3, strings.Repeat("m", 35)) || !strings.Contains(out3, strings.Repeat("n", 35)) {
		t.Errorf("same-file hunks: skipped = %d, out = %q (want both hunks, one header)", skipped3, out3)
	}
}

// rangeCommits returns the selected commits for a range, matching what the
// pipeline passes to CollectDiff.
func rangeCommits(t *testing.T, dir, from, to string) []Commit {
	t.Helper()
	return CollectCommitLog(context.Background(), dir, from, to, nil).Commits
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
	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"feat"}).Text()
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
	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"fix"}).Text()
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

	dc, err := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", rangeCommits(t, dir, "v0.1.0", "v0.2.0"), DiffBudgetBytes)
	if err != nil {
		t.Fatalf("CollectDiff: %v", err)
	}
	got := dc.Text
	if got == "" {
		t.Fatal("expected non-empty diff")
	}
	if dc.Truncated || dc.SkippedHunks != 0 {
		t.Errorf("full budget: Truncated = %v, SkippedHunks = %d, want none", dc.Truncated, dc.SkippedHunks)
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

	dc, err := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", rangeCommits(t, dir, "v0.1.0", "v0.2.0"), DiffBudgetBytes)
	if err != nil {
		t.Fatalf("CollectDiff: %v", err)
	}
	diff := dc.Text
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

	fullDC, err := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", rangeCommits(t, dir, "v0.1.0", "v0.2.0"), DiffBudgetBytes)
	if err != nil {
		t.Fatalf("CollectDiff: %v", err)
	}
	full := fullDC.Text
	if !strings.Contains(full, "a.go") || !strings.Contains(full, "b.md") {
		t.Fatalf("full diff missing files; got:\n%s", full)
	}
	if fullDC.Truncated || fullDC.SkippedHunks != 0 {
		t.Errorf("unbudgeted diff must not truncate: Truncated = %v, SkippedHunks = %d", fullDC.Truncated, fullDC.SkippedHunks)
	}

	// A 1-byte budget cannot fit a single hunk, so every hunk is skipped;
	// the structured status must report it (the marker text stays in the
	// output for display, but is not the status).
	tinyDC, err := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", rangeCommits(t, dir, "v0.1.0", "v0.2.0"), 1)
	if err != nil {
		t.Fatalf("CollectDiff: %v", err)
	}
	tiny := tinyDC.Text
	if !tinyDC.Truncated {
		t.Errorf("tight budget: Truncated = false, want true")
	}
	if tinyDC.SkippedHunks != 2 {
		t.Errorf("tight budget: SkippedHunks = %d, want 2 (one per file)", tinyDC.SkippedHunks)
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
	dc, err := CollectDiff(ctx, dir, "", "v0.2.0", rangeCommits(t, dir, "", "v0.2.0"), DiffBudgetBytes)
	if err != nil {
		t.Fatalf("CollectDiff: %v", err)
	}
	got := dc.Text
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
	// No git repo here: git diff must fail and the failure must surface as
	// a non-nil error, never as a complete (empty) context.
	dc, err := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", nil, DiffBudgetBytes)
	if err == nil {
		t.Fatalf("expected error on git failure, got %+v", dc)
	}
	if dc.Truncated || dc.SkippedHunks != 0 || dc.Text != "" {
		t.Errorf("failed diff must carry no content/status, got %+v", dc)
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

	// Option-injection attempts must be rejected before any git exec:
	// a non-nil error with no context content.
	t.Run("badFromTag", func(t *testing.T) {
		fakeDir, marker := fakeGitDir(t)
		t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		dc, err := CollectDiff(ctx, dir, "--output=/tmp/evil", "v0.2.0", nil, DiffBudgetBytes)
		if err == nil {
			t.Errorf("expected error for bad fromTag, got %+v", dc)
		}
		if _, merr := os.Stat(marker); merr == nil {
			t.Error("git was executed for bad fromTag; guard must reject before exec")
		}
	})
	t.Run("badToTag", func(t *testing.T) {
		fakeDir, marker := fakeGitDir(t)
		t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		dc, err := CollectDiff(ctx, dir, "v0.1.0", "-q", nil, DiffBudgetBytes)
		if err == nil {
			t.Errorf("expected error for bad toTag, got %+v", dc)
		}
		if _, merr := os.Stat(marker); merr == nil {
			t.Error("git was executed for bad toTag; guard must reject before exec")
		}
	})
	// Control: valid tags still work (the 1-byte budget forces a truncation,
	// reported by the structured fields, and the output must be non-empty).
	// Runs under the real PATH (t.Setenv restored it when the subtests
	// above ended).
	dc, err := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", rangeCommits(t, dir, "v0.1.0", "v0.2.0"), 1)
	if err != nil {
		t.Fatalf("valid tags: %v", err)
	}
	if dc.Text == "" {
		t.Error("expected non-empty diff for valid tags")
	}
	// Control: implicit HEAD — an empty toTag makes git resolve the missing
	// range endpoint to HEAD, so the diff covers the commit(s) after v0.1.0
	// (here the "two" commit, which rewrites f.txt).
	implicitDC, err := CollectDiff(ctx, dir, "v0.1.0", "", rangeCommits(t, dir, "v0.1.0", ""), DiffBudgetBytes)
	if err != nil {
		t.Fatalf("implicit HEAD: %v", err)
	}
	if implicitDC.Text == "" || !strings.Contains(implicitDC.Text, "f.txt") {
		t.Errorf("expected non-empty implicit-HEAD diff mentioning f.txt; got:\n%s", implicitDC.Text)
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
		if got := CollectCommitLog(ctx, dir, "--output=/tmp/evil", "v0.2.0", nil).Text(); got != "" {
			t.Errorf("expected empty log for bad fromTag, got:\n%s", got)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("git was executed for bad fromTag; guard must reject before exec")
		}
	})
	t.Run("badToTag", func(t *testing.T) {
		fakeDir, marker := fakeGitDir(t)
		t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		if got := CollectCommitLog(ctx, dir, "v0.1.0", "--output=/tmp/evil", nil).Text(); got != "" {
			t.Errorf("expected empty log for bad toTag, got:\n%s", got)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("git was executed for bad toTag; guard must reject before exec")
		}
	})
	// Control: valid tags still work. Runs under the real PATH (t.Setenv
	// restored it when the subtests above ended).
	if got := CollectCommitLog(ctx, dir, "v0.1.0", "v0.2.0", nil).Text(); got == "" {
		t.Error("expected non-empty log for valid tags")
	}
}

func TestCollectCommitLogRetainsSelectedSHAAndMergeEvidence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q", "--initial-branch=main")
	gitTa(t, dir, "commit", "--allow-empty", "-m", "chore: baseline")
	gitTa(t, dir, "tag", "v1.0.0")
	gitTa(t, dir, "checkout", "-b", "fix-archive")
	if err := os.WriteFile(filepath.Join(dir, "archive.txt"), []byte("verified"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", ".")
	const subject = "fix: export verified archives"
	const body = "Run annalist export --archive only for verified accounts. Archived records remain readable."
	gitTa(t, dir, "commit", "-m", subject, "-m", body)
	sourceSHA := TargetCommitSHA(ctx, dir, "HEAD")
	gitTa(t, dir, "commit", "--allow-empty", "-m", subject, "-m", body)
	repeatSHA := TargetCommitSHA(ctx, dir, "HEAD")
	gitTa(t, dir, "commit", "--allow-empty", "-m", "docs: update a guide")
	gitTa(t, dir, "checkout", "main")
	gitTa(t, dir, "merge", "--no-ff", "fix-archive", "-m", "Merge archive changes")
	mergeSHA := TargetCommitSHA(ctx, dir, "HEAD")
	selected := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"fix"})
	if len(selected.Commits) != 1 || selected.Commits[0].SHA != sourceSHA || selected.Commits[0].SHA == repeatSHA || selected.Commits[0].Subject != subject || selected.Commits[0].Body != body {
		t.Fatalf("selection lost exact source identity or message: %+v", selected)
	}
	if len(selected.MergeSHAs) != 1 || selected.MergeSHAs[0] != mergeSHA || strings.Contains(selected.Text(), "Merge archive") || strings.Contains(selected.Text(), "docs:") {
		t.Fatalf("merge evidence changed the selected source: %+v", selected)
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
	got := CollectCommitLog(ctx, dir, "v1.0.0", "HEAD", []string{"feat", "fix"}).Text()
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

func TestFairShare(t *testing.T) {
	for _, tc := range []struct {
		name   string
		costs  []int
		budget int
		want   []int
	}{
		{"even split", []int{100, 100}, 100, []int{50, 50}},
		{"small commit keeps its diff", []int{10, 1000}, 100, []int{10, 90}},
		{"everything fits", []int{5, 5}, 100, []int{5, 5}},
		{"single oversized commit", []int{300}, 100, []int{100}},
		{"empty commits cost nothing", []int{0, 0}, 5, []int{0, 0}},
		{"zero budget", []int{10, 10}, 0, []int{0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := fairShare(tc.costs, tc.budget)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("fairShare(%v, %d) = %v, want %v", tc.costs, tc.budget, got, tc.want)
			}
			if total := sumInts(got); total > tc.budget {
				t.Fatalf("allocation %d exceeds budget %d", total, tc.budget)
			}
		})
	}
}

func sumInts(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

// TestCollectDiffSharesBudgetAcrossCommits pins the property that one large
// change cannot consume the evidence budget for the rest of the release.
func TestCollectDiffSharesBudgetAcrossCommits(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: seed")
	gitTa(t, dir, "tag", "v0.1.0")

	var large strings.Builder
	for i := range 400 {
		fmt.Fprintf(&large, "line %d of the large change\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "large.go"), []byte(large.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "feat: large change")

	if err := os.WriteFile(filepath.Join(dir, "small.go"), []byte("small change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "fix: small change")
	gitTa(t, dir, "tag", "v0.2.0")

	commits := rangeCommits(t, dir, "v0.1.0", "v0.2.0")
	if len(commits) != 2 {
		t.Fatalf("commits = %d, want 2", len(commits))
	}
	// Enough for the small commit plus part of the large one, far less than
	// the large commit needs alone.
	dc, err := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", commits, 2048)
	if err != nil {
		t.Fatalf("CollectDiff: %v", err)
	}
	for _, want := range []string{"feat: large change", "fix: small change", "small change", "large.go"} {
		if !strings.Contains(dc.Text, want) {
			t.Fatalf("budget-starved diff missing %q; got:\n%s", want, dc.Text)
		}
	}
	if !dc.Truncated {
		t.Fatal("expected the large commit's share to be truncated")
	}
}

// TestCollectDiffSkipsNoiseHunks keeps lockfiles and test data in the --stat
// summary without spending hunk budget on them.
func TestCollectDiffSkipsNoiseHunks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gitTa(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "chore: seed")
	gitTa(t, dir, "tag", "v0.1.0")

	if err := os.MkdirAll(filepath.Join(dir, "testdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testdata", "fixture.json"), []byte("{\"seed\": true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bun.lock"), []byte("lockfile-version-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTa(t, dir, "add", "-A")
	gitTa(t, dir, "commit", "-q", "-m", "feat: add entry point")
	gitTa(t, dir, "tag", "v0.2.0")

	commits := rangeCommits(t, dir, "v0.1.0", "v0.2.0")
	dc, err := CollectDiff(ctx, dir, "v0.1.0", "v0.2.0", commits, DiffBudgetBytes)
	if err != nil {
		t.Fatalf("CollectDiff: %v", err)
	}
	if !strings.Contains(dc.Text, "main.go") {
		t.Fatalf("source hunk missing; got:\n%s", dc.Text)
	}
	if !strings.Contains(dc.Text, "bun.lock") || !strings.Contains(dc.Text, "testdata/fixture.json") {
		t.Fatalf("stat summary must still list skipped noise files; got:\n%s", dc.Text)
	}
	for _, noise := range []string{"diff --git a/bun.lock", "diff --git a/testdata/fixture.json"} {
		if strings.Contains(dc.Text, noise) {
			t.Fatalf("noise file hunks must not consume budget; got:\n%s", dc.Text)
		}
	}
	if dc.Truncated {
		t.Fatalf("noise filtering must not report truncation: %+v", dc)
	}
}
