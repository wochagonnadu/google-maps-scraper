package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"
)

const (
	maxPasteBytes = 32 * 1024
	pasteEnd      = "\x1b[201~"
)

// readKeys converts terminal keystrokes and bracketed paste into distinct events.
// In particular, pasted text never masquerades as Enter, Tab, or navigation keys.
func readKeys(keys chan<- keyEvent) {
	readKeyEvents(os.Stdin, keys)
}

func readKeyEvents(source io.Reader, keys chan<- keyEvent) {
	defer close(keys)
	reader := bufio.NewReader(source)
	for {
		evt, err := nextKey(reader)
		if err != nil {
			return
		}
		if evt.action != "" {
			keys <- evt
		}
	}
}

func nextKey(reader *bufio.Reader) (keyEvent, error) {
	r, _, err := reader.ReadRune()
	if err != nil {
		return keyEvent{}, err
	}
	switch r {
	case 3:
		return keyEvent{action: "interrupt"}, nil
	case 13, 10:
		return keyEvent{action: "enter"}, nil
	case 9:
		return keyEvent{action: "tab"}, nil
	case 127, 8:
		return keyEvent{action: "backspace"}, nil
	case 21:
		return keyEvent{action: "clear"}, nil
	case 22:
		return keyEvent{action: "pasteClipboard"}, nil
	case 27:
		return nextEscape(reader)
	default:
		if unicode.IsPrint(r) {
			return keyEvent{action: "char", char: r}, nil
		}
	}
	return keyEvent{}, nil
}

func nextEscape(reader *bufio.Reader) (keyEvent, error) {
	first, err := reader.ReadByte()
	if err != nil {
		return keyEvent{action: "escape"}, err
	}
	if first != '[' {
		return keyEvent{action: "escape"}, nil
	}

	// ANSI CSI sequences terminate with a byte in the range 0x40..0x7e.
	// This covers arrows and \x1b[200~ / \x1b[201~ bracketed paste.
	var sequence []byte
	for len(sequence) < 24 {
		b, err := reader.ReadByte()
		if err != nil {
			return keyEvent{}, err
		}
		sequence = append(sequence, b)
		if b >= 0x40 && b <= 0x7e {
			break
		}
	}
	switch string(sequence) {
	case "A":
		return keyEvent{action: "up"}, nil
	case "B":
		return keyEvent{action: "down"}, nil
	case "C":
		return keyEvent{action: "right"}, nil
	case "D":
		return keyEvent{action: "left"}, nil
	case "200~":
		content, truncated, err := readBracketedPaste(reader)
		if err != nil {
			return keyEvent{}, err
		}
		if truncated {
			return keyEvent{action: "pasteTooLong"}, nil
		}
		return keyEvent{action: "paste", text: content}, nil
	}
	return keyEvent{}, nil
}

// readBracketedPaste stops only at the terminal's explicit paste terminator.
// Other embedded keystroke-like text remains literal data.
func readBracketedPaste(reader *bufio.Reader) (string, bool, error) {
	var output strings.Builder
	marker := []byte(pasteEnd)
	matched := 0
	truncated := false
	add := func(b byte) {
		if output.Len() < maxPasteBytes {
			output.WriteByte(b)
		} else {
			truncated = true
		}
	}
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return "", false, err
		}
		if b == marker[matched] {
			matched++
			if matched == len(marker) {
				return output.String(), truncated, nil
			}
			continue
		}
		for _, pending := range marker[:matched] {
			add(pending)
		}
		matched = 0
		if b == marker[0] {
			matched = 1
		} else {
			add(b)
		}
	}
}

// sanitizePaste makes multiline clipboard content safe for single-line inputs.
func sanitizePaste(raw string) string {
	var out strings.Builder
	for _, r := range raw {
		switch {
		case unicode.IsSpace(r):
			out.WriteByte(' ')
		case unicode.IsControl(r):
			continue
		case unicode.IsPrint(r):
			out.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

// readSystemClipboard supports the explicit Ctrl+V shortcut. Command+V in
// standard macOS terminal emulators is handled by bracketed-paste input.
func readSystemClipboard(ctx context.Context) (string, error) {
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"pbpaste", "-Prefer", "txt"}}
	case "windows":
		candidates = [][]string{{"powershell.exe", "-NoProfile", "-Command", "[Console]::OutputEncoding=[Text.UTF8Encoding]::new(); Get-Clipboard -Raw"}}
	default:
		candidates = [][]string{
			{"wl-paste", "--no-newline"},
			{"xclip", "-selection", "clipboard", "-o"},
			{"xsel", "--clipboard", "--output"},
		}
	}
	var lastErr error
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate[0]); err != nil {
			lastErr = err
			continue
		}
		pasteCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		output, err := exec.CommandContext(pasteCtx, candidate[0], candidate[1:]...).Output()
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if len(output) > maxPasteBytes {
			return "", errors.New("clipboard text is too long (limit: 32 KiB)")
		}
		return sanitizePaste(string(output)), nil
	}
	if lastErr == nil {
		lastErr = errors.New("no clipboard utility available")
	}
	return "", fmt.Errorf("cannot read clipboard: %w (try terminal paste shortcut)", lastErr)
}

// resolvePaste maps terminal bracketed-paste payloads and Ctrl+V clipboard reads
// to a single sanitized value for text fields.
func resolvePaste(ctx context.Context, event keyEvent) (string, error) {
	switch event.action {
	case "paste":
		return sanitizePaste(event.text), nil
	case "pasteClipboard":
		return readSystemClipboard(ctx)
	default:
		return "", nil
	}
}
