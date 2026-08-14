package riverqueue

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/githubbuild"
)

// The build.failed notification must never carry a raw Go error. A spec failure
// passes its own reason through, because it names the commit or the offending
// line; every other bucket gets a sentence chosen from the error's type.
func TestBuildFailureReason(t *testing.T) {
	cases := []struct {
		name string
		// passthrough marks the buckets that deliberately forward the cause's own
		// wording, so the leak check below does not apply to them.
		passthrough bool
		cause       error
		want        string
	}{
		{name: "no cause", cause: nil, want: ""},
		{
			name:  "container build failure",
			cause: githubbuild.PermanentError{Err: fmt.Errorf("build agent: %w", githubbuild.BuildFailedError{Cause: errors.New("go build: exit 1")})},
			want:  "The container build failed. Check the build log for the error.",
		},
		{
			name:        "missing spec keeps the commit",
			passthrough: true,
			cause: githubbuild.PermanentError{Err: githubbuild.SpecError{
				Reason: "No astropods.yml found at commit abc1234.",
				Err:    errors.New("astropods.yml not found in repo at commit abc1234"),
			}},
			want: "No astropods.yml found at commit abc1234.",
		},
		{
			name:        "syntax error keeps the line number",
			passthrough: true,
			cause: githubbuild.PermanentError{Err: githubbuild.SpecError{
				Reason: "astropods.yml has a syntax error on line 4: mapping values are not allowed in this context.",
			}},
			want: "astropods.yml has a syntax error on line 4: mapping values are not allowed in this context.",
		},
		{
			name:  "permanent but not a spec problem",
			cause: githubbuild.PermanentError{Err: errors.New("some other permanent failure")},
			want:  "The build stopped on a problem Astro can't retry. Check the build log.",
		},
		{
			// A SpecError built without a Reason must not send a blank explanation.
			name:  "spec problem with no reader-facing reason",
			cause: githubbuild.PermanentError{Err: githubbuild.SpecError{Err: errors.New("spec validation failed: agent: must specify either image or build")}},
			want:  "The build stopped on a problem Astro can't retry. Check the build log.",
		},
		{
			name:  "bare spec error with no reason and no cause",
			cause: githubbuild.SpecError{},
			want:  "The build stopped on a problem Astro can't retry. Check the build log.",
		},
		{
			name:  "infrastructure failure",
			cause: fmt.Errorf("ensure ECR repo: %w", errors.New("timeout")),
			want:  "The build didn't finish after several tries. Check the build log or try pushing again.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildFailureReason(tc.cause)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
			// An empty cause string is not a leak: strings.Contains matches it in
			// anything, so only a cause with wording of its own is worth checking.
			if tc.cause != nil && !tc.passthrough && tc.cause.Error() != "" && strings.Contains(got, tc.cause.Error()) {
				t.Fatalf("reason leaks the raw error: %q", got)
			}
		})
	}
}

// A spec failure keeps two phrasings: the reader's in Reason, the engineer's in
// Error(), so the build record and the log are unaffected by the copy.
func TestSpecErrorKeepsEngineerPhrasing(t *testing.T) {
	err := githubbuild.SpecError{
		Reason: "No astropods.yml found at commit abc1234.",
		Err:    errors.New("astropods.yml not found in repo at commit abc1234"),
	}
	if err.Error() != "astropods.yml not found in repo at commit abc1234" {
		t.Fatalf("Error() should return the engineer phrasing, got %q", err.Error())
	}
	bare := githubbuild.SpecError{Reason: "astropods.yml is invalid: components.api.image is required."}
	if bare.Error() != bare.Reason {
		t.Fatalf("Error() should fall back to Reason when Err is nil, got %q", bare.Error())
	}
}
