package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// profileManifestPath is the repository-owned, platform-neutral profile
// manifest. It is read pinned to the source tag (see resolveProfile).
const profileManifestPath = ".annalist/release-notes.yaml"

// ErrInvalidProfile wraps a malformed profile request value (a non-empty
// profile name that fails the manifest name syntax).
var ErrInvalidProfile = errors.New("invalid profile")

// ErrInvalidDisplayVersion wraps a malformed display_version request value
// (blank, over the byte budget, or containing ASCII control bytes).
var ErrInvalidDisplayVersion = errors.New("invalid display version")

// ErrProfileConfig wraps repository profile-configuration failures: a missing
// manifest, a malformed manifest, an invalid manifest entry, a syntactically
// valid profile name that is absent from the manifest, or a missing/empty
// prompt file. Non-404 platform read failures do NOT wrap this sentinel —
// they propagate as infrastructure errors.
var ErrProfileConfig = errors.New("profile configuration")

// profileNameRE is the only accepted profile-name syntax: lowercase
// alphanumerics, '_', and '-', starting alphanumeric, at most 64 characters.
var profileNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// profileManifest is the strict, versioned shape of
// .annalist/release-notes.yaml. Nothing else is accepted (unknown fields are
// rejected by the decoder, a second YAML document is rejected by the caller).
type profileManifest struct {
	Version  int                          `yaml:"version"`
	Profiles map[string]profileDefinition `yaml:"profiles"`
}

// profileDefinition is a single manifest entry: the repository-root-relative
// prompt file that becomes the entire system prompt when the profile is
// selected.
type profileDefinition struct {
	Prompt string `yaml:"prompt"`
}

// validateProfileRequest reports whether a requested profile name is
// acceptable in a request: empty (legacy path) or syntactically valid.
func validateProfileRequest(name string) error {
	if name == "" {
		return nil
	}
	if !profileNameRE.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidProfile, name)
	}
	return nil
}

// maxDisplayVersionBytes bounds the presentation version length.
const maxDisplayVersionBytes = 128

// validateDisplayVersion trims the value and requires at most
// maxDisplayVersionBytes and no ASCII control bytes. It returns the trimmed
// value for use downstream.
func validateDisplayVersion(v string) (string, error) {
	trimmed := strings.TrimSpace(v)
	if len(trimmed) > maxDisplayVersionBytes {
		return "", fmt.Errorf("%w: exceeds %d bytes", ErrInvalidDisplayVersion, maxDisplayVersionBytes)
	}
	for i, b := range trimmed {
		if b < 0x20 || b == 0x7f {
			return "", fmt.Errorf("%w: ASCII control byte 0x%02x at byte %d", ErrInvalidDisplayVersion, b, i)
		}
	}
	return trimmed, nil
}

// validatePromptPath enforces the manifest prompt-path contract: a non-empty,
// repository-root-relative slash path ending in .md, with no absolute
// prefix, empty or "." segments, or ".." traversal.
func validatePromptPath(p string) error {
	if p == "" {
		return errors.New("prompt path is empty")
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("prompt path %q is absolute", p)
	}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "":
			return fmt.Errorf("prompt path %q has an empty segment", p)
		case ".":
			return fmt.Errorf("prompt path %q has a \".\" segment", p)
		case "..":
			return fmt.Errorf("prompt path %q has a \"..\" traversal segment", p)
		}
	}
	if !strings.HasSuffix(p, ".md") {
		return fmt.Errorf("prompt path %q must end in .md", p)
	}
	return nil
}

// resolveProfile loads the repository's profile manifest pinned to ref,
// validates EVERY manifest entry (so a repository never has a partially
// valid configuration), and returns the selected profile's prompt content.
// The returned prompt is the entire authoritative system prompt: the
// provider-specific legacy instructions file is intentionally not read or
// composed.
//
// Error mapping: a missing manifest, a missing named profile, an invalid
// manifest (version, name, path, unknown field, second document), or a
// missing/empty prompt file wraps ErrProfileConfig. A non-404 platform read
// failure propagates unchanged as an infrastructure error.
func resolveProfile(ctx context.Context, platform RepoFileReader, owner, repo, requested, ref string) (string, error) {
	raw, err := platform.ReadRepoFile(ctx, owner, repo, profileManifestPath, ref)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", fmt.Errorf("%w: %s not found (no profile configuration)", ErrProfileConfig, profileManifestPath)
		}
		return "", err
	}

	dec := yaml.NewDecoder(strings.NewReader(raw))
	dec.KnownFields(true)
	var manifest profileManifest
	if err := dec.Decode(&manifest); err != nil {
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("%w: %s is empty", ErrProfileConfig, profileManifestPath)
		}
		return "", fmt.Errorf("%w: %s: %v", ErrProfileConfig, profileManifestPath, err)
	}
	// A second decode must find end-of-input: only one YAML document is
	// allowed.
	var probe struct{}
	if err := dec.Decode(&probe); err == nil {
		return "", fmt.Errorf("%w: %s: unexpected second YAML document", ErrProfileConfig, profileManifestPath)
	} else if !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("%w: %s: %v", ErrProfileConfig, profileManifestPath, err)
	}
	if manifest.Version != 1 {
		return "", fmt.Errorf("%w: %s: unsupported version %d (want 1)", ErrProfileConfig, profileManifestPath, manifest.Version)
	}
	if len(manifest.Profiles) == 0 {
		return "", fmt.Errorf("%w: %s: at least one profile is required", ErrProfileConfig, profileManifestPath)
	}
	// Validate every entry, not only the requested one: a repository must
	// never be able to carry a partially valid configuration.
	for name, def := range manifest.Profiles {
		if !profileNameRE.MatchString(name) {
			return "", fmt.Errorf("%w: %s: invalid profile name %q", ErrProfileConfig, profileManifestPath, name)
		}
		if err := validatePromptPath(def.Prompt); err != nil {
			return "", fmt.Errorf("%w: %s: profile %q: %v", ErrProfileConfig, profileManifestPath, name, err)
		}
	}
	def, ok := manifest.Profiles[requested]
	if !ok {
		return "", fmt.Errorf("%w: profile %q not defined in %s", ErrProfileConfig, requested, profileManifestPath)
	}

	prompt, err := platform.ReadRepoFile(ctx, owner, repo, def.Prompt, ref)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", fmt.Errorf("%w: profile %q prompt %s not found", ErrProfileConfig, requested, def.Prompt)
		}
		return "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("%w: profile %q prompt %s is empty", ErrProfileConfig, requested, def.Prompt)
	}
	return prompt, nil
}
