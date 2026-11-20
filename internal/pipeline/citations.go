package pipeline

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/djdembeck/annalist/internal/engine"
)

// PullRequestReference comes from the authenticated commit-association API.
type PullRequestReference struct {
	Number int
	URL    string
}

// CommitReference contains all merged PR associations in the requested repo.
// CommitURL is used only when the provider proved that no association exists.
type CommitReference struct {
	PullRequests []PullRequestReference
	CommitURL    string
}

// SourceReferenceError keeps provider responses out of API errors and history.
// Unwrap preserves cancellation and deadline classification.
type SourceReferenceError struct {
	Platform string
	Err      error
}

func (e *SourceReferenceError) Error() string {
	if e.Platform == "github" {
		return "pipeline: cannot resolve source links through the GitHub API. Check the App's Pull requests: Read-only permission and installation approval. Verify API connectivity and repository associations"
	}
	return "pipeline: cannot resolve source links through Forgejo. Check pull-request read access, API support, and repository associations. A generic 404 does not prove a missing association"
}

func (e *SourceReferenceError) Unwrap() error { return e.Err }

// ValidateSourceURL checks a provider's HTML URL against the configured web
// root, repository, reference type, and exact object identity. Clone URLs are
// never used to infer a web root.
func ValidateSourceURL(root, owner, repo, kind, identity, raw string) error {
	base, err := url.Parse(root)
	if err != nil || base == nil || base.Host == "" || base.User != nil ||
		(base.Scheme != "http" && base.Scheme != "https") || base.RawQuery != "" || base.Fragment != "" {
		return errors.New("invalid platform web root")
	}
	for _, segment := range []string{owner, repo, kind, identity} {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, "/\\") {
			return errors.New("invalid source identity")
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" ||
		u.Scheme != base.Scheme || !strings.EqualFold(u.Host, base.Host) ||
		strings.ContainsAny(raw, "()[]<>\\\"'`") || strings.IndexFunc(raw, unicode.IsSpace) >= 0 ||
		strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return errors.New("provider returned an invalid source URL")
	}
	want := strings.TrimSuffix(base.Path, "/") + "/" + owner + "/" + repo + "/" + kind + "/" + identity
	if !strings.EqualFold(u.Path, want) || u.RawPath != "" || u.ForceQuery {
		return errors.New("provider returned a source URL for another repository or object")
	}
	return nil
}

var (
	sourceTokenRE           = regexp.MustCompile(`\[\[annalist:([0-9a-fA-F]{8,40})\]\]`)
	reservedSourceRE        = regexp.MustCompile(`(?i)(?:\[\[annalist|annalist:)`)
	primaryItemRE           = regexp.MustCompile(`^( {0,3})(?:[-+*]|[0-9]{1,9}[.)])[ \t]+`)
	headingRE               = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]|$)`)
	htmlBlockRE             = regexp.MustCompile(`(?i)^ {0,3}(?:<\?|<![a-z]|</?[a-z][a-z0-9-]*(?:[ \t/>]))`)
	otherLinkRE             = regexp.MustCompile(`(?im)<\s*/?\s*(?:a|area|img|link)\b|^ {0,3}\[[^\]\n]+\]:|\]\s*\[|\]\s*\n\s*\(`)
	webURLRE                = regexp.MustCompile("https?://[^\\s<>\"`]+")
	cachedCitationRE        = regexp.MustCompile(`\(\[(?:#[1-9][0-9]*|[0-9a-f]{7})\]\(https?://[^()\s]+\)(?:, \[(?:#[1-9][0-9]*|[0-9a-f]{7})\]\(https?://[^()\s]+\))*\)`)
	citationLinkRE          = regexp.MustCompile(`\[(#[1-9][0-9]*|[0-9a-f]{7})\]\((https?://[^()\s]+)\)`)
	providerReferencePathRE = regexp.MustCompile(`(?i)/(?:pulls?/[0-9]+|commits?/[0-9a-f]{40})/?$`)
)

type sourceUse struct {
	SHAs  []string `json:"sources"`
	Start int      `json:"start"`
	End   int      `json:"end"`
}

type cachedAttribution struct {
	NotesHash string      `json:"notes_hash"`
	Groups    []sourceUse `json:"groups"`
}

// attributedSources accepts ordinary multiline list items and nested action
// lists. A trailing group of distinct source tokens attributes each concern.
// A source may recur in other concerns, but never twice in the same group.
func attributedSources(notes string, commits engine.CommitLog, scoped bool) ([]sourceUse, error) {
	known := make(map[string]bool, len(commits.Commits))
	for _, commit := range commits.Commits {
		if !sourceTokenRE.MatchString(engine.SourceToken(commit.SHA)) || known[commit.SHA] {
			return nil, errors.New("invalid collected source identity")
		}
		known[commit.SHA] = true
	}
	// A model may copy a short object-name prefix instead of the full token.
	// Resolve it only when it names exactly one selected commit; an ambiguous
	// or unknown prefix still fails.
	resolve := func(token string) (string, bool) {
		token = strings.ToLower(token)
		if known[token] {
			return token, true
		}
		match := ""
		for sha := range known {
			if !strings.HasPrefix(sha, token) {
				continue
			}
			if match != "" {
				return "", false
			}
			match = sha
		}
		return match, match != ""
	}
	matches := sourceTokenRE.FindAllStringSubmatchIndex(notes, -1)
	if len(reservedSourceRE.FindAllStringIndex(notes, -1)) != len(matches) {
		return nil, errors.New("malformed source token")
	}
	seen := make(map[string]bool, len(commits.Commits))
	uses := make([]sourceUse, 0, len(matches))
	for _, m := range matches {
		sha, ok := resolve(notes[m[2]:m[3]])
		if !ok {
			return nil, errors.New("unknown source identity")
		}
		seen[sha] = true
		if n := len(uses); n > 0 && !strings.ContainsAny(notes[uses[n-1].End:m[0]], "\r\n") && strings.TrimSpace(notes[uses[n-1].End:m[0]]) == "" {
			if slices.Contains(uses[n-1].SHAs, sha) {
				return nil, errors.New("duplicate source identity in attribution group")
			}
			uses[n-1].SHAs = append(uses[n-1].SHAs, sha)
			uses[n-1].End = m[1]
		} else {
			uses = append(uses, sourceUse{SHAs: []string{sha}, Start: m[0], End: m[1]})
		}
	}
	if !scoped && len(seen) != len(known) {
		return nil, errors.New("missing source identity")
	}
	if err := checkSourcePlacement(notes, uses); err != nil {
		return nil, err
	}
	return uses, nil
}

func checkSourcePlacement(notes string, uses []sourceUse) error {
	item, itemSources, items := false, 0, 0
	paragraph, blank := false, false
	contentIndent, nestedIndent := 0, 0
	fence, fenceLen := byte(0), 0
	matchIndex, offset := 0, 0
	finishItem := func() bool {
		return !item || itemSources == 1
	}
	lines := strings.SplitAfter(notes, "\n")
	// A line that introduces a nested list is scaffolding, not primary prose.
	nestedListLeadIn := func(from, column int) bool {
		for _, next := range lines[from+1:] {
			next = strings.TrimSuffix(strings.TrimSuffix(next, "\n"), "\r")
			if strings.TrimSpace(next) == "" {
				continue
			}
			nextPrefix := primaryItemRE.FindStringIndex(next)
			return nextPrefix != nil && len(next)-len(strings.TrimLeft(next, " \t")) >= column
		}
		return false
	}
	for lineIndex, line := range lines {
		text := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		trimmed := strings.TrimSpace(text)
		indent := len(text) - len(strings.TrimLeft(text, " \t"))
		prefix := primaryItemRE.FindStringIndex(text)
		isFence := false
		if len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
			n := 0
			for n < len(trimmed) && trimmed[n] == trimmed[0] {
				n++
			}
			if n >= 3 {
				isFence = true
				if fence == 0 {
					fence, fenceLen = trimmed[0], n
				} else if fence == trimmed[0] && n >= fenceLen && strings.TrimSpace(trimmed[n:]) == "" {
					fence = 0
				}
			}
		}
		if fence == 0 && !isFence {
			comment := strings.Index(text, "<!--")
			if htmlBlockRE.MatchString(text) || (comment >= 0 && !unclosedCodeSpan(text[:comment])) {
				return errors.New("HTML blocks and comments cannot contain primary items")
			}
			if prefix != nil && (!item || indent < contentIndent) {
				if !finishItem() {
					return errors.New("primary item has no source")
				}
				item, paragraph, itemSources, items = true, true, 0, items+1
				contentIndent, nestedIndent = prefix[1], 0
			} else if item && (headingRE.MatchString(text) || strings.HasPrefix(trimmed, ">") || (blank && trimmed != "" && indent < contentIndent)) {
				if !finishItem() {
					return errors.New("primary item has no source")
				}
				item, paragraph = false, false
			} else if item && (trimmed == "" || prefix != nil || indent >= contentIndent+4) {
				paragraph = false
				if prefix != nil {
					nestedIndent = indent
				}
			} else if item && indent >= contentIndent && !(nestedIndent > 0 && indent > nestedIndent) {
				// Prose back at the primary item's column is part of the
				// primary item, even when a nested action precedes it. A short
				// label that introduces a nested list stays scaffolding.
				if strings.HasSuffix(trimmed, ":") && nestedListLeadIn(lineIndex, contentIndent) {
					paragraph = false
				} else {
					paragraph = true
					nestedIndent = 0
				}
			}
		} else {
			paragraph = false
		}
		if item && paragraph && itemSources != 0 && trimmed != "" {
			return errors.New("prose follows source attribution")
		}
		for matchIndex < len(uses) && uses[matchIndex].Start < offset+len(line) {
			m := uses[matchIndex]
			localStart, localEnd := m.Start-offset, m.End-offset
			if !item || !paragraph || isFence || fence != 0 || itemSources != 0 ||
				localStart == 0 || (text[localStart-1] != ' ' && text[localStart-1] != '\t') ||
				strings.TrimSpace(text[localEnd:]) != "" || unclosedCodeSpan(text[:localStart]) ||
				(prefix != nil && prefix[1] <= localStart && strings.TrimSpace(text[prefix[1]:localStart]) == "") {
				return errors.New("invalid source attribution")
			}
			itemSources++
			matchIndex++
		}
		blank = trimmed == ""
		offset += len(line)
	}
	if !finishItem() || matchIndex != len(uses) || fence != 0 || (items == 0 && strings.TrimSpace(notes) == "") {
		return errors.New("incomplete source attribution")
	}
	return nil
}

func unclosedCodeSpan(text string) bool {
	open := 0
	for i := 0; i < len(text); i++ {
		if text[i] != '`' {
			continue
		}
		n := 1
		for i+n < len(text) && text[i+n] == '`' {
			n++
		}
		if open == 0 {
			open = n
		} else if open == n {
			open = 0
		}
		i += n - 1
	}
	return open != 0
}

// validateModelLinks accepts source-provided documentation URLs. PR and commit
// links belong exclusively to the backend. Unsupported link syntax fails
// closed rather than hiding an unchecked target in Markdown or HTML.
func validateModelLinks(notes, source string) error {
	// Source groups have already passed identity and placement checks. Their
	// adjacent brackets are transport metadata, not reference-style links.
	// Keep the original notes for every URL and target check below.
	linkSyntax := notes
	if strings.Contains(notes, "[[annalist:") {
		linkSyntax = sourceTokenRE.ReplaceAllString(notes, "")
	}
	if otherLinkRE.MatchString(linkSyntax) {
		return errors.New("unsupported model link syntax")
	}
	allowed := make(map[string]bool)
	for _, raw := range webURLRE.FindAllString(source, -1) {
		allowed[strings.TrimRight(raw, ".,!?)]}")] = true
	}
	check := func(raw string) bool {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return false
		}
		if providerReferencePathRE.MatchString(u.Path) {
			return false
		}
		return allowed[raw]
	}
	for rest := notes; ; {
		start := strings.Index(rest, "](")
		if start < 0 {
			break
		}
		rest = rest[start+2:]
		end := strings.IndexByte(rest, ')')
		if end < 0 || !check(rest[:end]) {
			return errors.New("unverified model link")
		}
		rest = rest[end+1:]
	}
	for _, raw := range webURLRE.FindAllString(notes, -1) {
		if !check(strings.TrimRight(raw, ".,!?)]}")) {
			return errors.New("unverified model URL")
		}
	}
	return nil
}

// validateCachedCitations rechecks private source groups against the exact
// linked Markdown and current source selection without provider lookups.
func (p *Pipeline) validateCachedCitations(notes, source, attribution string, commits engine.CommitLog, spec Spec, scoped bool) error {
	if reservedSourceRE.MatchString(notes) {
		return errors.New("cached source token")
	}
	var cached cachedAttribution
	if err := json.Unmarshal([]byte(attribution), &cached); err != nil || cached.NotesHash != fmt.Sprintf("%x", sha256.Sum256([]byte(notes))) {
		return errors.New("missing or mismatched cached attribution")
	}
	groups := cachedCitationRE.FindAllStringIndex(notes, -1)
	if len(groups) != len(cached.Groups) {
		return errors.New("cached attribution group count differs")
	}
	known := make(map[string]bool, len(commits.Commits))
	for _, commit := range commits.Commits {
		if !sourceTokenRE.MatchString(engine.SourceToken(commit.SHA)) || known[commit.SHA] {
			return errors.New("invalid collected source identity")
		}
		known[commit.SHA] = true
	}
	root, pullPath := "https://github.com", "pull"
	if spec.Platform == "forgejo" {
		root, pullPath = p.Cfg.Forgejo.URL, "pulls"
	}
	seen := make(map[string]bool, len(commits.Commits))
	var withoutCitations strings.Builder
	start := 0
	for i, group := range groups {
		use := cached.Groups[i]
		if use.Start != group[0] || use.End != group[1] || len(use.SHAs) == 0 {
			return errors.New("invalid cached attribution span")
		}
		for j, sha := range use.SHAs {
			if !known[sha] || slices.Contains(use.SHAs[:j], sha) {
				return errors.New("unknown or duplicate cached source identity")
			}
			seen[sha] = true
		}
		links := citationLinkRE.FindAllStringSubmatch(notes[group[0]:group[1]], -1)
		lastPR, lastCommit := 0, ""
		for _, link := range links {
			if strings.HasPrefix(link[1], "#") {
				number, err := strconv.Atoi(link[1][1:])
				if err != nil || number <= lastPR || lastCommit != "" {
					return errors.New("invalid cached PR number")
				}
				if err := ValidateSourceURL(root, spec.Owner, spec.Repo, pullPath, strconv.Itoa(number), link[2]); err != nil {
					return err
				}
				lastPR = number
			} else {
				u, err := url.Parse(link[2])
				if err != nil {
					return err
				}
				sha := u.Path[strings.LastIndex(u.Path, "/")+1:]
				if !slices.Contains(use.SHAs, sha) || sha <= lastCommit || link[1] != sha[:7] {
					return errors.New("invalid cached commit identity")
				}
				if err := ValidateSourceURL(root, spec.Owner, spec.Repo, "commit", sha, link[2]); err != nil {
					return err
				}
				lastCommit = sha
			}
		}
		withoutCitations.WriteString(notes[start:group[0]])
		start = group[1]
	}
	withoutCitations.WriteString(notes[start:])
	if !scoped && len(seen) != len(known) {
		return errors.New("missing cached source identity")
	}
	if err := checkSourcePlacement(notes, cached.Groups); err != nil {
		return err
	}
	return validateModelLinks(withoutCitations.String(), source)
}

func (p *Pipeline) linkSources(ctx context.Context, platform Platform, spec Spec, raw string, uses []sourceUse, mergeSHAs []string) (string, string, error) {
	var shas []string
	seen := make(map[string]bool)
	for _, use := range uses {
		for _, sha := range use.SHAs {
			if !seen[sha] {
				shas = append(shas, sha)
				seen[sha] = true
			}
		}
	}
	var refs map[string]CommitReference
	if len(shas) != 0 {
		var err error
		refs, err = platform.ResolveCommitReferences(ctx, spec.Owner, spec.Repo, shas, mergeSHAs)
		if err != nil {
			return "", "", &SourceReferenceError{Platform: spec.Platform, Err: err}
		}
	}
	root, pullPath := "https://github.com", "pull"
	if spec.Platform == "forgejo" {
		root, pullPath = p.Cfg.Forgejo.URL, "pulls"
	}
	for _, sha := range shas {
		ref, ok := refs[sha]
		if !ok {
			return "", "", &SourceReferenceError{Platform: spec.Platform, Err: errors.New("provider omitted a source reference")}
		}
		if len(ref.PullRequests) != 0 {
			if ref.CommitURL != "" {
				return "", "", &SourceReferenceError{Platform: spec.Platform, Err: errors.New("ambiguous source reference")}
			}
			slices.SortFunc(ref.PullRequests, func(a, b PullRequestReference) int { return cmp.Compare(a.Number, b.Number) })
			last := 0
			for _, pr := range ref.PullRequests {
				if pr.Number <= last {
					return "", "", &SourceReferenceError{Platform: spec.Platform, Err: errors.New("duplicate or invalid pull request")}
				}
				if err := ValidateSourceURL(root, spec.Owner, spec.Repo, pullPath, strconv.Itoa(pr.Number), pr.URL); err != nil {
					return "", "", &SourceReferenceError{Platform: spec.Platform, Err: err}
				}
				last = pr.Number
			}
		} else if err := ValidateSourceURL(root, spec.Owner, spec.Repo, "commit", sha, ref.CommitURL); err != nil {
			return "", "", &SourceReferenceError{Platform: spec.Platform, Err: err}
		}
	}
	var rendered strings.Builder
	cachedGroups := make([]sourceUse, 0, len(uses))
	start := 0
	for _, use := range uses {
		var prs []PullRequestReference
		var commits []string
		for _, sha := range use.SHAs {
			ref := refs[sha]
			for _, pr := range ref.PullRequests {
				if !slices.ContainsFunc(prs, func(existing PullRequestReference) bool { return existing.Number == pr.Number }) {
					prs = append(prs, pr)
				}
			}
			if len(ref.PullRequests) == 0 {
				commits = append(commits, sha)
			}
		}
		slices.SortFunc(prs, func(a, b PullRequestReference) int { return cmp.Compare(a.Number, b.Number) })
		slices.Sort(commits)
		rendered.WriteString(raw[start:use.Start])
		groupStart := rendered.Len()
		rendered.WriteByte('(')
		separator := ""
		for _, pr := range prs {
			rendered.WriteString(separator)
			rendered.WriteString("[#" + strconv.Itoa(pr.Number) + "](" + pr.URL + ")")
			separator = ", "
		}
		for _, sha := range commits {
			rendered.WriteString(separator)
			rendered.WriteString("[" + sha[:7] + "](" + refs[sha].CommitURL + ")")
			separator = ", "
		}
		rendered.WriteByte(')')
		cachedGroups = append(cachedGroups, sourceUse{SHAs: use.SHAs, Start: groupStart, End: rendered.Len()})
		start = use.End
	}
	rendered.WriteString(raw[start:])
	notes := rendered.String()
	attribution, err := json.Marshal(cachedAttribution{NotesHash: fmt.Sprintf("%x", sha256.Sum256([]byte(notes))), Groups: cachedGroups})
	if err != nil {
		return "", "", fmt.Errorf("pipeline: encode source attribution: %w", err)
	}
	return notes, string(attribution), nil
}
