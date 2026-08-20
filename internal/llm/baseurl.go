package llm

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateBaseURL checks that a base URL is well-formed for the client, which
// appends "/v1/..." to it by string concatenation:
//
//   - It must parse and have a host.
//   - Scheme must be http or https.
//   - Path must be "" or "/": a base URL is scheme://host[:port]; a trailing
//     /v1 (the documented LLM_BASE_URL form) is accepted and stripped by
//     NormalizeBaseURL. Any other path would be swallowed into the request
//     path.
//   - No query string or fragment, not even a bare '?' or '#': the client
//     concatenates the request path onto the base, so either would corrupt
//     it.
//
// It deliberately does not enforce reachability: no DNS resolution, no
// private-range or cloud-metadata checks, and http is allowed for any host.
// The only writers of a base URL are the operator (LLM_BASE_URL env) and the
// admin-gated settings PUT — there is no untrusted writer — and the primary
// use case is a self-hosted LLM, which typically sits on a private IP or a
// LAN-only hostname. A reachability policy rejected exactly that (and made
// the /api/models proxy depend on the API server's DNS), so format is the
// whole job.

// ValidateBaseURL validates the shape of a base URL (see package rules above).
func ValidateBaseURL(u string) error {
	// Normalize first so the documented LLM_BASE_URL form
	// (scheme://host[:port]/v1, with or without a trailing slash) reduces
	// to a plain host before the path guard sees it.
	parsed, err := url.Parse(NormalizeBaseURL(u))
	if err != nil {
		return fmt.Errorf("invalid url: %v", err)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("missing host")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("scheme %q not allowed (use https or http)", parsed.Scheme)
	}
	// The client builds base+"/v1/..." by string concatenation, so a query
	// string or fragment would be swallowed into the request path. A bare
	// "?" (ForceQuery) or a bare "#" (empty fragment) still corrupts the
	// concatenation even though RawQuery/Fragment parse empty, so both are
	// rejected: ForceQuery from the parse result, '#' from the raw input.
	// This check runs before the path check so a query/fragment on an
	// otherwise-accepted form like /v1/ reports the real problem.
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery ||
		strings.Contains(u, "#") {
		return fmt.Errorf("query/fragment not allowed (base url is scheme://host[:port])")
	}
	if p := parsed.Path; p != "" && p != "/" {
		return fmt.Errorf("path %q not allowed (base url is scheme://host[:port], optionally with a trailing /v1)", p)
	}
	return nil
}
