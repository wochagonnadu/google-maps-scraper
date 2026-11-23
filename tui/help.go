package tui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

type fieldGuide struct {
	hint   string
	detail string
}

var fieldGuides = [...]fieldGuide{
	{"e.g. კოსმეტოლოგი თბილისი", "Type a business category and place, e.g. კოსმეტოლოგი თბილისი"},
	{"Browser: details / Fast: quick", "Browser collects richer details; Fast requires coordinates and skips emails"},
	{"Tbilisi: 41.7151,44.8271", "Use latitude,longitude. Optional in Browser mode, required in Fast mode"},
	{"ka / en / ru", "Two-letter result language: ka=Georgian, en=English, ru=Russian"},
	{"Try 2 for a quick test", "Depth controls map scrolls or HTTP batches. Start with 2 for testing"},
	{"Start with 2", "Number of parallel workers. More workers use more resources"},
	{"Browser mode only", "Visit business websites to extract email addresses (slower)"},
	{"CSV = spreadsheet / JSONL = data", "CSV opens in Excel/Sheets; JSONL saves one JSON object per line"},
	{"e.g. tbilisi_test.csv", "Save to a new .csv or .jsonl file; existing files are never overwritten"},
	{"Enter to run", "Validate fields and start collecting Google Maps listings"},
	{"Enter to exit", "Return to your terminal without starting a scrape"},
}

func displayCell(input string, width int, tail bool) string {
	if width < 1 {
		return ""
	}
	if runewidth.StringWidth(input) <= width {
		return input
	}
	if width == 1 {
		return "…"
	}
	if tail {
		runes := []rune(input)
		for len(runes) > 0 && runewidth.StringWidth(string(runes)) > width-1 {
			runes = runes[1:]
		}
		return "…" + string(runes)
	}
	var b strings.Builder
	for _, r := range input {
		if runewidth.StringWidth(b.String())+runewidth.RuneWidth(r) > width-1 {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "…"
}

// renderForm adapts to terminal width. Wide windows get per-field help on the
// right; narrow windows show the selected field's explanation below the form.
func renderForm(s settings, selected int, editing bool, editValue, message string, columns int) string {
	const (
		minSidebarWidth = 94
		labelWidth      = 17
	)
	sidebar := columns >= minSidebarWidth
	valueWidth := 32
	if !sidebar {
		valueWidth = columns - 26
		if valueWidth < 12 {
			valueWidth = 12
		}
		if valueWidth > 50 {
			valueWidth = 50
		}
	}
	leftWidth := 2 + labelWidth + 2 + valueWidth
	hintWidth := columns - leftWidth - 7

	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	b.WriteString(accent + bold + "  ◆ MAPS SCOUT" + reset + dim + "  /  Interactive terminal" + reset + "\r\n")
	b.WriteString(dim + "  " + strings.Repeat("─", max(16, min(columns-4, 67))) + reset + "\r\n\r\n")
	header := "  Configure your search"
	if sidebar {
		header += strings.Repeat(" ", max(2, leftWidth-runewidth.StringWidth(header))) + dim + "│  FIELD GUIDE" + reset
	}
	b.WriteString(header + "\r\n")
	b.WriteString(dim + "  ↑↓ / Tab: select   Enter: edit   Space: toggle   Ctrl+V / Cmd+V: paste" + reset + "\r\n\r\n")

	for i := 0; i < fieldCount; i++ {
		prefix := "  "
		if selected == i {
			prefix = accent + "› " + reset
		}
		content := s.display(i)
		if editing && i == selected {
			content = displayCell(editValue, valueWidth-1, true) + "▌"
		} else if content == "" {
			content = "(empty)"
		}
		content = displayCell(content, valueWidth, false)
		var row strings.Builder
		row.WriteString(prefix)
		if selected == i {
			row.WriteString(bold)
		} else {
			row.WriteString(dim)
		}
		row.WriteString(fmt.Sprintf("%-*s", labelWidth, fieldLabels[i]))
		row.WriteString(reset + "  " + content)
		if sidebar {
			row.WriteString(strings.Repeat(" ", max(0, valueWidth-runewidth.StringWidth(content))))
			if i == selected {
				row.WriteString("  │  " + accent + displayCell(fieldGuides[i].hint, hintWidth, false) + reset)
			} else {
				row.WriteString("  │  " + dim + displayCell(fieldGuides[i].hint, hintWidth, false) + reset)
			}
		}
		b.WriteString(row.String() + "\r\n")
	}
	b.WriteString("\r\n")
	if !sidebar {
		b.WriteString(dim + "  Tip: " + displayCell(fieldGuides[selected].detail, max(20, columns-10), false) + reset + "\r\n")
	}
	if message != "" {
		b.WriteString(red + "  ! " + displayCell(message, max(20, columns-8), false) + reset + "\r\n")
	} else if editing {
		b.WriteString(dim + "  Editing · Enter: save · Ctrl+U: clear · Ctrl+V / Cmd+V: paste" + reset + "\r\n")
	} else {
		b.WriteString(dim + "  Enter: edit/start · q: quit · Ctrl+C: exit" + reset + "\r\n")
	}
	return b.String()
}
