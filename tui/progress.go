package tui

import (
	"io"
	"os"
)

// recordCounter counts completed CSV/JSONL records as the scraper appends them.
// It reads only newly written bytes; multiline CSV fields are handled correctly.
// Counts may lag briefly behind buffered writes in the scraper.
type recordCounter struct {
	path      string
	jsonl     bool
	offset    int64
	rows      int
	quoted    bool
	seenHeader bool
}

func newRecordCounter(path string, jsonl bool) *recordCounter {
	return &recordCounter{path: path, jsonl: jsonl}
}

// Count returns records observed so far (not a percentage or an estimate).
func (c *recordCounter) Count() int {
	f, err := os.Open(c.path)
	if err != nil {
		return c.rows
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return c.rows
	}
	if info.Size() < c.offset {
		c.offset = 0
		c.rows = 0
		c.quoted = false
		c.seenHeader = false
	}

	if _, err := f.Seek(c.offset, io.SeekStart); err != nil {
		return c.rows
	}

	buffer := make([]byte, 32*1024)
	for {
		n, readErr := f.Read(buffer)
		for _, ch := range buffer[:n] {
			if !c.jsonl && ch == '"' {
				c.quoted = !c.quoted
			}
			if ch == '\n' && (c.jsonl || !c.quoted) {
				if c.jsonl {
					c.rows++
				} else if !c.seenHeader {
					c.seenHeader = true
				} else {
					c.rows++
				}
			}
		}
		c.offset += int64(n)
		if readErr != nil || n == 0 {
			break
		}
	}

	return c.rows
}
