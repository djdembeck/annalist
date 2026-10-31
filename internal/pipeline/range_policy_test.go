package pipeline

// Behavioral regressions for the explicit range floor: the
// previous_published policy, full history for a first release, target
// self-exclusion, unfinished-reservation handling, and legacy / exact
// from_tag precedence.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildReservationOrigin creates a three-commit origin:
//
//	c1 (tag v0.1.0) -- c2 -- c3 (HEAD)
//
// with distinct commit subjects so the generated range is observable.
func buildReservationOrigin(t *testing.T, dir string) string {
	t.Helper()
	origin := filepath.Join(dir, "origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := gitRun(origin, "init", "-q"); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	commit := func(subject, tag string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(origin, "f.txt"), []byte(subject), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := gitRun(origin, "add", "-A"); err != nil {
			t.Fatalf("git add: %v\n%s", err, out)
		}
		if out, err := gitRun(origin, "commit", "-q", "-m", subject); err != nil {
			t.Fatalf("git commit %q: %v\n%s", subject, err, out)
		}
		if tag != "" {
			if out, err := gitRun(origin, "tag", tag); err != nil {
				t.Fatalf("git tag %s: %v\n%s", tag, err, out)
			}
		}
	}
	// Untyped subjects: they survive the commit-type filter, so the
	// generated range is observable in the user message.
	commit("baseline commit one", "v0.1.0")
	commit("hidden fix behind reservation", "")
	commit("visible feature for release", "")
	return origin
}

// fixtureWithList wires the standard fixture with a fresh origin and an
// explicit platform release list for previous_published resolution.
func fixtureWithList(t *testing.T, origin string, list []ReleaseMeta, releases map[string]*Release) (*Pipeline, *llmStub, *fakePlatform) {
	t.Helper()
	pip, stub, f, _ := fixture(t, nil, releases)
	f.origin = origin
	f.releaseList = list
	return pip, stub, f
}

func publishedMeta(tag string) ReleaseMeta {
	return ReleaseMeta{Tag: tag, Draft: false, Prerelease: false}
}

// The unresolved-reservation bug shape: the highest reachable tag (v0.2.0)
// is a reserve-but-never-pushed value, the previous PUBLISHED release is
// v0.1.0, and the target is v0.3.0.
func TestRangePolicyUnfinishedReservationIsSkipped(t *testing.T) {
	origin := buildReservationOrigin(t, t.TempDir())
	for _, tag := range []string{"v0.2.0", "v0.3.0"} {
		if out, err := gitRun(origin, "tag", tag); err != nil {
			t.Fatalf("tag %s: %v\n%s", tag, err, out)
		}
	}
	pip, stub, _ := fixtureWithList(t, origin,
		[]ReleaseMeta{
			{Tag: "v0.1.0", Draft: false, Prerelease: false},
			{Tag: "v0.2.0", Draft: true, Prerelease: false},  // reservation, never published
			{Tag: "v0.3.0", Draft: false, Prerelease: false}, // target's own release
		},
		map[string]*Release{"v0.3.0": {ID: 1, Body: ""}})

	spec := genSpec("v0.3.0", "rp-unfin")
	spec.RangePolicy = RangePreviousPublished
	res, err := pip.GenerateNotes(context.Background(), spec, Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	// The floor must be v0.1.0 (the last PUBLISHED release), NOT the
	// v0.2.0 unfinished reservation: the commit behind the reservation
	// must be visible in the generated range.
	if res.FromTag != "v0.1.0" {
		t.Errorf("FromTag = %q, want v0.1.0 (draft reservation v0.2.0 must be skipped)", res.FromTag)
	}
	if !strings.Contains(stub.user(), "changes since v0.1.0") {
		t.Errorf("user message = %q, want it pinned to the v0.1.0 floor", stub.user())
	}
	if !strings.Contains(stub.user(), "hidden fix behind reservation") {
		t.Errorf("user message = %q, want the reservation-hidden commit in the log", stub.user())
	}
}

func TestRangePolicySelfExclusion(t *testing.T) {
	origin := buildReservationOrigin(t, t.TempDir())
	for _, target := range [2][2]string{{"v0.2.0", "HEAD~1"}, {"v0.3.0", "HEAD"}} {
		if out, err := gitRun(origin, "tag", target[0], target[1]); err != nil {
			t.Fatalf("tag %s: %v\n%s", target[0], err, out)
		}
	}
	pip, stub, _ := fixtureWithList(t, origin,
		[]ReleaseMeta{
			{Tag: "v0.1.0", Draft: false, Prerelease: false},
			{Tag: "v0.2.0", Draft: false, Prerelease: false},
			{Tag: "v0.3.0", Draft: false, Prerelease: false}, // the target's own release
		},
		map[string]*Release{"v0.3.0": {ID: 1, Body: ""}})

	spec := genSpec("v0.3.0", "rp-self")
	spec.RangePolicy = RangePreviousPublished
	res, err := pip.GenerateNotes(context.Background(), spec, Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	// The target's own release must be excluded even though it is
	// published and "ancestral" to itself: floor = v0.2.0.
	if res.FromTag != "v0.2.0" {
		t.Errorf("FromTag = %q, want v0.2.0 (target's own release excluded)", res.FromTag)
	}
	if !strings.Contains(stub.user(), "changes since v0.2.0") {
		t.Errorf("user message = %q, want it pinned to the v0.2.0 floor", stub.user())
	}
}

func TestRangePolicyPublishedFloorStaysInTagStream(t *testing.T) {
	origin := buildReservationOrigin(t, t.TempDir())
	for _, target := range [4][2]string{
		{"docs/v0.1.0", "HEAD~2"}, {"docs/v0.2.0", "HEAD~1"},
		{"other/v0.2.1", "HEAD~1"}, {"docs/v0.3.0", "HEAD"},
	} {
		if out, err := gitRun(origin, "tag", target[0], target[1]); err != nil {
			t.Fatalf("tag %s: %v\n%s", target[0], err, out)
		}
	}
	pip, stub, _ := fixtureWithList(t, origin,
		[]ReleaseMeta{publishedMeta("v0.1.0"), publishedMeta("docs/v0.1.0"),
			{Tag: "docs/v0.2.0", Draft: true}, publishedMeta("other/v0.2.1"), publishedMeta("docs/v0.3.0")},
		map[string]*Release{"docs/v0.3.0": {ID: 1, Body: ""}})
	spec := genSpec("docs/v0.3.0", "rp-stream")
	spec.RangePolicy = RangePreviousPublished
	result, err := pip.GenerateNotes(context.Background(), spec, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.FromTag != "docs/v0.1.0" || !strings.Contains(stub.user(), "hidden fix behind reservation") {
		t.Fatalf("wrong stream/range: floor=%q, message=%q", result.FromTag, stub.user())
	}
}

func TestRangePolicyFirstReleaseIsFullHistory(t *testing.T) {
	// A genuinely first release: one commit, tagged, released.
	origin := t.TempDir()
	if out, err := gitRun(origin, "init", "-q"); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(origin, "f.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := gitRun(origin, "add", "-A"); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := gitRun(origin, "commit", "-q", "-m", "baseline commit one"); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	if out, err := gitRun(origin, "tag", "v0.1.0"); err != nil {
		t.Fatalf("git tag: %v\n%s", err, out)
	}
	pip, stub, _ := fixtureWithList(t, origin,
		// No published release at all: full history (floor = "").
		[]ReleaseMeta{{Tag: "v0.1.0", Draft: true, Prerelease: false}},
		map[string]*Release{"v0.1.0": {ID: 1, Body: ""}})

	spec := genSpec("v0.1.0", "rp-first")
	spec.RangePolicy = RangePreviousPublished
	res, err := pip.GenerateNotes(context.Background(), spec, Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	if res.FromTag != "" {
		t.Errorf("FromTag = %q, want empty (explicit full history, not legacy tag resolution)", res.FromTag)
	}
	// Full history means even the very first commit is in the user message.
	if !strings.Contains(stub.user(), "baseline commit one") {
		t.Errorf("user message = %q, want the very first commit (full history)", stub.user())
	}
}

func TestRangePolicyLegacyOmittedKeepsPreviousTag(t *testing.T) {
	origin := buildReservationOrigin(t, t.TempDir())
	for _, tag := range []string{"v0.2.0", "v0.3.0"} {
		if out, err := gitRun(origin, "tag", tag); err != nil {
			t.Fatalf("tag %s: %v\n%s", tag, err, out)
		}
	}
	pip, _, _ := fixtureWithList(t, origin,
		[]ReleaseMeta{
			{Tag: "v0.1.0", Draft: false, Prerelease: false},
			{Tag: "v0.2.0", Draft: true, Prerelease: false}, // unfinished reservation
		},
		map[string]*Release{"v0.3.0": {ID: 1, Body: ""}})

	// Omits RangePolicy: legacy behavior is the previous valid tag, which
	// DOES land on the reservation (v0.2.0) — retained for compatibility.
	spec := genSpec("v0.3.0", "rp-legacy")
	res, err := pip.GenerateNotes(context.Background(), spec, Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	if res.FromTag != "v0.2.0" {
		t.Errorf("legacy FromTag = %q, want v0.2.0 (legacy previous-tag resolution unchanged)", res.FromTag)
	}
}

func TestRangePolicyExactFromTagWins(t *testing.T) {
	origin := buildReservationOrigin(t, t.TempDir())
	if out, err := gitRun(origin, "tag", "v0.3.0"); err != nil {
		t.Fatalf("tag: %v\n%s", err, out)
	}
	pip, _, _ := fixtureWithList(t, origin,
		[]ReleaseMeta{publishedMeta("v0.1.0")},
		map[string]*Release{"v0.3.0": {ID: 1, Body: ""}})

	spec := genSpec("v0.3.0", "rp-exact")
	spec.FromTag = "v0.2.9" // exact nonempty: wins over the policy
	spec.RangePolicy = RangePreviousPublished
	res, err := pip.GenerateNotes(context.Background(), spec, Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	if res.FromTag != "v0.2.9" {
		t.Errorf("FromTag = %q, want the exact v0.2.9 honored verbatim", res.FromTag)
	}
}

func TestRangePolicyTargetTagMissingFailsClosed(t *testing.T) {
	origin := buildReservationOrigin(t, t.TempDir())
	pip, _, _ := fixtureWithList(t, origin,
		[]ReleaseMeta{publishedMeta("v0.1.0")},
		map[string]*Release{})

	spec := genSpec("v0.3.0", "rp-missing") // no tag v0.3.0 in the clone
	spec.RangePolicy = RangePreviousPublished
	_, err := pip.GenerateNotes(context.Background(), spec, Options{})
	if err == nil || !errors.Is(err, ErrPreviousPublishedResolution) {
		t.Errorf("err = %v, want ErrPreviousPublishedResolution (target tag absent, fail closed)", err)
	}
}

// A published release strictly BETWEEN the floor and the target, sitting on
// a branch that never merged, must not become the floor: it is not an
// ancestor of the target's commit. The ancestors-only guard (not just the
// strictly-lower version guard) is what excludes it.
func TestRangePolicyNonAncestorReleaseIsExcluded(t *testing.T) {
	origin := buildReservationOrigin(t, t.TempDir())
	// c1 -- c2 -- c3
	// The ancestor floor is the PUBLISHED v0.9.0 release at c1; the target
	// is c3. A side release at c2 sits strictly between them on a branch
	// that never merges.
	if out, err := gitRun(origin, "tag", "v0.9.0", "HEAD~2"); err != nil {
		t.Fatalf("tag v0.9.0: %v\n%s", err, out)
	}
	if out, err := gitRun(origin, "tag", "v1.0.0"); err != nil {
		t.Fatalf("tag v1.0.0: %v\n%s", err, out)
	}
	// A side release off c2: higher than the floor, lower than the target,
	// published, but reachable only from a branch that never merges.
	if out, err := gitRun(origin, "checkout", "-q", "-b", "side", "HEAD~1"); err != nil {
		t.Fatalf("checkout side: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(origin, "f.txt"), []byte("side work"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arg := range [][]string{
		{"add", "-A"}, {"commit", "-q", "-m", "unmerged release work"}, {"tag", "v0.9.5"},
		{"checkout", "-q", "-"},
	} {
		if out, err := gitRun(origin, arg...); err != nil {
			t.Fatalf("git %v: %v\n%s", arg, err, out)
		}
	}
	pip, stub, _ := fixtureWithList(t, origin,
		[]ReleaseMeta{
			publishedMeta("v0.9.0"), // the ancestor floor
			publishedMeta("v0.9.5"), // HIGHER, published, NOT an ancestor
			{Tag: "v0.8.6"},         // published, tag absent from the clone
			publishedMeta("v1.0.0"), // the target's own release
		},
		map[string]*Release{"v1.0.0": {ID: 1, Body: ""}})

	spec := genSpec("v1.0.0", "rp-nonanc")
	spec.RangePolicy = RangePreviousPublished
	res, err := pip.GenerateNotes(context.Background(), spec, Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	// v0.9.5 loses on ancestry (v.compare excludes >= target, so only the
	// ancestor guard can drop it): floor = v0.9.0.
	if res.FromTag != "v0.9.0" {
		t.Fatalf("FromTag = %q, want v0.9.0 (unmerged v0.9.5 must stay excluded)", res.FromTag)
	}
	ug := stub.user()
	if !strings.Contains(ug, "hidden fix behind reservation") {
		t.Errorf("user message = %q, want the ancestor c2 commit in the range", ug)
	}
	if strings.Contains(ug, "unmerged release work") {
		t.Errorf("user message = %q, the unmerged side commit must not be in the range", ug)
	}
}
