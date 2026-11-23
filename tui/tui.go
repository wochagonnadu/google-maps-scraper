// Package tui provides an optional interactive terminal interface for CLI scraping.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gosom/google-maps-scraper/runner"
	"golang.org/x/term"
)

const (
	reset  = "\x1b[0m"
	accent = "\x1b[38;5;43m"
	dim    = "\x1b[38;5;245m"
	bold   = "\x1b[1m"
	red    = "\x1b[38;5;203m"
	green  = "\x1b[38;5;114m"
)

const (
	fieldQuery = iota
	fieldSource
	fieldMode
	fieldGeo
	fieldLanguage
	fieldDepth
	fieldWorkers
	fieldEmail
	fieldExport
	fieldPath
	fieldStart
	fieldQuit
	fieldCount
)

var fieldLabels = [...]string{
	"Search query", "Input type", "Mode", "Coordinates", "Language",
	"Depth", "Workers", "Extract emails", "Export",
	"Output file", "Start scraping", "Quit",
}

// Runner implements runner.Runner without changing existing CLI behavior.
type Runner struct {
	cfg *runner.Config
}

// New creates an interactive terminal runner.
func New(cfg *runner.Config) runner.Runner {
	return &Runner{cfg: cfg}
}

// Close implements runner.Runner.
func (r *Runner) Close(context.Context) error { return nil }

type settings struct {
	query    string
	keywords string
	listMode bool
	fast     bool
	geo      string
	language string
	depth    string
	workers  string
	email    bool
	jsonl    bool
	output   string
}

func newSettings(cfg *runner.Config) settings {
	depth := cfg.MaxDepth
	if depth < 1 {
		depth = 3
	}
	workers := cfg.Concurrency
	if workers < 1 {
		workers = 1
	}
	lang := cfg.LangCode
	if lang == "" {
		lang = "en"
	}
	return settings{
		query:    "",
		listMode: true,
		fast:     cfg.FastMode,
		geo:      cfg.GeoCoordinates,
		language: lang,
		depth:    strconv.Itoa(depth),
		workers:  strconv.Itoa(workers),
		email:    cfg.Email,
		jsonl:    cfg.JSON,
		output:   defaultOutput(cfg.JSON),
	}
}

func defaultOutput(jsonl bool) string {
	if jsonl {
		return "results.jsonl"
	}
	return "results.csv"
}

func (s *settings) display(i int) string {
	switch i {
	case fieldQuery:
		if s.listMode {
			return keywordListSummary(*s)
		}
		return s.query
	case fieldSource:
		if s.listMode {
			return "Keyword list  [Space to switch]"
		}
		return "Single query  [Space to switch]"
	case fieldMode:
		if s.fast {
			return "Fast (HTTP)  [Space to switch]"
		}
		return "Browser (full details)  [Space to switch]"
	case fieldGeo:
		if s.geo == "" {
			return "(optional for Browser; required for Fast)"
		}
		return s.geo
	case fieldLanguage:
		return s.language
	case fieldDepth:
		return s.depth
	case fieldWorkers:
		return s.workers
	case fieldEmail:
		if s.fast {
			return "Unavailable in Fast mode"
		}
		if s.email {
			return "Yes  [Space to switch]"
		}
		return "No  [Space to switch]"
	case fieldExport:
		if s.jsonl {
			return "JSONL  [Space to switch]"
		}
		return "CSV  [Space to switch]"
	case fieldPath:
		return s.output
	case fieldStart:
		return "Press Enter to run"
	case fieldQuit:
		return "Press Enter to exit"
	}
	return ""
}

func (s *settings) setText(i int, v string) {
	v = strings.TrimSpace(v)
	switch i {
	case fieldQuery:
		s.query = v
	case fieldGeo:
		s.geo = v
	case fieldLanguage:
		s.language = v
	case fieldDepth:
		s.depth = v
	case fieldWorkers:
		s.workers = v
	case fieldPath:
		s.output = v
	}
}

func (s *settings) toggle(i int) {
	switch i {
	case fieldSource:
		s.listMode = !s.listMode
	case fieldMode:
		s.fast = !s.fast
		if s.fast {
			s.email = false
		}
	case fieldEmail:
		if !s.fast {
			s.email = !s.email
		}
	case fieldExport:
		if s.jsonl && s.output == "results.jsonl" {
			s.output = "results.csv"
		} else if !s.jsonl && s.output == "results.csv" {
			s.output = "results.jsonl"
		}
		s.jsonl = !s.jsonl
	}
}

func isTextField(i int) bool {
	switch i {
	case fieldQuery, fieldGeo, fieldLanguage, fieldDepth, fieldWorkers, fieldPath:
		return true
	}
	return false
}

func (s settings) validate() error {
	if s.listMode {
		if err := validateQueryList(s); err != nil {
			return err
		}
	} else {
		if strings.TrimSpace(s.query) == "" {
			return errors.New("enter a search query")
		}
		if strings.ContainsAny(s.query, "\r\n") {
			return errors.New("query must be on one line")
		}
	}
	if s.fast && s.geo == "" {
		return errors.New("Fast mode requires coordinates (latitude,longitude)")
	}
	if s.geo != "" {
		parts := strings.Split(s.geo, ",")
		if len(parts) != 2 {
			return errors.New("coordinates must be latitude,longitude")
		}
		lat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lon, errLon := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errLat != nil || errLon != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return errors.New("invalid latitude or longitude")
		}
	}
	if len(s.language) != 2 {
		return errors.New("language must be a two-letter code, e.g. en, ru, ka")
	}
	if err := positiveInt(s.depth, "depth", 1, 100); err != nil {
		return err
	}
	if err := positiveInt(s.workers, "workers", 1, 128); err != nil {
		return err
	}
	if s.output == "" {
		return errors.New("select an output filename")
	}
	if strings.ContainsAny(s.output, "\r\n") {
		return errors.New("output filename contains invalid characters")
	}
	if ext := strings.ToLower(filepath.Ext(s.output)); (s.jsonl && ext != ".jsonl") || (!s.jsonl && ext != ".csv") {
		return errors.New("output extension must match selected CSV/JSONL format")
	}
	if _, err := os.Stat(s.output); err == nil {
		return errors.New("output file already exists; choose another name to avoid overwriting it")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check output file: %w", err)
	}
	if info, err := os.Stat(filepath.Dir(s.output)); err != nil || !info.IsDir() {
		return errors.New("output directory does not exist")
	}
	return nil
}

func positiveInt(v, name string, minVal, maxVal int) error {
	num, err := strconv.Atoi(v)
	if err != nil || num < minVal || num > maxVal {
		return fmt.Errorf("%s must be between %d and %d", name, minVal, maxVal)
	}
	return nil
}

func (s settings) args(queryFile string) []string {
	args := []string{
		"-input", queryFile,
		"-results", s.output,
		"-lang", s.language,
		"-depth", s.depth,
		"-c", s.workers,
		"-exit-on-inactivity", "3m",
	}
	if s.fast {
		args = append(args, "-fast-mode")
	}
	if s.geo != "" {
		args = append(args, "-geo", s.geo)
	}
	if s.email && !s.fast {
		args = append(args, "-email")
	}
	if s.jsonl {
		args = append(args, "-json")
	}
	return args
}

type keyEvent struct {
	action string
	char   rune
	text   string
}

// Run starts the optional interactive user interface.
func (r *Runner) Run(ctx context.Context) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("TUI requires an interactive terminal (stdin and stdout)")
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("enable terminal input: %w", err)
	}
	defer func() {
		fmt.Fprint(os.Stdout, "\x1b[?2004l\x1b[?25h\x1b[?1049l")
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
	}()
	fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?2004h\x1b[?25l")
	keys := make(chan keyEvent, 32)
	go readKeys(keys)
	s := newSettings(r.cfg)
	for {
		launch, err := editForm(ctx, keys, &s)
		if err != nil {
			if errors.Is(err, errExit) {
				return nil
			}
			return err
		}
		if !launch {
			return nil
		}
		if err := runScrape(ctx, keys, s); err != nil {
			if errors.Is(err, errExit) {
				return nil
			}
			return err
		}
	}
}

var errExit = errors.New("exit TUI")

func editForm(ctx context.Context, keys <-chan keyEvent, s *settings) (bool, error) {
	selected := fieldQuery
	editing := false
	editValue := ""
	message := ""
	for {
		drawForm(*s, selected, editing, editValue, message)
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case evt, ok := <-keys:
			if !ok {
				return false, errExit
			}
			if evt.action == "interrupt" {
				return false, errExit
			}
			if editing {
				switch evt.action {
				case "enter", "tab":
					s.setText(selected, editValue)
					editing = false
					message = ""
					if evt.action == "tab" {
						selected = (selected + 1) % fieldCount
					}
				case "backspace":
					if len(editValue) > 0 {
						_, n := utf8.DecodeLastRuneInString(editValue)
						editValue = editValue[:len(editValue)-n]
					}
				case "clear":
					editValue = ""
				case "paste", "pasteClipboard":
					pasted, err := resolvePaste(ctx, evt)
					if err != nil {
						message = err.Error()
					} else if pasted != "" {
						editValue += pasted
						message = ""
					}
				case "pasteTooLong":
					message = "Pasted text exceeds the 32 KiB limit"
				case "char":
					editValue += string(evt.char)
				}
				continue
			}
			switch evt.action {
			case "up":
				selected = (selected + fieldCount - 1) % fieldCount
				message = ""
			case "down", "tab":
				selected = (selected + 1) % fieldCount
				message = ""
			case "paste", "pasteClipboard":
				if selected == fieldQuery && s.listMode {
					pasted, pasteErr := resolveKeywordPaste(ctx, evt)
					if pasteErr != nil {
						message = pasteErr.Error()
					} else if pasted != "" {
						s.keywords = pasted
						message = fmt.Sprintf("%d keywords pasted", len(s.queries()))
					}
					continue
				}
				if !isTextField(selected) {
					message = "Select a text field before pasting"
					continue
				}
				pasted, err := resolvePaste(ctx, evt)
				if err != nil {
					message = err.Error()
				} else if pasted != "" {
					editing = true
					editValue = pasted
					message = ""
				}
			case "pasteTooLong":
				message = "Pasted text exceeds the 32 KiB limit"
			case "char":
				if evt.char == 'q' {
					return false, errExit
				}
				if evt.char == ' ' {
					s.toggle(selected)
					message = ""
				}
			case "enter", "right", "left":
				switch {
				case selected == fieldQuit:
					return false, errExit
				case selected == fieldStart && evt.action == "enter":
					if err := s.validate(); err != nil {
						message = err.Error()
						continue
					}
					return true, nil
				case selected == fieldQuery && s.listMode && evt.action == "enter":
					value, saved, editErr := editKeywords(ctx, keys, s.keywords)
					if editErr != nil {
						return false, editErr
					}
					if saved {
						s.keywords = value
						message = fmt.Sprintf("Saved %d unique keywords", len(s.queries()))
					}
				case isTextField(selected) && evt.action == "enter":
					editing = true
					editValue = s.display(selected)
					if selected == fieldGeo {
						// Do not turn the placeholder text into saved coordinates.
						editValue = s.geo
					}
					message = ""
				default:
					s.toggle(selected)
					message = ""
				}
			}
		}
	}
}

func drawForm(s settings, selected int, editing bool, value, message string) {
	columns, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || columns < 40 {
		columns = 80
	}
	fmt.Fprint(os.Stdout, renderForm(s, selected, editing, value, message, columns))
}

func truncate(s string, maxRunes int) string {
	runes := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes-1]) + "…"
}

type logTail struct {
	mu   sync.Mutex
	data string
}

func (l *logTail) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data += string(p)
	if len(l.data) > 12000 {
		l.data = l.data[len(l.data)-12000:]
	}
	return len(p), nil
}

func (l *logTail) recent() []string {
	l.mu.Lock()
	data := l.data
	l.mu.Unlock()
	lines := strings.Split(data, "\n")
	var clean []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Suppress the CLI banner; keep actionable worker logs.
		if line == "" || strings.ContainsAny(line, "╔╗╚╝═") || strings.Contains(line, "Consider sponsoring") {
			continue
		}
		clean = append(clean, formatActivity(line))
	}
	if len(clean) > 5 {
		clean = clean[len(clean)-5:]
	}
	return clean
}

func runScrape(ctx context.Context, keys <-chan keyEvent, s settings) error {
	temp, err := os.CreateTemp("", "gmaps-tui-query-*.txt")
	if err != nil {
		return fmt.Errorf("create query file: %w", err)
	}
	defer os.Remove(temp.Name())
	if _, err := fmt.Fprintln(temp, s.queryText()); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate scraper executable: %w", err)
	}
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(childCtx, executable, s.args(temp.Name())...)
	cmd.Stdout = io.Discard
	logs := &logTail{}
	cmd.Stderr = logs
	// SIGINT lets the existing scraper close writers before exiting.
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = 3 * time.Second
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start scraper: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	started := time.Now()
	tracker := newRecordCounter(s.output, s.jsonl)
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	finished := false
	var runErr error
	stopped := false
	for {
		drawProgress(s, started, tracker.Count(), logs.recent(), finished, stopped, runErr)
		select {
		case <-ctx.Done():
			if !finished {
				cancel()
				<-done
			}
			return ctx.Err()
		case err := <-done:
			runErr = err
			finished = true
		case <-ticker.C:
		case evt, ok := <-keys:
			if !ok {
				cancel()
				<-done
				return nil
			}
			if !finished && (evt.action == "interrupt" || (evt.action == "char" && evt.char == 'q')) {
				stopped = true
				cancel()
				runErr = <-done
				finished = true
			} else if finished && (evt.action == "enter" || (evt.action == "char" && evt.char == 'q')) {
				if evt.action == "char" {
					return errExit
				}
				return nil
			}
		}
	}
}

func drawProgress(s settings, started time.Time, records int, logs []string, finished, stopped bool, runErr error) {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	b.WriteString(accent + bold + "  ◆ MAPS SCOUT" + reset + dim + "  /  Scraping" + reset + "\r\n")
	b.WriteString(dim + "  ───────────────────────────────────────────────────────────────" + reset + "\r\n\r\n")
	state := accent + "● RUNNING" + reset
	if stopped {
		state = red + "■ STOPPED" + reset
	} else if finished && runErr != nil {
		state = red + "✕ FAILED" + reset
	} else if finished {
		state = green + "✓ COMPLETED" + reset
	}
	b.WriteString("  Status:       " + state + "\r\n")
	if s.listMode {
		b.WriteString(fmt.Sprintf("  Queries:      %d keywords\r\n", len(s.queries())))
	} else {
		b.WriteString("  Query:        " + truncate(s.query, 70) + "\r\n")
	}
	b.WriteString(fmt.Sprintf("  Results:      %d records\r\n", records))
	b.WriteString("  Elapsed:      " + time.Since(started).Truncate(time.Second).String() + "\r\n")
	b.WriteString("  Export:       " + truncate(s.output, 70) + "\r\n")
	b.WriteString("\r\n" + dim + "  Latest activity" + reset + "\r\n")
	if len(logs) == 0 {
		b.WriteString(dim + "  Waiting for scraper output…" + reset + "\r\n")
	}
	columns, _, sizeErr := term.GetSize(int(os.Stdout.Fd()))
	if sizeErr != nil || columns < 40 {
		columns = 80
	}
	for _, entry := range logs {
		for i, line := range wrapActivity(entry, columns-8) {
			prefix := "    "
			if i == 0 {
				prefix = "  · "
			}
			b.WriteString(dim + prefix + reset + line + "\r\n")
		}
	}
	b.WriteString("\r\n")
	if finished {
		if runErr != nil && !stopped {
			b.WriteString(red + "  Error: " + truncate(runErr.Error(), 75) + reset + "\r\n")
		}
		b.WriteString(dim + "  Enter: new search · q: quit" + reset + "\r\n")
	} else {
		b.WriteString(dim + "  q / Ctrl+C: stop and preserve collected results" + reset + "\r\n")
		b.WriteString(dim + "  Total available results are unknown; no misleading percent shown." + reset + "\r\n")
	}
	fmt.Fprint(os.Stdout, b.String())
}
