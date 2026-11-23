package tui

import (
	"strings"
	"testing"
)

func TestFormatActivityStructured(t *testing.T) {
	t.Parallel()
	line := `{"level":"error","component":"scrapemate","error":"context canceled","time":"2026-11-23T18:10:44Z"}`
	got := formatActivity(line)
	want := "ERROR · scrapemate: context canceled"
	if got != want {
		t.Fatalf("formatted log = %q, want %q", got, want)
	}
}

func TestFormatActivityUsesMessageAndError(t *testing.T) {
	t.Parallel()
	line := `{"level":"info","component":"scrapemate","message":"Visiting maps","error":"timeout"}`
	got := formatActivity(line)
	want := "INFO · scrapemate: Visiting maps — timeout"
	if got != want {
		t.Fatalf("formatted log = %q, want %q", got, want)
	}
}

func TestFormatActivityTelemetryIsDistinct(t *testing.T) {
	t.Parallel()
	raw := `posthog 2026/11/23 21:11:19 ERROR: sending request - Post "https://eu.i.posthog.com/"`
	got := formatActivity(raw)
	if !strings.Contains(got, "Telemetry") || !strings.Contains(got, "PostHog") {
		t.Fatalf("analytics transport should be labeled separately: %q", got)
	}
}

func TestFormatActivityLeavesUnknownLinesUnmodified(t *testing.T) {
	t.Parallel()
	raw := "scraper started: 2 workers"
	if got := formatActivity(raw); got != raw {
		t.Fatalf("plain log changed: %q", got)
	}
}

func TestActivityWrapPreservesFullText(t *testing.T) {
	t.Parallel()
	raw := "INFO · scrapemate: კოსმეტოლოგი თბილისი — collecting results"
	parts := wrapActivity(raw, 23)
	if len(parts) < 2 {
		t.Fatalf("expected wrapped activity, got %v", parts)
	}
	if got := strings.Join(parts, ""); got != raw {
		t.Fatalf("wrapped log changed content: %q", got)
	}
	for _, line := range parts {
		if len([]rune(line)) > 23 {
			t.Fatalf("line too long: %q", line)
		}
	}
}
