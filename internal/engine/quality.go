package engine

import (
	"regexp"
	"strings"
	"unicode"
)

// CheckNotes returns deterministic quality issues for built-in release notes.
// These bounded lexical checks do not establish semantic fidelity.
func CheckNotes(r Resolved, notes string) []string {
	if r.Instructions != "" {
		return nil
	}
	if r.Tone != "" {
		if _, builtin := Personas[r.Tone]; !builtin {
			return nil
		}
	}
	var issues []string
	if r.Tone == "engineer" && hasProseSemicolon(notes) {
		issues = append(issues, "strict_prose_semicolon")
	}
	if hasOversizedPrimaryParagraph(notes) {
		issues = append(issues, "primary_bullet_overloaded")
	}
	return issues
}

var qualityPrimaryItemRE = regexp.MustCompile(`^( {0,3})(?:[-+*]|[0-9]{1,9}[.)])[ \t]+`)

// Primary prose paragraphs are bounded independently. Nested actions, code,
// commands, URLs, and identifiers are not generic release-note prose.
func hasOversizedPrimaryParagraph(notes string) bool {
	item, paragraph, nested, fenced := false, false, false, false
	contentIndent, words, sentences := 0, 0, 0
	sentenceOpen := false
	oversized := func() bool {
		n := sentences
		if sentenceOpen {
			n++
		}
		return n > 2 || words > 60
	}
	for line := range strings.SplitSeq(notes, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if oversized() {
				return true
			}
			fenced, paragraph = !fenced, false
			words, sentences, sentenceOpen = 0, 0, false
			continue
		}
		if fenced {
			continue
		}
		prefix := qualityPrimaryItemRE.FindStringIndex(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if prefix != nil && (!item || indent < contentIndent) {
			if oversized() {
				return true
			}
			item, paragraph, nested = true, true, false
			contentIndent, words, sentences, sentenceOpen = prefix[1], 0, 0, false
			line = line[prefix[1]:]
		} else if trimmed == "" || prefix != nil || indent >= contentIndent+4 || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ">") {
			if oversized() {
				return true
			}
			if prefix != nil {
				nested = true
			}
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ">") {
				item = false
			}
			paragraph = false
			words, sentences, sentenceOpen = 0, 0, false
			continue
		} else if !paragraph {
			if !item || nested || indent < contentIndent {
				continue
			}
			paragraph = true
		}
		if !commandLine(line) {
			countPrimaryProse(line, &words, &sentences, &sentenceOpen)
		}
	}
	return oversized()
}

func countPrimaryProse(line string, words, sentences *int, sentenceOpen *bool) {
	for i := 0; i < len(line); {
		if isSpace(line[i]) {
			i++
			continue
		}
		if line[i] == '`' || line[i] == '"' || line[i] == '\'' {
			i = skipDelimited(line, i, line[i], len(line))
			continue
		}
		start := i
		for i < len(line) && !isSpace(line[i]) {
			i++
		}
		token := line[start:i]
		word := strings.Trim(token, "*()[]{}.,!?;:\"'")
		if strings.ContainsAny(word, "_/\\=@:") || strings.Contains(word, ".") || strings.HasPrefix(word, "-") || strings.ContainsAny(token, "`") {
			continue
		}
		prose := false
		for _, r := range word {
			if unicode.IsDigit(r) {
				prose = false
				break
			}
			prose = prose || unicode.IsLetter(r)
		}
		if prose {
			*words++
			*sentenceOpen = true
		}
		ending := strings.TrimRight(token, "*()[]{}\"'")
		if *sentenceOpen && (strings.HasSuffix(ending, ".") || strings.HasSuffix(ending, "!") || strings.HasSuffix(ending, "?")) {
			*sentences++
			*sentenceOpen = false
		}
	}
}

// hasProseSemicolon ignores recognizable code, command, URL, identifier, and
// quoted spans. This deliberately uses lexical boundaries rather than trying
// to infer the meaning of an entire line or sentence.
func hasProseSemicolon(s string) bool {
	for lineStart := 0; lineStart < len(s); {
		lineEnd := strings.IndexByte(s[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(s)
		} else {
			lineEnd += lineStart
		}
		if commandLine(s[lineStart:lineEnd]) {
			lineStart = lineEnd + 1
			continue
		}
		for i := lineStart; i < lineEnd; {
			switch {
			case s[i] == '`':
				i = skipDelimited(s, i, '`', lineEnd)
			case s[i] == '\'' || s[i] == '"':
				i = skipDelimited(s, i, s[i], lineEnd)
			case strings.HasPrefix(s[i:lineEnd], "https://") || strings.HasPrefix(s[i:lineEnd], "http://"):
				for i < lineEnd && !isSpace(s[i]) {
					i++
				}
			case isIdentifierByte(s[i]):
				for i < lineEnd && isIdentifierByte(s[i]) {
					i++
				}
			default:
				if s[i] == ';' {
					return true
				}
				i++
			}
		}
		lineStart = lineEnd + 1
	}
	return false
}

func commandLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	for _, segment := range strings.Split(line, ";") {
		segment = strings.TrimSpace(segment)
		if strings.HasPrefix(segment, "$ ") {
			segment = strings.TrimSpace(strings.TrimPrefix(segment, "$ "))
		}
		if !isCommandSegment(segment) {
			return false
		}
	}
	return true
}

func isCommandSegment(line string) bool {
	for _, cmd := range []string{"curl ", "wget ", "go ", "git ", "npm ", "pnpm ", "yarn ", "docker ", "kubectl ", "make ", "migratectl "} {
		if strings.HasPrefix(line, cmd) {
			return true
		}
	}
	return false
}

func skipDelimited(s string, start int, delimiter byte, end int) int {
	for i := start + 1; i < end; i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == delimiter {
			return i + 1
		}
	}
	return end
}

func isIdentifierByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\r' }
