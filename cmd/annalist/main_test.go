package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/djdembeck/annalist/internal/pipeline"
)

func TestPrintGenerationResult(t *testing.T) {
	for _, tc := range []struct {
		name            string
		result          pipeline.Result
		err             error
		code            int
		stdout, warning string
	}{
		{name: "preview", result: pipeline.Result{Outcome: "generated", Notes: "Captured notes."}, stdout: "Captured notes.\n"},
		{name: "review-needed preview", result: pipeline.Result{Outcome: "generated", Notes: "Captured notes; unchanged.", WarningCode: "quality_review_required", QualityIssues: []string{"strict_prose_semicolon", "source_context_incomplete"}}, code: 1, stdout: "Captured notes; unchanged.\n", warning: "Do not publish this preview automatically."},
		{name: "advisory context warning stays successful", result: pipeline.Result{Outcome: "cached", Notes: "Cached notes.", WarningCode: "source_context_incomplete", QualityIssues: []string{"source_context_incomplete"}}, code: 0, stdout: "Cached notes.\n", warning: "Some diff hunks did not fit the source context."},
		{name: "published cache warning remains successful", result: pipeline.Result{Outcome: "published", Notes: "Published notes.", WarningCode: "cache_persistence"}, stdout: "Published notes.\n", warning: "notes were published, but the local note cache could not be saved"},
		{name: "quality rejection", err: fmt.Errorf("untrusted output must not be printed: %w", &pipeline.QualityError{Issues: []string{"strict_prose_semicolon"}}), code: 1, warning: "Publication blocked: notes need quality review. Nothing was published."},
		{name: "unchecked publication", err: &pipeline.QualityError{Issues: []string{"quality_check_required"}}, code: 1, warning: "The notes do not have a current quality check."},
		{name: "invalid attribution preview", result: pipeline.Result{Outcome: "generated", WarningCode: "quality_review_required", QualityIssues: []string{"source_attribution_invalid"}}, code: 1, stdout: "\n", warning: "Source attribution is invalid, so the preview was withheld."},
		{name: "source lookup failure", err: &pipeline.SourceReferenceError{Platform: "github", Err: errors.New("private provider response")}, code: 1, warning: "Pull requests: Read-only"},
		{name: "ordinary failure", err: errors.New("connection unavailable"), code: 1, warning: "generating notes: connection unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := printGenerationResult(&stdout, &stderr, tc.result, tc.err)
			if code != tc.code || stdout.String() != tc.stdout || !strings.Contains(stderr.String(), tc.warning) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if tc.warning == "" && stderr.Len() != 0 {
				t.Fatalf("unexpected warning: %q", stderr.String())
			}
			if strings.Contains(stderr.String(), "untrusted output") || strings.Contains(stderr.String(), "Captured notes") || strings.Contains(stderr.String(), "private provider response") {
				t.Fatalf("warning leaked generated notes: %q", stderr.String())
			}
			if tc.name == "review-needed preview" {
				if !strings.Contains(stderr.String(), "Strict engineer prose contains semicolons.") {
					t.Fatalf("missing blocking reason: %s", stderr.String())
				}
				if strings.Contains(stderr.String(), "Some diff hunks did not fit the source context.") {
					t.Fatalf("advisory context limit reported as blocking: %s", stderr.String())
				}
			}
		})
	}
}
