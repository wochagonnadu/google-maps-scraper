package tui

import (
	"encoding/json"
	"strings"
	"unicode"
)

// formatActivity converts structured scraper logs to human-readable progress
// messages without dropping errors or confusing analytics with scraping.
func formatActivity(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	if strings.HasPrefix(strings.ToLower(line), "posthog ") {
		// These errors are about analytics transport, not Google Maps requests.
		if strings.Contains(line, "messages dropped") {
			return "Telemetry (PostHog): analytics events were dropped"
		}
		if strings.Contains(line, "sending request") {
			return "Telemetry (PostHog): unable to send analytics"
		}
		return line
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		return line
	}
	if len(fields) == 0 {
		return line
	}

	get := func(key string) string {
		raw, ok := fields[key]
		if !ok {
			return ""
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			return value
		}
		// Avoid losing diagnostic information if a field isn't a string.
		return string(raw)
	}

	level := strings.ToUpper(get("level"))
	component := get("component")
	message := get("message")
	errText := get("error")
	job := get("job")

	if message == "" {
		message = job
	}
	if errText != "" && !strings.Contains(message, errText) {
		if message == "" {
			message = errText
		} else {
			message += " — " + errText
		}
	}
	if message == "" {
		return line
	}

	var out strings.Builder
	if level != "" {
		out.WriteString(level)
		out.WriteString(" · ")
	}
	if component != "" {
		out.WriteString(component)
		out.WriteString(": ")
	}
	out.WriteString(message)
	return out.String()
}

// wrapActivity preserves all diagnostic text instead of truncating log lines
// at 85 characters. It wraps by display column, not byte length.
func wrapActivity(line string, width int) []string {
	if width < 1 {
		return []string{line}
	}
	var result []string
	var part strings.Builder
	used := 0
	for _, r := range line {
		runeWidth := 1
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
			runeWidth = 0
		} else if r > 0x1100 && (r <= 0x115f ||
			r >= 0x2e80 && r <= 0xa4cf ||
			r >= 0xac00 && r <= 0xd7a3 ||
			r >= 0xf900 && r <= 0xfaff ||
			r >= 0xfe10 && r <= 0xfe19 ||
			r >= 0xfe30 && r <= 0xfe6f ||
			r >= 0xff00 && r <= 0xff60 ||
			r >= 0xffe0 && r <= 0xffe6) {
			runeWidth = 2
		}
		if used+runeWidth > width && part.Len() > 0 {
			result = append(result, part.String())
			part.Reset()
			used = 0
		}
		part.WriteRune(r)
		used += runeWidth
	}
	if part.Len() > 0 {
		result = append(result, part.String())
	}
	if len(result) == 0 {
		return []string{""}
	}
	return result
}
