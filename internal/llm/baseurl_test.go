package llm

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// TestValidateBaseURL covers the literal-IP, scheme, and path branches of
// the SSRF guard without any network access: hostnames go through a fake
// lookup (public answer, or a lookup error for .invalid names) instead of
// real DNS, so every case is deterministic.
func TestValidateBaseURL(t *testing.T) {
	fakeLookup := func(host string) ([]string, error) {
		if strings.Contains(host, ".invalid") {
			return nil, fmt.Errorf("no such host")
		}
		return []string{"93.184.216.34"}, nil
	}

	cases := []struct {
		in      string
		wantErr string // regexp substring; empty means OK
	}{
		{in: "", wantErr: `invalid url|missing host`},
		{in: "https:///v1", wantErr: `missing host`},

		// Scheme: https always; http only for loopback hosts.
		{in: "ftp://example.com", wantErr: `scheme "ftp" not allowed`},
		{in: "ws://example.com", wantErr: `scheme "ws" not allowed`},
		{in: "example.com", wantErr: `missing host`}, // no scheme
		{in: "http://api.openai.com", wantErr: `http is only allowed for loopback`},
		{in: "http://example.com", wantErr: `http is only allowed for loopback`},
		{in: "http://127.0.0.1:8080", wantErr: ""},
		{in: "http://localhost:8080", wantErr: ""},
		{in: "http://[::1]:8080", wantErr: ""},

		// Literal IPs: public accepted; private, loopback, link-local
		// (covers 169.254.169.254 cloud metadata), and v4-mapped-v6 internal
		// rejected.
		{in: "https://93.184.216.34", wantErr: ""},
		{in: "https://93.184.216.34:443", wantErr: ""},
		{in: "https://10.0.0.1", wantErr: `non-public range`},
		{in: "https://192.168.1.1", wantErr: `non-public range`},
		{in: "https://172.16.0.1", wantErr: `non-public range`},
		{in: "https://127.0.0.1", wantErr: `non-public range`},
		{in: "https://169.254.169.254", wantErr: `non-public range`},
		{in: "https://[::1]", wantErr: `non-public range`},
		{in: "https://[fd00::1]", wantErr: `non-public range`},
		{in: "https://[fe80::1]", wantErr: `non-public range`},
		{in: "https://[::]", wantErr: `non-public range`},
		{in: "https://[::ffff:10.0.0.1]", wantErr: `non-public range`},
		{in: "https://[::ffff:127.0.0.1]", wantErr: `non-public range`},
		{in: "https://[::ffff:169.254.169.254]", wantErr: `non-public range`},
		{in: "https://[::ffff:93.184.216.34]", wantErr: ""},

		// CGNAT / shared address space (RFC 6598 100.64.0.0/10) is not
		// covered by IsPrivate: it must be rejected explicitly, including the
		// Alibaba Cloud metadata address 100.100.100.200 and its
		// v4-mapped-v6 form.
		{in: "https://100.64.0.1", wantErr: `non-public range`},
		{in: "https://100.100.100.200", wantErr: `non-public range`},
		{in: "https://100.127.255.254", wantErr: `non-public range`},
		{in: "https://[::ffff:100.100.100.200]", wantErr: `non-public range`},
		// Just outside the CGNAT range: public and accepted.
		{in: "https://100.63.255.255", wantErr: ""},

		// Hostname branch (fake lookup, no DNS): public resolution passes,
		// unresolvable names fail.
		{in: "https://api.openai.com", wantErr: ""},
		{in: "https://llm.example.com", wantErr: ""},
		{in: "https://unresolvable.invalid", wantErr: `does not resolve`},

		// Path: only "" or "/" — the client appends /v1/... itself. A
		// trailing /v1 (the documented LLM_BASE_URL form) is accepted and
		// normalized away; any other path is rejected.
		{in: "https://example.com/", wantErr: ""},
		{in: "https://example.com/extra/path", wantErr: `path "/extra/path" not allowed`},
		{in: "https://api.example.com/v1", wantErr: ""},
		{in: "https://api.example.com/v1/", wantErr: ""},
		{in: "https://api.example.com/v1/chat", wantErr: `path "/v1/chat" not allowed`},
		{in: "http://127.0.0.1:8080/", wantErr: ""},

		// Query strings and fragments are swallowed into the request path
		// by the client's string-concatenated URL; reject them. A bare "?"
		// (ForceQuery) and a bare "#" (empty fragment) parse with empty
		// RawQuery/Fragment but still corrupt the concatenation, and the
		// accepted /v1 form with a trailing "?" is just as bad.
		{in: "https://api.example.com?x=1", wantErr: `query/fragment not allowed`},
		{in: "https://api.example.com#frag", wantErr: `query/fragment not allowed`},
		{in: "https://api.example.com?", wantErr: `query/fragment not allowed`},
		{in: "https://api.example.com#", wantErr: `query/fragment not allowed`},
		{in: "https://api.example.com/v1?", wantErr: `query/fragment not allowed`},

		// url.Parse failure (invalid port) hits the "invalid url" branch.
		{in: "https://example.com:badport", wantErr: `invalid url`},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			err := ValidateBaseURLWith(tc.in, fakeLookup)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateBaseURL(%q) = %v, want nil", tc.in, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateBaseURL(%q) = nil, want error matching %q", tc.in, tc.wantErr)
			}
			if !regexp.MustCompile(tc.wantErr).MatchString(err.Error()) {
				t.Fatalf("ValidateBaseURL(%q) = %q, want error matching %q", tc.in, err, tc.wantErr)
			}
		})
	}
}

// TestValidateBaseURLHostResolution covers the hostname branch with fake
// lookups (no network): every resolved address must be public — a hostname
// with even one internal A record (mixed record sets, nip.io/xip.io-style
// rebinding) is rejected, as are all-internal sets and lookup failure.
func TestValidateBaseURLHostResolution(t *testing.T) {
	fake := func(addrs ...string) func(string) ([]string, error) {
		return func(string) ([]string, error) { return addrs, nil }
	}

	cases := []struct {
		name    string
		host    string
		lookup  func(string) ([]string, error)
		wantErr string // regexp substring; empty means OK
	}{
		{name: "public only", host: "public.example", lookup: fake("93.184.216.34"), wantErr: ""},
		{name: "all public", host: "public.example", lookup: fake("93.184.216.34", "93.184.216.35"), wantErr: ""},
		{
			name: "mixed public and internal", host: "mixed.example",
			lookup: fake("93.184.216.34", "10.0.0.1"), wantErr: `non-public address`,
		},
		{
			name: "mixed internal first", host: "mixed.example",
			lookup: fake("10.0.0.1", "93.184.216.34"), wantErr: `non-public address`,
		},
		{name: "all internal", host: "internal.example", lookup: fake("10.0.0.1", "192.168.0.1"), wantErr: `non-public address`},
		{name: "metadata only", host: "meta.example", lookup: fake("169.254.169.254"), wantErr: `non-public address`},
		{name: "CGNAT only", host: "meta.example", lookup: fake("100.100.100.200"), wantErr: `non-public address`},
		{name: "CGNAT mixed with public", host: "meta.example", lookup: fake("100.100.100.200", "93.184.216.34"), wantErr: `non-public address`},
		{name: "v4-mapped-v6 internal", host: "v6.example", lookup: fake("::ffff:192.168.1.1"), wantErr: `non-public address`},
		{name: "v4-mapped-v6 public", host: "v6.example", lookup: fake("::ffff:93.184.216.34"), wantErr: ""},
		// An empty answer is rejected too: there is no public address to
		// reach (matches the pre-guard "resolves only to non-public" rule).
		{name: "empty resolution", host: "empty.example", lookup: fake(), wantErr: `resolves to no addresses`},
		{
			name: "lookup failure", host: "down.example",
			lookup:  func(string) ([]string, error) { return nil, fmt.Errorf("no such host") },
			wantErr: `does not resolve`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBaseURLWith("https://"+tc.host, tc.lookup)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateBaseURL(https://%s, %s) = %v, want nil", tc.host, tc.name, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateBaseURL(https://%s, %s) = nil, want error matching %q", tc.host, tc.name, tc.wantErr)
			}
			if !regexp.MustCompile(tc.wantErr).MatchString(err.Error()) {
				t.Fatalf("ValidateBaseURL(https://%s, %s) = %q, want error matching %q", tc.host, tc.name, err, tc.wantErr)
			}
		})
	}
}
