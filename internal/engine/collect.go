package engine

import (
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

// version is a parsed numeric semver triple.
type version struct {
	major, minor, patch int
}

// compare returns -1, 0, or 1 comparing v against o numerically.
func (v version) compare(o version) int {
	if v.major != o.major {
		return cmpInt(v.major, o.major)
	}
	if v.minor != o.minor {
		return cmpInt(v.minor, o.minor)
	}
	return cmpInt(v.patch, o.patch)
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

func parseVersion(tag string) (version, bool) {
	m := versionRe.FindStringSubmatch(tag)
	if m == nil {
		return version{}, false
	}
	major, err1 := strconv.Atoi(m[1])
	minor, err2 := strconv.Atoi(m[2])
	patch, err3 := strconv.Atoi(m[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return version{}, false
	}
	return version{major: major, minor: minor, patch: patch}, true
}

// ResolvePrevTag returns the greatest valid semver-ish tag strictly less than
// current in workdir, using numeric comparison so v1.10.0 > v1.9.0. Returns ""
// when no strictly-less candidate exists (first release). On git failure, or if
// current is not a parseable v?X.Y.Z, it still compares eligible candidates and
// returns the greatest one ("" when none).
func ResolvePrevTag(ctx context.Context, workdir, current string) string {
	cmd := exec.CommandContext(ctx, "git", "tag", "--sort=version:refname")
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	cur, curOK := parseVersion(current)

	var bestTag string
	var best version
	haveBest := false
	for _, line := range strings.Split(string(out), "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" {
			continue
		}
		v, ok := parseVersion(tag)
		if !ok {
			continue
		}
		if curOK && v.compare(cur) >= 0 {
			// Not strictly less than current: not a candidate.
			continue
		}
		if !haveBest || v.compare(best) > 0 {
			best, bestTag, haveBest = v, tag, true
		}
	}
	if !haveBest {
		return ""
	}
	return bestTag
}

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
// If includeTypes is nil or empty, all commits are kept.
func FilterCommitLog(raw string, includeTypes []string) string {
	if raw == "" {
		return ""
	}

	records := strings.Split(raw, "\x00")
	var kept []string

	for _, rec := range records {
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

		// Keep rules
		keptLine := "- " + subject
		if body != "" {
			keptLine += "\n\n" + body
		}
		if breaking {
			// Breaking commits always kept, with body
			kept = append(kept, keptLine)
		} else if !hasType {
			// Untyped commits always kept
			kept = append(kept, keptLine)
		} else if len(includeTypes) == 0 {
			// No filter configured, keep all
			kept = append(kept, keptLine)
		} else {
			// Check if type is in include set (case-insensitive)
			commitType := strings.ToLower(matches[1])
			for _, allowed := range includeTypes {
				if commitType == strings.ToLower(allowed) {
					kept = append(kept, keptLine)
					break
				}
			}
		}
	}

	return strings.Join(kept, "\n")
}

// CollectCommitLog collects the commit log between fromTag and toTag. With
// fromTag == "" it uses `git log --pretty=format:- %s%n%b%x00 --no-merges
// --reverse HEAD`; otherwise `git log --pretty=format:- %s%n%b%x00 --no-merges
// <from>..<to>`. The raw
// output consists of NUL-delimited records: each record holds a commit subject
// prefixed with "- " followed by the commit body. The records are then passed
// through FilterCommitLog, which applies conventional-commit type filtering
// and keeps breaking-change commits with their body. Returns "" on git error.
// Invalid (non-ref-name) tags are rejected, causing the function to return ""
// like a git failure, so tags are never spliced into git argv unsanitized.
// An empty toTag is not expected (production callers always supply one); if
// it occurs, git resolves the missing range endpoint to HEAD, so the
// function returns the log output up to HEAD rather than "".
func CollectCommitLog(ctx context.Context, workdir, fromTag, toTag string, includeTypes []string) string {
	if (toTag != "" && !validGitRef(toTag)) || (fromTag != "" && !validGitRef(fromTag)) {
		return ""
	}

	// --no-merges keeps Merge commits out of the raw log: a merge subject
	// has no "type: description" form, so FilterCommitLog would treat it as
	// untyped and always keep it — letting a merge body that effectively
	// carries non-selected types (chore/docs, etc.) leak into the LLM log
	// even when a type filter is in effect. `--no-merges` omits the merge
	// commit's subject and body entirely; Git still traverses non-merge commits
	// reachable from both parents, so branch work remains available while
	// merge-only text is dropped.
	var args []string
	if fromTag == "" {
		args = []string{"log", `--pretty=format:- %s%n%b%x00`, "--no-merges", "--reverse", "HEAD"}
	} else {
		args = []string{"log", `--pretty=format:- %s%n%b%x00`, "--no-merges", fromTag + ".." + toTag}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return FilterCommitLog(string(out), includeTypes)
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

// CollectDiff assembles the deep-mode diff between fromTag and toTag: a
// `git diff --stat` summary (always included, regardless of budget) followed
// by hunks selected under maxBytes by file-class priority. Returns "" on git
// failure or when there is no diff content at all.
//
// When fromTag is non-empty the range is three-dot (from...to): git diffs
// from merge-base(from,to), which equals from on linear history and stays
// correct on rebased branches — the merge-base anchoring tag-to-tag release
// flow needs. The unchanged commit log (from..to) covers the same endpoints
// in the common linear case. When fromTag is empty (first release) the diff
// is taken against the empty tree, which has no merge base with toTag, so a
// plain two-commit range is used.
// Invalid (non-ref-name) tags are rejected, causing the function to return ""
// like a git failure, so tags are never spliced into git argv unsanitized.
// An empty toTag is not expected (production callers always supply one); if
// it occurs, git resolves the missing range endpoint to HEAD, so the
// function returns the diff output up to HEAD rather than "".
func CollectDiff(ctx context.Context, workdir, fromTag, toTag string, maxBytes int) string {
	if (toTag != "" && !validGitRef(toTag)) || (fromTag != "" && !validGitRef(fromTag)) {
		return ""
	}

	rangeSpec := fromTag + "..." + toTag
	if fromTag == "" {
		cmd := exec.CommandContext(ctx, "git", "hash-object", "-t", "tree", "/dev/null")
		cmd.Dir = workdir
		treeOut, err := cmd.Output()
		if err != nil {
			return ""
		}
		rangeSpec = strings.TrimSpace(string(treeOut)) + ".." + toTag
	}

	statOut, err := runGit(ctx, workdir, "diff", "--stat", rangeSpec)
	if err != nil {
		return ""
	}
	patchOut, err := runGit(ctx, workdir, "diff", "--patch", "-U3", rangeSpec)
	if err != nil {
		return ""
	}
	statText := strings.TrimSpace(string(statOut))

	units := parseDiff(string(patchOut))
	body, skipped := selectHunks(units, maxBytes)

	var b strings.Builder
	b.WriteString(statText)
	b.WriteString("\n")
	b.WriteString(body)
	if skipped > 0 {
		fmt.Fprintf(&b, "\n\n[diff truncated: %d more hunk(s) omitted to stay within the %d-byte budget]", skipped, maxBytes)
	}
	return strings.TrimSpace(b.String())
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
