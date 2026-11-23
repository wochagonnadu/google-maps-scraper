# Interactive terminal mode (experimental)

The optional TUI lets you configure and run a local scrape without manually
writing an input file or remembering CLI flags.

## Start

Install a compatible native scraper binary (including the Playwright browser
runtime), or build from source as described in the repository README.

```sh
./google-maps-scraper -tui
```

The existing CLI and Web UI remain unchanged when `-tui` is not used.

## Controls

- **Up / Down** or **Tab**: move between fields.
- **Enter**: edit a text field, save it, or start the scrape.
- **Space**: switch between Browser/Fast, enable email extraction, or select CSV/JSONL.
- **Ctrl+V**: paste from the system clipboard (macOS uses `pbpaste`; Linux uses Wayland/X11 clipboard tools; Windows uses PowerShell).
- **Cmd+V** on macOS terminal emulators: standard terminal paste, including UTF-8/Georgian text (bracketed-paste mode).
- **Ctrl+U**: clear a text field while editing.
- **q / Ctrl+C**: exit the form or stop a running scrape.
- **Enter** after completion: return to the search form.

Text input supports Unicode queries, including Georgian and Cyrillic. Paste works whether a text field is already in edit mode or just selected. In edit mode, pasting appends to the existing value; when only selected, it replaces the field's value and opens edit mode. Clipboard content with line breaks is normalized to a single line to prevent accidental form submissions. Large pastes over 32 KiB are rejected.

A help column beside every setting explains what to enter and provides examples. In narrower terminal windows, help for the currently selected field appears underneath the form. For a quick test, use `კოსმეტოლოგი თბილისი`, `ka`, depth `2`, workers `2`, and `tbilisi_test.csv`.

## Modes and output

- **Browser mode** supports optional coordinates and email extraction.
- **Fast mode** requires latitude,longitude (for example `41.7151,44.8271`).
  Fast mode cannot extract website emails.
- Choose **CSV** or **JSONL** and an output filename with a matching extension.
- The TUI **refuses to overwrite an existing results file**. Choose a new
  filename for each run.
- Results are written in the working directory unless an output path is given.
  Interrupted jobs may leave partial results in that file.

The dashboard displays collected records, elapsed time, and recent log
messages. Structured JSON logs are summarized as readable level/component/message
entries; long log entries wrap instead of being truncated. PostHog analytics
transport failures are labeled as telemetry issues, not scraping failures. Since Google Maps doesn't provide a known total number of matches,
the TUI deliberately does **not** invent a percentage-complete indicator.
Record counts update when the scraper flushes data to disk.

## Implementation scope

This is the first, dependency-light terminal UI: it uses the existing
`golang.org/x/term` dependency for raw keyboard input and starts the same
scraper binary with established CLI flags. It does **not** change the scraping
engine or replace the Web UI.

An event-driven UI framework, history, richer cancellation, and XLSX export
can be reviewed in follow-up contributions. This first implementation is
intended to be small enough to validate upstream before expanding it.

## Notes

- Requires an interactive terminal, not redirected stdin/stdout.
- `Ctrl+V` clipboard access requires a supported system clipboard command. If none is available, use your terminal's native paste shortcut instead; an informative error is shown.
- If launched from a source build, `go run . -tui` can be used; the TUI
  locates and starts the same compiled executable for the actual scrape.
- If you stop a scrape, use a new output filename for a subsequent run.
