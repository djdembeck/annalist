package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// CloneTo clones cloneURL into {dataDir}/clones/{platform}/{shortHash}/ where
// shortHash is the first 16 hex chars of sha256(cloneURL), falling back to a
// random suffix if that directory already exists. The caller-provided header is
// injected via http.extraHeader so the token NEVER appears in the URL. Returns
// a workdir and a cleanup func that removes the clone dir (errors ignored).
func CloneTo(ctx context.Context, dataDir, platform, cloneURL, header string) (workdir string, cleanup func(), err error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", nil, fmt.Errorf("git binary not found: %w", err)
	}

	dir := cloneDir(dataDir, platform, cloneURL)
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", nil, fmt.Errorf("create clone dir %s: %w", parent, err)
	}

	cmd := exec.CommandContext(ctx, "git",
		"-c", "credential.helper=",
		"-c", "http.extraHeader=Authorization: "+header,
		"clone", "--quiet", cloneURL, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("git clone %s: %w: %s", cloneURL, err, strings.TrimSpace(string(out)))
	}

	cleanup = func() { _ = os.RemoveAll(dir) }
	return dir, cleanup, nil
}

// cloneDir computes a unique clone workdir under {dataDir}/clones/{platform}
// named after the first 16 hex chars of sha256(cloneURL) plus a random
// suffix. The suffix is always present — not only on collision — because
// concurrent clones of the same URL (e.g. two profiles of one release
// generating in parallel) must never share a workdir.
func cloneDir(dataDir, platform, cloneURL string) string {
	sum := sha256.Sum256([]byte(cloneURL))
	shortHash := hex.EncodeToString(sum[:])[:16]
	return filepath.Join(dataDir, "clones", platform, shortHash+randomSuffix())
}

func randomSuffix() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "-" + strconv.FormatInt(int64(os.Getpid()), 16)
	}
	return "-" + hex.EncodeToString(b)
}

// Version is a parsed numeric semver triple — the one canonical release-tag
// semver in this codebase; the pipeline's range floor compares with it.
type Version struct {
	Major, Minor, Patch int
}

// Compare returns -1, 0, or 1 comparing v against o numerically.
func (v Version) Compare(o Version) int {
	if v.Major != o.Major {
		return cmpInt(v.Major, o.Major)
	}
	if v.Minor != o.Minor {
		return cmpInt(v.Minor, o.Minor)
	}
	return cmpInt(v.Patch, o.Patch)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// versionRe matches a semver-ish v?X.Y.Z tag (optional leading v, three
// dot-separated integers). A trailing prerelease/build suffix is ignored — the
// first three components just need to be digits.
var versionRe = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)`)

// ParseTagVersion parses a tag (or a tag's version segment) as a Version.
// ok is false when the leading component is not a v?X.Y.Z triple; a trailing
// prerelease/build suffix never changes the result.
func ParseTagVersion(tag string) (Version, bool) {
	m := versionRe.FindStringSubmatch(tag)
	if m == nil {
		return Version{}, false
	}
	major, err1 := strconv.Atoi(m[1])
	minor, err2 := strconv.Atoi(m[2])
	patch, err3 := strconv.Atoi(m[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return Version{}, false
	}
	return Version{Major: major, Minor: minor, Patch: patch}, true
}

// SplitPrefixedTag splits a prefixed release tag (<prefix>/vX.Y.Z, e.g.
// "ggsimpleads/v1.0.2") into its prefix — including the trailing "/" — and its
// version part. ok is false for tags without a prefix or whose trailing segment
// is not a v?X.Y.Z version.
func SplitPrefixedTag(tag string) (prefix, ver string, ok bool) {
	i := strings.LastIndex(tag, "/")
	if i < 0 || i == len(tag)-1 {
		return "", "", false
	}
	p, v := tag[:i+1], tag[i+1:]
	if _, ok := ParseTagVersion(v); !ok {
		return "", "", false
	}
	return p, v, true
}

// ResolvePrevTag returns the greatest valid semver-ish tag strictly less than
// current in workdir, using numeric comparison so v1.10.0 > v1.9.0. Returns ""
// when no strictly-less candidate exists (first release). On git failure, or if
// current is not a parseable v?X.Y.Z, it still compares eligible candidates and
// returns the greatest one ("" when none).
//
// For a prefixed tag (<prefix>/vX.Y.Z, one release stream per artifact) the
// candidates are restricted to tags of the SAME prefix: the previous release of
// that stream, never an unrelated tag from another stream or the repo-wide
// stream. A prefixed tag with no earlier tag of its prefix is the first release
// of its stream and resolves to "" (an empty range), matching the scope rule
// that a stream's first release documents no changes to its artifact.
func ResolvePrevTag(ctx context.Context, workdir, current string) string {
	cmd := exec.CommandContext(ctx, "git", "tag", "--sort=version:refname")
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	var bestTag string
	var best Version
	haveBest := false

	if prefix, ver, ok := SplitPrefixedTag(current); ok {
		// Same-prefix comparison: only this stream's tags are candidates.
		cur, _ := ParseTagVersion(ver)
		for _, line := range strings.Split(string(out), "\n") {
			tag := strings.TrimSpace(line)
			if tag == "" {
				continue
			}
			p, v, ok := SplitPrefixedTag(tag)
			if !ok || p != prefix {
				continue
			}
			pv, _ := ParseTagVersion(v)
			if pv.Compare(cur) >= 0 {
				continue
			}
			if !haveBest || pv.Compare(best) > 0 {
				best, bestTag, haveBest = pv, tag, true
			}
		}
		if !haveBest {
			return ""
		}
		return bestTag
	}

	cur, curOK := ParseTagVersion(current)
	for _, line := range strings.Split(string(out), "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" {
			continue
		}
		v, ok := ParseTagVersion(tag)
		if !ok {
			continue
		}
		if curOK && v.Compare(cur) >= 0 {
			// Not strictly less than current: not a candidate.
			continue
		}
		if !haveBest || v.Compare(best) > 0 {
			best, bestTag, haveBest = v, tag, true
		}
	}
	if !haveBest {
		return ""
	}
	return bestTag
}

// TagCommitSHA returns the commit SHA that tag Points to in workdir,
// resolving annotated tags to their target commit. Returns "" on git
// failure, for a tag absent from the clone, or for an invalid ref name.
func TagCommitSHA(ctx context.Context, workdir, tag string) string {
	if !validGitRef(tag) {
		return ""
	}
	out, err := runGit(ctx, workdir, "rev-list", "-n", "1", tag+"^{commit}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TargetCommitSHA resolves a git ref (usually the target release tag, which
// must exist when notes are generated for it) to the full commit SHA it
// points at, unwrapping annotated tags. Returns "" on git failure or for an
// invalid ref name.
func TargetCommitSHA(ctx context.Context, workdir, ref string) string {
	if !validGitRef(ref) {
		return ""
	}
	out, err := runGit(ctx, workdir, "rev-list", "-n", "1", ref+"^{commit}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// IsAncestorCommit reports whether commit ancestor is reachable from commit
// descendant. Both must be full SHAs already in workdir. Returns false on
// git failure (malformed SHAs, unborn objects) — callers treating an
// unverifiable candidate as non-ancestor are the common, fail-closed use.
func IsAncestorCommit(ctx context.Context, workdir, ancestor, descendant string) bool {
	if !isFullSHA(ancestor) || !isFullSHA(descendant) {
		return false
	}
	_, err := runGit(ctx, workdir, "merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}

// isFullSHA reports whether s is a 40-character lowercase hex SHA-1.
func isFullSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// DefaultCommitTypes is the commit-type filter used when no saved repo or
// global setting and no config value specify one. It must stay in sync
// with DEFAULT_COMMIT_TYPES in web/src/lib/commitTypes.ts, which the
// settings UI displays as the pre-checked default.
const DefaultCommitTypes = "fix,feat,refactor,perf"

// ParseCommitTypes splits a comma-separated string of conventional commit types,
// trimming whitespace and dropping empty entries. Returns nil if the input is
// empty or contains only commas/whitespace.
func ParseCommitTypes(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// commitTypeRe matches conventional-commit subjects:
// type(scope)!: description  or  type!: description  or  type: description
var commitTypeRe = regexp.MustCompile(`^([A-Za-z0-9_-]+)(?:\(([^)]*)\))?(!)?: (.*)$`)

// breakingChangeRe matches "BREAKING CHANGE:" or "BREAKING-CHANGE:" at start of line.
var breakingChangeRe = regexp.MustCompile(`(?im)^BREAKING[ -]CHANGE:`)

// validGitRef reports whether s is safe to use as a git ref name (tag) in a
// command-line range expression. It mirrors the rules that make a string
// safe to pass to git as a single argv element without option-injection:
// it must be non-empty, must not start with '-', and must contain no
// characters git itself rejects in ref names (check-ref-format):
// control chars, space, ~, ^, :, ?, *, [, backslash, and no '..' or '@{'
// sequences, no leading/trailing '/', no '//', no trailing '.'.
func validGitRef(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || strings.ContainsRune("~^:?*[\\ ", r) {
			return false
		}
	}
	if strings.Contains(s, "..") || strings.Contains(s, "@{") ||
		strings.HasPrefix(s, "/") || strings.HasPrefix(s, ".") ||
		strings.HasSuffix(s, "/") || strings.HasSuffix(s, ".") ||
		strings.Contains(s, "//") {
		return false
	}
	return true
}

// FilterCommitLog takes raw NUL-delimited commit records (each prefixed with "- ")
// and filters them by includeTypes. Kept commits carry their body (when present)
// appended after the subject. Breaking commits are always kept. Untyped commits
// are always kept. Typed commits are kept only if their type is in includeTypes.
// If includeTypes is nil or empty, all commits are kept; an include set
// containing the wildcard "*" is an explicit keep-all and behaves the same.
//
// This raw-message filter has no tree/parent metadata, so it never dedups
// commits: identical subjects are always both kept here. CollectCommitLog
// applies the same per-record decision plus the metadata-based empty-repeat
// exclusion.
func FilterCommitLog(raw string, includeTypes []string) string {
	if raw == "" {
		return ""
	}

	keepAll := keepAllFor(includeTypes)
	var kept []string

	for _, rec := range strings.Split(raw, "\x00") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}

		// Strip leading "- " prefix
		subjectAndBody := rec
		if strings.HasPrefix(rec, "- ") {
			subjectAndBody = rec[2:]
		}

		// Split into subject (first line) and body (rest)
		parts := strings.SplitN(subjectAndBody, "\n", 2)
		subject := parts[0]
		body := ""
		if len(parts) > 1 {
			body = strings.TrimSpace(parts[1])
		}

		if commitKeepDecision(subject, body, includeTypes, keepAll) {
			line := "- " + subject
			if body != "" {
				line += "\n\n" + body
			}
			kept = append(kept, line)
		}
	}

	return strings.Join(kept, "\n")
}

// keepAllFor reports whether no type filtering applies: an empty include set,
// or an explicit wildcard.
func keepAllFor(includeTypes []string) bool {
	if len(includeTypes) == 0 {
		return true
	}
	for _, allowed := range includeTypes {
		if allowed == "*" {
			return true
		}
	}
	return false
}

// commitKeepDecision applies the conventional-commit type filter to one
// commit's subject/body. Breaking commits and untyped commits are always
// kept; typed commits are kept when keepAll or when their type is in
// includeTypes (case-insensitive).
func commitKeepDecision(subject, body string, includeTypes []string, keepAll bool) bool {
	// Parse conventional commit type
	matches := commitTypeRe.FindStringSubmatch(subject)
	hasType := matches != nil
	breaking := false

	if hasType {
		// Group 3 is "!" shorthand
		breaking = matches[3] == "!"
	}
	// Also check body for BREAKING CHANGE: trailer, even on untyped
	// subjects — untyped breaking commits must keep their body too.
	if !breaking && body != "" {
		breaking = breakingChangeRe.MatchString(body)
	}

	keep := breaking || !hasType || keepAll
	if !keep {
		// Check if type is in include set (case-insensitive)
		commitType := strings.ToLower(matches[1])
		for _, allowed := range includeTypes {
			if commitType == strings.ToLower(allowed) {
				keep = true
				break
			}
		}
	}

	return keep
}

// Commit is a retained source change. SHA is the exact Git object identity.
type Commit struct {
	SHA     string
	Subject string
	Body    string
}

// CommitLog keeps selected source commits and the range's merge identities.
// MergeSHAs are association evidence only, never release-note source items.
type CommitLog struct {
	Commits   []Commit
	MergeSHAs []string
}

// Text renders the source messages without losing their bodies or qualifiers.
func (commits CommitLog) Text() string {
	var text strings.Builder
	for i, commit := range commits.Commits {
		if i > 0 {
			text.WriteByte('\n')
		}
		text.WriteString("- ")
		text.WriteString(commit.Subject)
		if commit.Body != "" {
			text.WriteString("\n\n")
			text.WriteString(commit.Body)
		}
	}
	return text.String()
}

// SourceToken identifies a source commit in the model's output. Only Annalist
// replaces it with a verified link, after checking the complete attribution.
func SourceToken(sha string) string {
	return "[[annalist:" + sha + "]]"
}

func (commits CommitLog) promptText() string {
	var text strings.Builder
	for i, commit := range commits.Commits {
		if i > 0 {
			text.WriteString("\n\n")
		}
		text.WriteString("Source: [[annalist:")
		text.WriteString(commit.SHA)
		text.WriteString("]]\n- ")
		text.WriteString(commit.Subject)
		if commit.Body != "" {
			text.WriteString("\n\n")
			text.WriteString(commit.Body)
		}
	}
	return text.String()
}

// CollectCommitLog collects selected commit identities and messages between fromTag and toTag. With
// fromTag == "" it uses `git log --reverse <toTag>` — the whole
// history reachable from the release's own tag, not from the clone's default-
// branch HEAD, so a first release never picks up commits that landed after
// it. An empty toTag (not expected; production callers always supply one)
// falls back to HEAD. Otherwise it uses `<from>..<to>`.
//
// Each commit is read with tree/parent metadata (%H %T %P %s %b) so the
// per-record keep decision (conventional-commit type filtering, breaking and
// untyped commits always kept — the same rules as FilterCommitLog) can also
// exclude zero-tree-delta empty repeats: a commit whose tree equals its
// first parent's tree is provably redundant when its full message (subject
// plus body) duplicates an already retained change. Distinct empty commits,
// metadata-only changes (different tree), and nonempty commits sharing a
// subject are all retained. Filtered-out commits never count as retained,
// so deduplication can only suppress a commit whose identical twin was kept.
// Returns an empty log on git error. Invalid (non-ref-name) tags are rejected,
// like a git failure, so tags are never spliced
// into git argv unsanitized. An empty toTag is not expected (production
// callers always supply one); if it occurs, git resolves the missing range
// endpoint to HEAD, so the function returns the log output up to HEAD
// rather than an empty log.
func CollectCommitLog(ctx context.Context, workdir, fromTag, toTag string, includeTypes []string) CommitLog {
	if (toTag != "" && !validGitRef(toTag)) || (fromTag != "" && !validGitRef(fromTag)) {
		return CommitLog{}
	}

	// Read merge metadata in the same log, but never select a merge's subject
	// or body as a source item. Forgejo uses these exact merge identities to
	// find verified PRs, then checks retained branch commits by API membership.

	// The format carries one NUL-delimited field per record: SHA, tree,
	// parent SHAs, subject, body. Git escapes control characters in the
	// text fields, so NUL can only be the field separator.
	const format = `--pretty=format:%H%x00%T%x00%P%x00%s%x00%b%x00`
	var args []string
	if fromTag == "" {
		// A release with no predecessor covers the artifact's whole history,
		// bounded at the release's own tag: the clone's HEAD is the default
		// branch tip, which may already contain later releases' commits, so
		// using toTag keeps a stream's first release from documenting work
		// that landed after it. An empty toTag (unexpected) falls back to HEAD.
		end := toTag
		if end == "" {
			end = "HEAD"
		}
		args = []string{"log", format, "--reverse", end}
	} else {
		args = []string{"log", format, fromTag + ".." + toTag}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return CommitLog{}
	}
	return selectLogRecords(ctx, workdir, string(out), includeTypes)
}

// logRecord is one commit from the metadata log format.
type logRecord struct {
	Commit
	tree    string
	parents []string
}

// fullMessage is the normalized full commit message: subject, with the
// trimmed body appended when present. Two records are message-identical
// iff their fullMessage values are equal — subject alone never dedups.
func (r logRecord) fullMessage() string {
	if r.Body == "" {
		return r.Subject
	}
	return r.Subject + "\n\n" + r.Body
}

// zeroDelta reports whether the commit changed no file content: its tree
// equals its first parent's tree. A root (parentless) commit is never
// zero-delta — its tree IS the change. A parent whose tree is unknown
// (unresolvable) is treated as non-equal, so an unverifiable commit is
// retained, never dropped.
func (r logRecord) zeroDelta(treeOf map[string]string) bool {
	if len(r.parents) == 0 {
		return false
	}
	return treeOf[r.parents[0]] == r.tree
}

// parseLogRecords splits a metadata log (records of
// sha \x00 tree \x00 parents \x00 subject \x00 body \x00) into records. A
// trailing partial group from a malformed final record is dropped.
func parseLogRecords(raw string) []logRecord {
	fields := strings.Split(raw, "\x00")
	var recs []logRecord
	for i := 0; i+4 < len(fields); i += 5 {
		recs = append(recs, logRecord{
			Commit: Commit{
				SHA:     strings.TrimSpace(fields[i]),
				Subject: fields[i+3],
				Body:    strings.TrimSpace(fields[i+4]),
			},
			tree:    fields[i+1],
			parents: strings.Fields(fields[i+2]),
		})
	}
	return recs
}

// firstParentTrees maps commit SHA to tree SHA for every first parent that
// is not itself a recorded commit (typically the range's from-bound and, on
// non-linear history, parents below it). The in-log commits' own trees seed
// the map; the remaining SHAs are resolved with ONE batched `git
// cat-file --batch` call — never one process per commit. A failed batch
// leaves those parents unresolved (their children then fail closed to
// retained by zeroDelta).
func firstParentTrees(ctx context.Context, workdir string, recs []logRecord) map[string]string {
	trees := make(map[string]string, len(recs))
	for _, r := range recs {
		trees[r.SHA] = r.tree
	}
	var missing []string
	seen := make(map[string]bool, len(recs))
	for _, r := range recs {
		if len(r.parents) == 0 {
			continue
		}
		p := r.parents[0]
		if _, ok := trees[p]; !ok && !seen[p] {
			seen[p] = true
			missing = append(missing, p)
		}
	}
	if extra := catFileCommitTrees(ctx, workdir, missing); extra != nil {
		for sha, tree := range extra {
			trees[sha] = tree
		}
	}
	return trees
}

// catFileCommitTrees resolves commit SHAs to their tree SHAs in a single
// `git cat-file --batch` invocation. Returns nil on git failure or I/O
// error; unknown objects in the batch are simply absent from the result.
// The batch input is fed via cmd.Stdin (a reader), not a StdinPipe: a
// pre-started pipe fill would block on the pipe capacity for large batches
// while the process has not run yet.
func catFileCommitTrees(ctx context.Context, workdir string, shas []string) map[string]string {
	if len(shas) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch")
	cmd.Dir = workdir
	var input strings.Builder
	for _, s := range shas {
		input.WriteString(s)
		input.WriteByte('\n')
	}
	cmd.Stdin = strings.NewReader(input.String())
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	trees := make(map[string]string, len(shas))
	data := out
	for len(data) > 0 {
		nl := bytes.IndexByte(data, '\n')
		if nl < 0 {
			break
		}
		// Header line: "<sha> <type> <size>".
		header := data[:nl]
		data = data[nl+1:]
		fields := strings.Fields(string(header))
		if len(fields) != 3 || fields[1] != "commit" {
			continue
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || len(data) < size {
			continue
		}
		obj := data[:size]
		data = data[size:]
		// The raw object is followed by a single separator newline.
		if len(data) > 0 && data[0] == '\n' {
			data = data[1:]
		}
		tree, ok := strings.CutPrefix(string(obj), "tree ")
		if !ok {
			continue
		}
		if end := strings.IndexByte(tree, '\n'); end >= 0 {
			tree = tree[:end]
		}
		trees[fields[0]] = tree
	}
	return trees
}

// selectLogRecords applies the per-record keep decision plus the
// zero-tree-delta empty-repeat exclusion to a metadata log. The exclusion
// is order-independent with respect to tree-delta commits: a zero-delta
// commit is provably the redundant one whenever any retained commit shares
// its full message, regardless of log order. Among zero-delta commits, the
// first in log order is retained and later identical ones are dropped.
func selectLogRecords(ctx context.Context, workdir, raw string, includeTypes []string) CommitLog {
	recs := parseLogRecords(raw)
	if len(recs) == 0 {
		return CommitLog{}
	}
	keepAll := keepAllFor(includeTypes)
	trees := firstParentTrees(ctx, workdir, recs)

	// Pass 1: per-record type-filter decision. Filtered-out commits never
	// become "retained", so a zero-delta commit whose message matches a
	// filtered commit is not suppressed.
	type decision struct {
		keep    bool
		commit  Commit
		delta   bool
		message string
	}
	decs := make([]decision, len(recs))
	var kept CommitLog
	for i, r := range recs {
		if len(r.parents) > 1 {
			kept.MergeSHAs = append(kept.MergeSHAs, r.SHA)
			continue
		}
		keep := commitKeepDecision(r.Subject, r.Body, includeTypes, keepAll)
		if !keep {
			continue
		}
		decs[i] = decision{
			keep:    keep,
			commit:  r.Commit,
			delta:   r.zeroDelta(trees),
			message: r.fullMessage(),
		}
	}

	// Pass 2: zero-tree-delta empty-repeat exclusion. A zero-delta commit
	// is provably redundant iff its full message is owned by a retained
	// NON-zero-delta commit (log-order independent — the delta twin
	// provably contains the change) or by an earlier retained zero-delta
	// twin in log order. Delta commits are never dropped.
	deltaOwned := make(map[string]bool, len(decs))
	for _, d := range decs {
		if d.keep && !d.delta {
			deltaOwned[d.message] = true
		}
	}
	zeroDeltaClaimed := make(map[string]bool, len(decs))
	for _, d := range decs {
		if !d.keep {
			continue
		}
		if d.delta && (deltaOwned[d.message] || zeroDeltaClaimed[d.message]) {
			continue
		}
		kept.Commits = append(kept.Commits, d.commit)
		if d.delta {
			zeroDeltaClaimed[d.message] = true
		}
	}
	return kept
}

// DiffBudgetBytes caps the hunk content (excluding the always-included --stat
// summary) sent to the LLM in deep mode. Follows the ai-publish 256 KB default.
const DiffBudgetBytes = 256 * 1024

// ClassifyFile maps a repository path to one of the deep-mode diff classes:
// "source", "test", "config", or "docs". Deterministic; order matters (docs,
// then test, then config, source as the default).
func ClassifyFile(path string) string {
	p := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))

	if p == "readme.md" || strings.HasSuffix(p, ".md") ||
		strings.HasPrefix(p, "docs/") || strings.HasPrefix(p, "doc/") {
		return "docs"
	}

	if strings.Contains(p, "/test/") || strings.Contains(p, "/tests/") ||
		strings.Contains(p, "/__tests__/") || strings.Contains(p, "tests/") ||
		strings.HasPrefix(p, "test_") ||
		strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, "_test.py") ||
		strings.HasSuffix(p, ".test.ts") || strings.HasSuffix(p, ".test.js") ||
		strings.HasSuffix(p, ".spec.ts") || strings.HasSuffix(p, ".spec.js") {
		return "test"
	}

	switch {
	case strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml") ||
		strings.HasSuffix(p, ".toml") || strings.HasSuffix(p, ".ini") || strings.HasSuffix(p, ".lock") ||
		strings.HasSuffix(p, ".mod") || strings.HasSuffix(p, ".sum") || strings.HasSuffix(p, ".cfg") ||
		strings.HasSuffix(p, ".conf") || strings.HasSuffix(p, ".env") || strings.HasSuffix(p, ".xml") ||
		strings.HasSuffix(p, ".tf") || strings.HasSuffix(p, ".bicep") || strings.HasSuffix(p, ".properties"):
		return "config"
	case strings.HasPrefix(p, ".github/") || strings.HasPrefix(p, ".forgejo/") ||
		strings.HasPrefix(p, "infra/") || strings.HasPrefix(p, "terraform/") || strings.HasPrefix(p, "k8s/"):
		return "config"
	}
	return "source"
}

// classPriority orders deep-mode diff classes for byte-budget selection:
// source first, then config, test, docs.
var classPriority = map[string]int{
	"source": 0,
	"config": 1,
	"test":   2,
	"docs":   3,
}

// diffUnit is one budgeted hunk of a file section of the patch.
type diffUnit struct {
	class      string
	filePath   string
	fileHeader string
	hunkText   string
	origIndex  int
}

// DiffContext is the structured result of CollectDiff. Text is the
// assembled diff (always-included --stat summary plus the hunks selected
// under the byte budget). Truncated is true only when the selection
// actually omitted hunks to honor the budget, and SkippedHunks counts
// them; consumers must use these fields, not search Text for the marker.
// Text carries the same human-readable truncation marker as before for
// display purposes, but the marker is not a status.
type DiffContext struct {
	Text         string
	Truncated    bool
	SkippedHunks int
}

// CollectDiff assembles the deep-mode diff between fromTag and toTag: a
// `git diff --stat` summary (always included, regardless of budget) followed
// by hunks selected under maxBytes by file-class priority. On success it
// returns a DiffContext whose Truncated/SkippedHunks reflect the actual
// budget selection. A git failure (including an invalid, non-ref-name tag,
// which is rejected before any git argv is built) is returned as a
// non-nil error — never as a zero DiffContext — so an empty Text with a
// nil error means "no diff content", and a git failure is distinguishable
// from a genuinely empty range. When there is no diff content at all the
// context is empty (no Text, not truncated).
//
// When fromTag is non-empty the range is three-dot (from...to): git diffs
// from merge-base(from,to), which equals from on linear history and stays
// correct on rebased branches — the merge-base anchoring tag-to-tag release
// flow needs. The unchanged commit log (from..to) covers the same endpoints
// in the common linear case. When fromTag is empty (first release) the diff
// is taken against the empty tree, which has no merge base with toTag, so a
// plain two-commit range is used.
// An empty toTag is not expected (production callers always supply one); if
// it occurs, git resolves the missing range endpoint to HEAD, so the
// function returns the diff output up to HEAD rather than "".
func CollectDiff(ctx context.Context, workdir, fromTag, toTag string, maxBytes int) (DiffContext, error) {
	if (toTag != "" && !validGitRef(toTag)) || (fromTag != "" && !validGitRef(fromTag)) {
		return DiffContext{}, fmt.Errorf("invalid git ref in diff range: from=%q to=%q", fromTag, toTag)
	}

	rangeSpec := fromTag + "..." + toTag
	if fromTag == "" {
		cmd := exec.CommandContext(ctx, "git", "hash-object", "-t", "tree", "/dev/null")
		cmd.Dir = workdir
		treeOut, err := cmd.Output()
		if err != nil {
			return DiffContext{}, fmt.Errorf("empty tree hash: %w", err)
		}
		rangeSpec = strings.TrimSpace(string(treeOut)) + ".." + toTag
	}

	statOut, err := runGit(ctx, workdir, "diff", "--stat", rangeSpec)
	if err != nil {
		return DiffContext{}, fmt.Errorf("git diff --stat %s: %w", rangeSpec, err)
	}
	patchOut, err := runGit(ctx, workdir, "diff", "--patch", "-U3", rangeSpec)
	if err != nil {
		return DiffContext{}, fmt.Errorf("git diff --patch %s: %w", rangeSpec, err)
	}
	statText := strings.TrimSpace(string(statOut))

	units := parseDiff(string(patchOut))
	body, skipped := selectHunks(units, maxBytes)

	dc := DiffContext{Truncated: skipped > 0, SkippedHunks: skipped}
	if statText == "" && body == "" {
		return dc, nil
	}

	var b strings.Builder
	b.WriteString(statText)
	b.WriteString("\n")
	b.WriteString(body)
	if skipped > 0 {
		fmt.Fprintf(&b, "\n\n[diff truncated: %d more hunk(s) omitted to stay within the %d-byte budget]", skipped, maxBytes)
	}
	dc.Text = strings.TrimSpace(b.String())
	return dc, nil
}

// runGit runs a git subcommand in workdir and returns raw stdout.
func runGit(ctx context.Context, workdir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workdir
	return cmd.Output()
}

// parseDiff splits a git patch into per-hunk units. Sections with no @@ block
// (binary files, empty diffs) contribute no units — they are represented
// only in the --stat summary.
func parseDiff(patch string) []diffUnit {
	var units []diffUnit
	orig := 0
	// Section starts are "diff --git " only at line beginnings, so the
	// literal string inside added-line content of a patch file cannot
	// spawn a fake section.
	starts := make([]int, 0, 8)
	if strings.HasPrefix(patch, "diff --git ") {
		starts = append(starts, 0)
	}
	from := 0
	for {
		idx := strings.Index(patch[from:], "\ndiff --git ")
		if idx < 0 {
			break
		}
		from += idx + 1
		starts = append(starts, from)
	}
	for i, s := range starts {
		e := len(patch)
		if i+1 < len(starts) {
			e = starts[i+1]
		}
		sec := patch[s:e]
		filePath := diffFilePath(sec)
		idx := strings.Index(sec, "\n@@")
		if idx < 0 {
			continue
		}
		header := sec[:idx+1] // through the newline before the first @@
		rest := sec[idx+1:]
		for strings.HasPrefix(rest, "@@") {
			k := strings.Index(rest, "\n@@")
			var hunk string
			if k < 0 {
				hunk = rest
			} else {
				hunk = rest[:k+1]
				rest = rest[k+1:]
			}
			units = append(units, diffUnit{
				class:      ClassifyFile(filePath),
				filePath:   filePath,
				fileHeader: header,
				hunkText:   hunk,
				origIndex:  orig,
			})
			orig++
			if k < 0 {
				break
			}
		}
	}
	return units
}

// diffFilePath extracts the file path from a section's `diff --git a/<old>
// b/<new>` line, preferring the b/ (new) path and falling back to the a/
// path for deletions (b/dev/null).
func diffFilePath(section string) string {
	line := section
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	rest, ok := strings.CutPrefix(line, "diff --git ")
	if !ok {
		return ""
	}
	// The b/ marker is the last " b/" so paths containing " b/" still parse.
	i := strings.LastIndex(rest, " b/")
	if i < 0 {
		return strings.TrimPrefix(rest, "a/")
	}
	old := strings.TrimPrefix(rest[:i], "a/")
	neu := rest[i+3:]
	if neu == "/dev/null" || neu == "dev/null" {
		return old
	}
	return neu
}

// selectHunks deterministically orders units by (class priority, file path,
// original position) and greedily emits hunks under maxBytes. A hunk that
// would exceed the budget is skipped whole (never truncated mid-hunk); later
// cheaper hunks may still fit. Returns the emitted text and skip count.
func selectHunks(units []diffUnit, maxBytes int) (string, int) {
	sorted := make([]diffUnit, len(units))
	copy(sorted, units)
	sort.SliceStable(sorted, func(i, j int) bool {
		pi, pj := classPriority[sorted[i].class], classPriority[sorted[j].class]
		if pi != pj {
			return pi < pj
		}
		if sorted[i].filePath != sorted[j].filePath {
			return sorted[i].filePath < sorted[j].filePath
		}
		return sorted[i].origIndex < sorted[j].origIndex
	})

	emitted := make(map[string]bool, len(sorted))
	var out strings.Builder
	used, skipped := 0, 0
	for _, u := range sorted {
		cost := len(u.hunkText)
		if !emitted[u.filePath] {
			cost += len(u.fileHeader)
		}
		if used+cost > maxBytes {
			skipped++
			continue
		}
		if !emitted[u.filePath] {
			out.WriteString(u.fileHeader)
			emitted[u.filePath] = true
		}
		out.WriteString(u.hunkText)
		used += cost
	}
	return out.String(), skipped
}
