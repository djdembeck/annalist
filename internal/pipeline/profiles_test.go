package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fileReader is a minimal RepoFileReader backed by a path->content map for
// resolveProfile unit tests.
type fileReader struct {
	files map[string]string
}

func (f *fileReader) ReadRepoFile(ctx context.Context, owner, repo, path, ref string) (string, error) {
	if c, ok := f.files[path]; ok {
		return c, nil
	}
	return "", ErrNotFound
}

const validManifest = `version: 1
profiles:
  maintainer:
    prompt: .annalist/prompts/maintainer.md
  customer:
    prompt: .annalist/prompts/customer.md
  compact:
    prompt: .annalist/prompts/compact.md
`

func manifestReader(manifest string, prompts map[string]string) *fileReader {
	f := &fileReader{files: map[string]string{}}
	if manifest != "" {
		f.files[profileManifestPath] = manifest
	}
	for p, c := range prompts {
		f.files[p] = c
	}
	return f
}

func prompts() map[string]string {
	return map[string]string{
		".annalist/prompts/maintainer.md": "MAINTAINER PROMPT",
		".annalist/prompts/customer.md":   "CUSTOMER PROMPT",
		".annalist/prompts/compact.md":    "COMPACT PROMPT",
	}
}

func TestResolveProfileSelectsPrompt(t *testing.T) {
	for name, want := range map[string]string{
		"maintainer": "MAINTAINER PROMPT",
		"customer":   "CUSTOMER PROMPT",
		"compact":    "COMPACT PROMPT",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := resolveProfile(context.Background(), manifestReader(validManifest, prompts()), "o", "r", name, "v1.0.0")
			if err != nil {
				t.Fatalf("resolveProfile(%q): %v", name, err)
			}
			if got != want {
				t.Errorf("prompt = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveProfileRejectsManifestErrors(t *testing.T) {
	cases := map[string]struct {
		manifest  string
		prompts   map[string]string
		requested string
	}{
		"unknown field": {
			manifest: "version: 1\nextra: true\nprofiles:\n  a:\n    prompt: .annalist/prompts/a.md\n",
			prompts:  map[string]string{".annalist/prompts/a.md": "A"},
		},
		"unknown profile entry field": {
			manifest: "version: 1\nprofiles:\n  a:\n    prompt: .annalist/prompts/a.md\n    tone: x\n",
			prompts:  map[string]string{".annalist/prompts/a.md": "A"},
		},
		"unsupported version": {
			manifest: "version: 2\nprofiles:\n  a:\n    prompt: .annalist/prompts/a.md\n",
			prompts:  map[string]string{".annalist/prompts/a.md": "A"},
		},
		"missing version": {
			manifest: "profiles:\n  a:\n    prompt: .annalist/prompts/a.md\n",
			prompts:  map[string]string{".annalist/prompts/a.md": "A"},
		},
		"no profiles": {
			manifest: "version: 1\n",
			prompts:  map[string]string{".annalist/prompts/a.md": "A"},
		},
		"invalid profile name uppercase": {
			manifest: "version: 1\nprofiles:\n  Bad:\n    prompt: .annalist/prompts/a.md\n",
			prompts:  map[string]string{".annalist/prompts/a.md": "A"},
		},
		"invalid profile name leading dash": {
			manifest: "version: 1\nprofiles:\n  -bad:\n    prompt: .annalist/prompts/a.md\n",
			prompts:  map[string]string{".annalist/prompts/a.md": "A"},
		},
		"invalid profile name too long": {
			manifest: "version: 1\nprofiles:\n  " + strings.Repeat("a", 65) + ":\n    prompt: .annalist/prompts/a.md\n",
			prompts:  map[string]string{".annalist/prompts/a.md": "A"},
		},
		"absolute prompt path": {
			manifest: "version: 1\nprofiles:\n  a:\n    prompt: /etc/passwd.md\n",
			prompts:  map[string]string{"/etc/passwd.md": "A"},
		},
		"traversal prompt path": {
			manifest: "version: 1\nprofiles:\n  a:\n    prompt: ../secret.md\n",
			prompts:  map[string]string{"../secret.md": "A"},
		},
		"dot segment prompt path": {
			manifest: "version: 1\nprofiles:\n  a:\n    prompt: .annalist/./prompts/a.md\n",
			prompts:  map[string]string{".annalist/./prompts/a.md": "A"},
		},
		"empty segment prompt path": {
			manifest: "version: 1\nprofiles:\n  a:\n    prompt: .annalist//prompts/a.md\n",
			prompts:  map[string]string{".annalist//prompts/a.md": "A"},
		},
		"prompt path without md suffix": {
			manifest: "version: 1\nprofiles:\n  a:\n    prompt: .annalist/prompts/a.txt\n",
			prompts:  map[string]string{".annalist/prompts/a.txt": "A"},
		},
		"empty prompt path": {
			manifest: "version: 1\nprofiles:\n  a:\n    prompt: \"\"\n",
			prompts:  map[string]string{},
		},
		"second yaml document": {
			manifest: "version: 1\nprofiles:\n  a:\n    prompt: .annalist/prompts/a.md\n---\nversion: 1\n",
			prompts:  map[string]string{".annalist/prompts/a.md": "A"},
		},
		"malformed yaml": {
			manifest: "version: [1\n",
			prompts:  map[string]string{},
		},
		"empty manifest": {
			manifest: "",
			prompts:  map[string]string{},
		},
		"unknown requested profile": {
			manifest:  validManifest,
			prompts:   prompts(),
			requested: "nobody",
		},
		"missing prompt file": {
			manifest:  validManifest,
			prompts:   map[string]string{".annalist/prompts/maintainer.md": "MAINTAINER PROMPT"},
			requested: "customer",
		},
		"empty prompt file": {
			manifest:  validManifest,
			prompts:   map[string]string{".annalist/prompts/maintainer.md": "   \n"},
			requested: "maintainer",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			requested := tc.requested
			if requested == "" {
				requested = "maintainer"
			}
			_, err := resolveProfile(context.Background(), manifestReader(tc.manifest, tc.prompts), "o", "r", requested, "v1.0.0")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrProfileConfig) {
				t.Errorf("err = %v, want ErrProfileConfig", err)
			}
		})
	}
}

func TestResolveProfilePropagatesInfraError(t *testing.T) {
	// A non-404 read failure is an infrastructure error, not a profile-config
	// failure: it must NOT wrap ErrProfileConfig.
	reader := failingReader{err: errors.New("api down")}
	_, err := resolveProfile(context.Background(), &reader, "o", "r", "maintainer", "v1.0.0")
	if err == nil || errors.Is(err, ErrProfileConfig) {
		t.Fatalf("err = %v, want non-ErrProfileConfig infrastructure error", err)
	}
	if err.Error() != "api down" {
		t.Fatalf("err = %v, want api down", err)
	}
}

// failingReader returns a fixed error for every read (simulating an
// infrastructure failure, not a 404).
type failingReader struct{ err error }

func (f *failingReader) ReadRepoFile(ctx context.Context, owner, repo, path, ref string) (string, error) {
	return "", f.err
}

func TestResolveProfileValidatesEveryEntry(t *testing.T) {
	// The requested profile is valid, but a sibling entry has a bad path:
	// the whole manifest must fail.
	manifest := `version: 1
profiles:
  good:
    prompt: .annalist/prompts/good.md
  bad:
    prompt: ../escape.md
`
	p := map[string]string{".annalist/prompts/good.md": "GOOD"}
	_, err := resolveProfile(context.Background(), manifestReader(manifest, p), "o", "r", "good", "v1.0.0")
	if err == nil || !errors.Is(err, ErrProfileConfig) {
		t.Fatalf("err = %v, want ErrProfileConfig (sibling entry invalid)", err)
	}
}

func TestValidateProfileRequest(t *testing.T) {
	cases := map[string]bool{
		"":                      true,
		"a":                     true,
		"customer":              true,
		"a-b_c9":                true,
		strings.Repeat("a", 64): true,
		"Bad":                   false,
		"-bad":                  false,
		"_bad":                  false,
		"a b":                   false,
		strings.Repeat("a", 65): false,
	}
	for name, wantOK := range cases {
		err := validateProfileRequest(name)
		if wantOK && err != nil {
			t.Errorf("validateProfileRequest(%q) = %v, want nil", name, err)
		}
		if !wantOK {
			if err == nil {
				t.Errorf("validateProfileRequest(%q) = nil, want error", name)
			}
			if !errors.Is(err, ErrInvalidProfile) {
				t.Errorf("validateProfileRequest(%q) = %v, want ErrInvalidProfile", name, err)
			}
		}
	}
}

func TestValidateDisplayVersion(t *testing.T) {
	ok, err := validateDisplayVersion("  2.0  ")
	if err != nil || ok != "2.0" {
		t.Errorf("validateDisplayVersion(2.0) = (%q, %v), want (2.0, nil)", ok, err)
	}

	long := strings.Repeat("x", 129)
	if _, err := validateDisplayVersion(long); err == nil || !errors.Is(err, ErrInvalidDisplayVersion) {
		t.Errorf("129-byte version: err = %v, want ErrInvalidDisplayVersion", err)
	}
	if _, err := validateDisplayVersion(strings.Repeat("x", 128)); err != nil {
		t.Errorf("128-byte version: err = %v, want nil", err)
	}

	if _, err := validateDisplayVersion("bad\x00byte"); err == nil || !errors.Is(err, ErrInvalidDisplayVersion) {
		t.Errorf("control byte: err = %v, want ErrInvalidDisplayVersion", err)
	}
	if _, err := validateDisplayVersion("tab\tinside"); err == nil || !errors.Is(err, ErrInvalidDisplayVersion) {
		t.Errorf("tab byte: err = %v, want ErrInvalidDisplayVersion", err)
	}
}
