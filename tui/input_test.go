package tui

import (
	"bufio"
	"context"
	"strings"
	"testing"
)

func TestCtrlVClipboardEvent(t *testing.T) {
	t.Parallel()
	reader := bufio.NewReader(strings.NewReader("\x16"))
	evt, err := nextKey(reader)
	if err != nil || evt.action != "pasteClipboard" {
		t.Fatalf("Ctrl+V: evt=%+v, err=%v", evt, err)
	}
}

func TestBracketedPastePreservesGeorgian(t *testing.T) {
	t.Parallel()
	reader := bufio.NewReader(strings.NewReader("\x1b[200~კოსმეტოლოგი თბილისი\x1b[201~"))
	evt, err := nextKey(reader)
	if err != nil || evt.action != "paste" || evt.text != "კოსმეტოლოგი თბილისი" {
		t.Fatalf("Georgian bracketed paste: evt=%+v, err=%v", evt, err)
	}
	text, err := resolvePaste(context.Background(), evt)
	if err != nil || text != "კოსმეტოლოგი თბილისი" {
		t.Fatalf("sanitized input=%q, err=%v", text, err)
	}
}

func TestBracketedPasteNewlinesAreNotEnter(t *testing.T) {
	t.Parallel()
	keys := make(chan keyEvent, 10)
	readKeyEvents(strings.NewReader("\x1b[200~line one\nline two\tქართული\x1b[201~\r"), keys)
	var events []keyEvent
	for evt := range keys {
		events = append(events, evt)
	}
	if len(events) != 2 || events[0].action != "paste" || events[1].action != "enter" {
		t.Fatalf("paste must be single event followed by Enter, got %+v", events)
	}
	normalized := sanitizePaste(events[0].text)
	if normalized != "line one line two ქართული" {
		t.Fatalf("unexpected normalized paste: %q", normalized)
	}
}

func TestCtrlVNotInterpretedAsLetter(t *testing.T) {
	t.Parallel()
	reader := bufio.NewReader(strings.NewReader("\x16q"))
	first, err := nextKey(reader)
	if err != nil || first.action != "pasteClipboard" {
		t.Fatalf("first event=%+v, err=%v", first, err)
	}
	second, err := nextKey(reader)
	if err != nil || second.action != "char" || second.char != 'q' {
		t.Fatalf("second event=%+v, err=%v", second, err)
	}
}

func TestPasteSizeLimit(t *testing.T) {
	t.Parallel()
	reader := bufio.NewReader(strings.NewReader("\x1b[200~" + strings.Repeat("x", maxPasteBytes+1) + "\x1b[201~"))
	evt, err := nextKey(reader)
	if err != nil || evt.action != "pasteTooLong" {
		t.Fatalf("expected length error, got evt=%+v, err=%v", evt, err)
	}
}

func TestFormHelpResponsive(t *testing.T) {
	t.Parallel()
	s := settings{query: "კოსმეტოლოგი თბილისი", language: "ka",
		depth: "2", workers: "2", output: "test.csv"}
	wide := renderForm(s, fieldQuery, false, "", "", 110)
	if !strings.Contains(wide, "FIELD GUIDE") || !strings.Contains(wide, "41.7151,44.8271") {
		t.Fatalf("wide layout must show per-field help sidebar")
	}
	if !strings.Contains(wide, s.query) {
		t.Fatalf("Georgian query must render unmodified")
	}
	narrow := renderForm(s, fieldQuery, false, "", "", 70)
	if strings.Contains(narrow, "FIELD GUIDE") || !strings.Contains(narrow, "Tip:") {
		t.Fatalf("narrow layout must show contextual help below form")
	}
}
