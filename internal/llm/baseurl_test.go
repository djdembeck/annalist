package llm

import (
	"regexp"
	"testing"
)

// TestValidateBaseURL covers the format branches of the base-URL guard:
// scheme, host, path, and query/fragment. It is pure string processing —
// no DNS, no network — so every case is deterministic.
func TestValidateBaseURL(t *testing.T) {
	cases := []struct {
		in      string
		wantErr string // regexp substring; empty means OK
	}{
		{in: "", wantErr: `invalid url|missing host`},
		{in: "https:///v1", wantErr: `missing host`},

		// Scheme: http and https are both allowed for any host.
		{in: "ftp://example.com", wantErr: `scheme "ftp" not allowed`},
		{in: "ws://example.com", wantErr: `scheme "ws" not allowed`},
		{in: "example.com", wantErr: `missing host`}, // no scheme
		{in: "http://api.openai.com", wantErr: ""},
		{in: "http://example.com", wantErr: ""},
		{in: "http://127.0.0.1:8080", wantErr: ""},
		{in: "http://localhost:8080", wantErr: ""},
		{in: "http://[::1]:8080", wantErr: ""},

		// Literal IPs of any kind are accepted: the endpoint is
		// operator-chosen, and self-hosted LLMs commonly sit on private
		// ranges (10.x, 192.168.x, loopback, link-local).
		{in: "http://10.0.105.201:8090", wantErr: ""},
		{in: "https://93.184.216.34", wantErr: ""},
		{in: "https://93.184.216.34:443", wantErr: ""},
		{in: "https://10.0.0.1", wantErr: ""},
		{in: "https://192.168.1.1", wantErr: ""},
		{in: "https://127.0.0.1", wantErr: ""},
		{in: "https://169.254.169.254", wantErr: ""},
		{in: "https://100.100.100.200", wantErr: ""},
		{in: "https://[fd00::1]", wantErr: ""},
		{in: "https://[fe80::1]", wantErr: ""},
		{in: "https://[::ffff:10.0.0.1]", wantErr: ""},

		// Hostnames are accepted without resolution: no DNS in the check.
		{in: "https://api.openai.com", wantErr: ""},
		{in: "https://axonhub.theiahd.nl", wantErr: ""},
		{in: "https://llm.invalid", wantErr: ""},

		// Path: only "" or "/" — the client appends /v1/... itself. A
		// trailing /v1 (the documented LLM_BASE_URL form) is accepted and
		// normalized away; any other path is rejected.
		{in: "https://example.com/", wantErr: ""},
		{in: "https://example.com/extra/path", wantErr: `path "/extra/path" not allowed`},
		{in: "https://api.example.com/v1", wantErr: ""},
		{in: "https://api.example.com/v1/", wantErr: ""},
		{in: "http://10.0.105.201:8090/v1", wantErr: ""},
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
			err := ValidateBaseURL(tc.in)
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
				t.Fatalf("ValidateBaseURL(%q) = %q, want error matching %q", tc.in, err.Error(), tc.wantErr)
			}
		})
	}
}
