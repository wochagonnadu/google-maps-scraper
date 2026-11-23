package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
)

const maxKeywords = 1000

// splitKeywords transforms newline-separated search terms into unique jobs.
// Blank lines are ignored, and the original order is preserved.
func splitKeywords(value string) []string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	seen := make(map[string]bool)
	var queries []string
	for _, raw := range strings.Split(normalized, "\n") {
		keyword := strings.TrimSpace(raw)
		if keyword == "" {
			continue
		}
		key := strings.ToLower(keyword)
		if seen[key] {
			continue
		}
		seen[key] = true
		queries = append(queries, keyword)
	}
	return queries
}

// sanitizeKeywordPaste preserves line breaks but removes terminal controls.
// Bracketed paste is a single input event, so pasted lines never submit a form.
func sanitizeKeywordPaste(raw string) string {
	raw = strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	var out strings.Builder
	for _, r := range raw {
		switch {
		case r == '\n':
			out.WriteByte('\n')
		case r == '\t':
			out.WriteByte(' ')
		case unicode.IsControl(r):
			continue
		case unicode.IsPrint(r):
			out.WriteRune(r)
		}
	}
	return out.String()
}

func resolveKeywordPaste(ctx context.Context, event keyEvent) (string, error) {
	switch event.action {
	case "paste":
		return sanitizeKeywordPaste(event.text), nil
	case "pasteClipboard":
		raw, err := readSystemClipboardText(ctx)
		if err != nil {
			return "", err
		}
		return sanitizeKeywordPaste(raw), nil
	}
	return "", nil
}

func (s settings) queries() []string {
	if s.listMode {
		return splitKeywords(s.keywords)
	}
	if query := strings.TrimSpace(s.query); query != "" {
		return []string{query}
	}
	return nil
}

// queryText is written to the native CLI's -input file, one query per line.
func (s settings) queryText() string {
	return strings.Join(s.queries(), "\n")
}

// editKeywords is a small multiline editor. Enter adds a new line; Ctrl+S
// confirms the entire list, and Ctrl+G cancels without discarding the old list.
func editKeywords(ctx context.Context, keys <-chan keyEvent, initial string) (string, bool, error) {
	value := initial
	message := ""
	for {
		drawKeywordEditor(value, message)
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case evt, ok := <-keys:
			if !ok {
				return "", false, errExit
			}
			switch evt.action {
			case "interrupt":
				return "", false, errExit
			case "cancel", "escape":
				return initial, false, nil
			case "save":
				queries := splitKeywords(value)
				if len(queries) == 0 {
					message = "Enter at least one keyword"
					continue
				}
				if len(queries) > maxKeywords {
					message = fmt.Sprintf("Too many keywords: maximum %d", maxKeywords)
					continue
				}
				return strings.Join(queries, "\n"), true, nil
			case "enter":
				value += "\n"
				message = ""
			case "backspace":
				if len(value) > 0 {
					_, size := utf8.DecodeLastRuneInString(value)
					value = value[:len(value)-size]
				}
				message = ""
			case "clear":
				value = ""
				message = ""
			case "paste", "pasteClipboard":
				pasted, err := resolveKeywordPaste(ctx, evt)
				if err != nil {
					message = err.Error()
				} else {
					value += pasted
					message = ""
				}
			case "pasteTooLong":
				message = "Pasted list is too large (max 32 KiB)"
			case "char":
				value += string(evt.char)
				message = ""
			}
		}
	}
}

func drawKeywordEditor(value, message string) {
	columns, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || columns < 40 {
		columns = 80
	}
	if err != nil || rows < 15 {
		rows = 24
	}
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	b.WriteString(accent + bold + "  ◆ MAPS SCOUT  /  KEYWORD LIST" + reset + "\r\n")
	b.WriteString(dim + "  One keyword per line. Enter adds a line; paste a full list with Ctrl+V or Cmd+V." + reset + "\r\n")
	b.WriteString(dim + "  Ctrl+S save  ·  Ctrl+G cancel  ·  Ctrl+U clear all" + reset + "\r\n\r\n")

	lines := strings.Split(value, "\n")
	visible := rows - 10
	if visible < 4 {
		visible = 4
	}
	start := len(lines) - visible
	if start < 0 {
		start = 0
	}
	for i := start; i < len(lines); i++ {
		prefix := fmt.Sprintf("  %3d │ ", i+1)
		maxWidth := columns - 11
		if maxWidth < 12 {
			maxWidth = 12
		}
		line := displayCell(lines[i], maxWidth, i == len(lines)-1)
		if i == len(lines)-1 {
			b.WriteString(accent + prefix + reset + line + "▌\r\n")
		} else {
			b.WriteString(dim + prefix + reset + line + "\r\n")
		}
	}
	b.WriteString("\r\n")
	b.WriteString(dim + fmt.Sprintf("  Valid unique keywords: %d", len(splitKeywords(value))) + reset + "\r\n")
	if message != "" {
		b.WriteString(red + "  ! " + displayCell(message, columns-8, false) + reset + "\r\n")
	}
	fmt.Fprint(os.Stdout, b.String())
}

// keywordListSummary provides a short summary in the configuration form.
func keywordListSummary(s settings) string {
	queries := s.queries()
	if len(queries) == 0 {
		return "(empty)  Enter to edit / Ctrl+V to paste"
	}
	if len(queries) == 1 {
		return "1 keyword: " + queries[0]
	}
	return fmt.Sprintf("%d keywords: %s …", len(queries), queries[0])
}

// ensure the input mode never writes an empty list to the scraper.
func validateQueryList(s settings) error {
	queries := s.queries()
	if len(queries) == 0 {
		return errors.New("enter at least one keyword")
	}
	if len(queries) > maxKeywords {
		return fmt.Errorf("too many keywords (maximum %d)", maxKeywords)
	}
	return nil
}
