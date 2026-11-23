package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gosom/google-maps-scraper/runner"
)

func TestDefaultSettings(t *testing.T) {
	t.Parallel()
	cfg := &runner.Config{Concurrency: 4, MaxDepth: 3, LangCode: "ka", JSON: true}
	got := newSettings(cfg)
	if got.language != "ka" || got.depth != "3" || got.workers != "4" || got.output != "results.jsonl" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}

func TestArgs(t *testing.T) {
	t.Parallel()
	s := settings{
		query: "dentists in Tbilisi", fast: true, geo: "41.7151,44.8271",
		language: "ka", depth: "3", workers: "4", jsonl: true,
		output: "results.jsonl",
	}
	want := []string{"-input", "/tmp/queries.txt", "-results", "results.jsonl",
		"-lang", "ka", "-depth", "3", "-c", "4",
		"-exit-on-inactivity", "3m", "-fast-mode", "-geo", "41.7151,44.8271", "-json"}
	if got := s.args("/tmp/queries.txt"); !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestValidateProtectsExistingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "results.csv")
	if err := os.WriteFile(path, []byte("name\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := settings{query: "cafes", language: "en", depth: "1", workers: "1", output: path}
	if err := s.validate(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected overwrite protection, got %v", err)
	}
}

func TestValidateFastModeRequiresGeo(t *testing.T) {
	t.Parallel()
	s := settings{query: "cafes", fast: true, language: "en", depth: "1", workers: "1",
		output: filepath.Join(t.TempDir(), "data.csv")}
	if err := s.validate(); err == nil || !strings.Contains(err.Error(), "requires coordinates") {
		t.Fatalf("expected missing geo error, got %v", err)
	}
	s.geo = "41.7151,44.8271"
	if err := s.validate(); err != nil {
		t.Fatalf("valid fast settings rejected: %v", err)
	}
}

func TestRecordCounterCSVMultilineAndAppend(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "results.csv")
	counter := newRecordCounter(path, false)
	if got := counter.Count(); got != 0 {
		t.Fatalf("missing file: got %d", got)
	}
	appendFile(t, path, "name,description\n\"First\",\"Line one\n")
	if got := counter.Count(); got != 0 {
		t.Fatalf("unfinished multiline row: got %d", got)
	}
	appendFile(t, path, "Line two\"\n\"Second\",\"a \"\"quoted\"\" value\"\n")
	if got := counter.Count(); got != 2 {
		t.Fatalf("expected 2 CSV records, got %d", got)
	}
	if got := counter.Count(); got != 2 {
		t.Fatalf("counter recounted unchanged content: %d", got)
	}
}

func TestRecordCounterJSONLAppend(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "results.jsonl")
	counter := newRecordCounter(path, true)
	appendFile(t, path, "{\"name\":\"first\"}\n{\"name\":\"second")
	if got := counter.Count(); got != 1 {
		t.Fatalf("expected 1 completed JSONL row, got %d", got)
	}
	appendFile(t, path, "\"}\n")
	if got := counter.Count(); got != 2 {
		t.Fatalf("expected 2 completed JSONL rows, got %d", got)
	}
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
