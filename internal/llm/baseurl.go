package llm

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// cgnatRange is the RFC 6598 shared address space (100.64.0.0/10). It is not
// covered by netip.Addr.IsPrivate but is routable only on private networks
// (carrier-grade NAT), and it includes cloud metadata endpoints
// (Alibaba Cloud uses 100.100.100.200).
var cgnatRange = netip.MustParsePrefix("100.64.0.0/10")

// ValidateBaseURL rejects endpoints that callers would otherwise send outbound
// LLM requests (Chat, ListModels) to, carrying the bearer key. Without this
// guard a user-supplied base URL is an SSRF vector: internal ranges, cloud
// metadata, arbitrary schemes.
//
// It is enforced at every point an effective base URL is dialed: the settings
// PUT handler (before persist), the /api/models proxy (before the request),
// and the webhook-triggered generation path (before the Chat call).
//
// Rules:
//   - Must parse and have a host.
//   - Scheme https; http is allowed only for loopback hosts
//     (localhost/127.0.0.1/::1) for local dev. Loopback is rejected over
//     https — a TLS local endpoint is an operator concern (LLM_BASE_URL env),
//     not a settings value.
//   - Path must be "" or "/": a base URL is scheme://host[:port]; the
//     client appends /v1/... itself. A trailing /v1 (the documented
//     LLM_BASE_URL form) is accepted and stripped by NormalizeBaseURL.
//   - No query string or fragment, not even a bare '?' or '#': the client
//     concatenates the request path onto the base, so either would corrupt
//     it.
//   - https hosts must be public: a literal private, loopback, CGNAT
//     (100.64.0.0/10), link-local unicast (covers the 169.254.169.254
//     cloud-metadata address), link-local multicast, or unspecified address is
//     rejected, as is a v4-mapped-v6 address mapping to one of those ranges.
//   - https hostnames are resolved via lookup; the URL is rejected when the
//     host fails to resolve or any resolved address is in one of those
//     internal ranges. A public self-host (the intended use) resolves to
//     public addresses and passes. Residual DNS-rebinding risk: addresses are
//     checked here, not per outbound request, so a DNS answer that changes
//     between validation and connection is the only remaining gap.
func ValidateBaseURL(u string) error {
	return ValidateBaseURLWith(u, net.LookupHost)
}

// ValidateBaseURLWith is ValidateBaseURL with an injectable DNS lookup, for
// tests that must not touch the network.
func ValidateBaseURLWith(u string, lookup func(string) ([]string, error)) error {
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
		return fmt.Errorf("scheme %q not allowed (use https, or http for a loopback host)", parsed.Scheme)
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
	host := parsed.Hostname() // strips port and IPv6 brackets
	if parsed.Scheme == "http" {
		// Local dev exception: plain http, but only to the local machine.
		if !isLoopbackHost(host) {
			return fmt.Errorf("http is only allowed for loopback hosts, use https")
		}
		return nil
	}
	return checkBaseURLHost(host, lookup)
}

// isLoopbackHost reports whether host is a loopback literal: "localhost",
// 127.0.0.1, or ::1.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// isPublicNetIP reports whether ip (a literal address) is routable outside the
// local/private network: not private, loopback, CGNAT (100.64.0.0/10, which
// IsPrivate does not cover), link-local unicast (which covers the
// 169.254.169.254 cloud-metadata address), link-local multicast, or
// unspecified. v4-mapped-v6 addresses are checked as their v4 form.
func isPublicNetIP(ip netip.Addr) bool {
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	if ip.Is4() && cgnatRange.Contains(ip) {
		return false
	}
	return !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}

// checkBaseURLHost enforces the literal-IP and hostname rules for a base-URL
// host.
func checkBaseURLHost(host string, lookup func(string) ([]string, error)) error {
	if ip, err := netip.ParseAddr(host); err == nil {
		if !isPublicNetIP(ip) {
			return fmt.Errorf("host %s is in a non-public range", host)
		}
		return nil
	}
	addrs, err := lookup(host)
	if err != nil {
		return fmt.Errorf("host %q does not resolve: %v", host, err)
	}
	// Every resolved address must be public: a hostname with even one
	// internal A record (mixed record sets, rebinding-style) must fail.
	// An empty answer fails too — there is no public address to reach.
	for _, a := range addrs {
		ip, err := netip.ParseAddr(a)
		if err != nil || !isPublicNetIP(ip) {
			return fmt.Errorf("host %q resolves to a non-public address", host)
		}
	}
	if len(addrs) == 0 {
		return fmt.Errorf("host %q resolves to no addresses", host)
	}
	return nil
}
