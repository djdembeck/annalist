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

// cloneDir computes {dataDir}/clones/{platform}/{shortHash} where shortHash is
// the first 16 hex chars of sha256(cloneURL), or that hash plus a random suffix
// if the resulting directory already exists.
func cloneDir(dataDir, platform, cloneURL string) string {
	sum := sha256.Sum256([]byte(cloneURL))
	shortHash := hex.EncodeToString(sum[:])[:16]

	dir := filepath.Join(dataDir, "clones", platform, shortHash)
	if _, err := os.Stat(dir); err == nil {
		// Collision (or stale clone from a prior run): disambiguate with a
		// random 8-char suffix so concurrent clones never share a workdir.
		dir = filepath.Join(dataDir, "clones", platform, shortHash+randomSuffix())
	}
	return dir
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

// FilterCommitLog takes raw NUL-delimited commit records (each prefixed with "- ")
// and filters them by includeTypes. Breaking commits are always kept with their
// body appended. Untyped commits are always kept. Typed commits are kept only if
// their type is in includeTypes. If includeTypes is nil or empty, all commits are
// kept (but breaking commits still get body appended).
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
			// Also check body for BREAKING CHANGE: trailer
			if !breaking && body != "" {
				breaking = breakingChangeRe.MatchString(body)
			}
		}

		// Keep rules
		if breaking {
			// Breaking commits always kept with body
			keptLine := "- " + subject
			if body != "" {
				keptLine += "\n\n" + body
			}
			kept = append(kept, keptLine)
		} else if !hasType {
			// Untyped commits always kept
			kept = append(kept, "- "+subject)
		} else if len(includeTypes) == 0 {
			// No filter configured, keep all
			kept = append(kept, "- "+subject)
		} else {
			// Check if type is in include set
			commitType := matches[1]
			for _, allowed := range includeTypes {
				if commitType == allowed {
					kept = append(kept, "- "+subject)
					break
				}
			}
		}
	}

	return strings.Join(kept, "\n")
}

// CollectCommitLog reproduces prose-releaser's "Collect commit history" step
// exactly. With fromTag == "" it uses `git log --pretty=format:"- %s" --reverse
// HEAD`; otherwise `git log --pretty=format:"- %s" <from>..<to>`. Returns the
// trimmed stdout ("" when empty or on git error).
func CollectCommitLog(ctx context.Context, workdir, fromTag, toTag string, includeTypes []string) string {
	var args []string
	if fromTag == "" {
		args = []string{"log", `--pretty=format:- %s%n%b%x00`, "--reverse", "HEAD"}
	} else {
		args = []string{"log", `--pretty=format:- %s%n%b%x00`, fromTag + ".." + toTag}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return FilterCommitLog(string(out), includeTypes)
}
