package tui

import (
	"bufio"
	"context"
	"strings"
	"testing"

	"github.com/gosom/google-maps-scraper/runner"
)

func TestKeywordListDefaultMode(t *testing.T) {
	t.Parallel()
	s := newSettings(&runner.Config{LangCode: "ka", MaxDepth: 2, Concurrency: 2})
	if !s.listMode {
		t.Fatal("expected keyword list to be the default input mode")
	}
	s.toggle(fieldSource)
	if s.listMode {
		t.Fatal("expected input source switch to single query")
	}
	s.toggle(fieldSource)
	if !s.listMode {
		t.Fatal("expected input source switch back to keyword list")
	}
}

func TestSplitKeywordsPreservesGeorgianAndDeduplicates(t *testing.T) {
	t.Parallel()
	raw := "კოსმეტოლოგი თბილისი\r\n\nდერმატოლოგი თბილისი\r\nკოსმეტოლოგი თბილისი\n  Laser clinics Tbilisi  "
	queries := splitKeywords(raw)
	expected := []string{"კოსმეტოლოგი თბილისი", "დერმატოლოგი თბილისი", "Laser clinics Tbilisi"}
	if len(queries) != len(expected) {
		t.Fatalf("got %v, expected %v", queries, expected)
	}
	for i, want := range expected {
		if queries[i] != want {
			t.Errorf("query[%d]=%q, want %q", i, queries[i], want)
		}
	}
}

func TestKeywordPastePreservesLineBreaks(t *testing.T) {
	t.Parallel()
	paste := "კოსმეტოლოგი თბილისი\r\nდერმატოლოგი თბილისი\t\nлазерное лечение Тбилиси"
	got, err := resolveKeywordPaste(context.Background(), keyEvent{action: "paste", text: paste})
	if err != nil {
		t.Fatal(err)
	}
	want := "კოსმეტოლოგი თბილისი\nდერმატოლოგი თბილისი \nлазерное лечение Тбилиси"
	if got != want {
		t.Fatalf("paste got %q, want %q", got, want)
	}
	s := settings{listMode: true, keywords: got}
	if len(s.queries()) != 3 {
		t.Fatalf("expected 3 independent searches, got %v", s.queries())
	}
	if s.queryText() != got {
		t.Fatalf("query file data changed: %q", s.queryText())
	}
}

func TestKeywordListValidation(t *testing.T) {
	t.Parallel()
	s := settings{listMode: true, language: "ka", depth: "2", workers: "2", output: "test.csv"}
	if err := validateQueryList(s); err == nil {
		t.Fatal("empty list should fail")
	}
	s.keywords = "\n   \nA\n\nB\nA\n"
	if err := validateQueryList(s); err != nil {
		t.Fatal(err)
	}
	if got := s.queryText(); got != "A\nB" {
		t.Fatalf("expected one query per line, got %q", got)
	}
}

func TestSingleQueryModeIsBackwardCompatible(t *testing.T) {
	t.Parallel()
	s := settings{listMode: false, query: "beauty clinics Tbilisi", keywords: "unused\nlist", language: "en", depth: "2", workers: "2", output: "test.csv"}
	if got := s.queryText(); got != s.query {
		t.Fatalf("single-query output = %q", got)
	}
	if err := validateQueryList(s); err != nil {
		t.Fatal(err)
	}
}

func TestKeywordEditorSaveAndCancelKeyEvents(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ key rune; want string }{
		{19, "save"}, {7, "cancel"},
	} {
		reader := bufio.NewReader(strings.NewReader(string(test.key)))
		event, err := nextKey(reader)
		if err != nil || event.action != test.want {
			t.Fatalf("key=%d, event=%+v, err=%v", test.key, event, err)
		}
	}
}

func TestKeywordEditorMultilineSave(t *testing.T) {
	t.Parallel()
	keys := make(chan keyEvent, 4)
	keys <- keyEvent{action: "paste", text: "косметолог თბილისი\nდერმატოლოგი თბილისი"}
	keys <- keyEvent{action: "save"}
	close(keys)
	got, saved, err := editKeywords(context.Background(), keys, "")
	if err != nil || !saved {
		t.Fatalf("editor save failed: got=%q saved=%v err=%v", got, saved, err)
	}
	if got != "косметолог თბილისი\nდერმატოლოგი თბილისი" {
		t.Fatalf("editor mixed-language paste changed: %q", got)
	}
}
