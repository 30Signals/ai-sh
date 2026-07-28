// Package memory stores durable, user-authored notes that are prepended to
// every system prompt: shell preferences, tool choices, machine quirks.
//
// The store is a hand-editable markdown file living beside config.json, so a
// user can maintain it with an editor and never touch the CLI. Nothing is
// written implicitly — entries only appear when the user runs `ai --remember`
// or edits the file.
package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/ai-sh/internal/config"
)

const (
	// MaxEntries bounds the file so the system prompt cannot grow without
	// limit; the local models have small context windows.
	MaxEntries = 50
	// MaxEntryLen bounds a single note. Anything longer is prose, not a
	// preference, and belongs in the file by hand.
	MaxEntryLen = 200
	// maxPromptBytes caps what reaches the model even if the file was edited
	// by hand past the limits above.
	maxPromptBytes = 2000
)

const fileHeader = `# ai-sh memory
# One note per line. Lines starting with # are ignored.
# These notes are sent to the model with every prompt.
`

// Path returns the memory file location.
func Path() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "memory.md"), nil
}

// Load returns the stored notes in file order. A missing file yields no
// entries and no error.
func Load() ([]string, error) {
	lines, err := readLines()
	if err != nil {
		return nil, err
	}

	var entries []string
	for _, line := range lines {
		if entry, ok := parseEntry(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// Add appends a note, reporting false when an equivalent note is already
// stored. Existing lines are left alone, so hand-written comments survive.
func Add(entry string) (bool, error) {
	entry = normalize(entry)
	if entry == "" {
		return false, fmt.Errorf("nothing to remember")
	}
	if len(entry) > MaxEntryLen {
		return false, fmt.Errorf("note is %d characters, limit is %d — shorten it or edit the file directly", len(entry), MaxEntryLen)
	}

	existing, err := Load()
	if err != nil {
		return false, err
	}
	for _, e := range existing {
		if strings.EqualFold(e, entry) {
			return false, nil
		}
	}
	if len(existing) >= MaxEntries {
		return false, fmt.Errorf("memory holds the maximum of %d notes — drop one with: ai --forget <n>", MaxEntries)
	}

	path, err := Path()
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}

	body := ""
	if _, err := os.Stat(path); os.IsNotExist(err) {
		body = fileHeader
	} else if err != nil {
		return false, err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.WriteString(body + "- " + entry + "\n"); err != nil {
		return false, err
	}
	return true, nil
}

// Forget removes the 1-based nth note as listed by Load, leaving comments and
// blank lines untouched. It returns the note that was removed.
func Forget(n int) (string, error) {
	lines, err := readLines()
	if err != nil {
		return "", err
	}

	seen := 0
	for i, line := range lines {
		entry, ok := parseEntry(line)
		if !ok {
			continue
		}
		seen++
		if seen != n {
			continue
		}
		return entry, writeLines(append(lines[:i:i], lines[i+1:]...))
	}

	if seen == 0 {
		return "", fmt.Errorf("nothing remembered yet")
	}
	return "", fmt.Errorf("no note %d (memory holds %d)", n, seen)
}

// Clear removes every note, and the file with them.
func Clear() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Prompt renders the notes as a system prompt section, or "" when there is
// nothing to say. Errors are swallowed deliberately: an unreadable memory file
// must not stop a user from generating a command.
func Prompt() string {
	entries, err := Load()
	if err != nil || len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("User notes — apply them when relevant, ignore them otherwise:\n")
	for _, entry := range entries {
		line := "- " + entry + "\n"
		if sb.Len()+len(line) > maxPromptBytes {
			break
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// parseEntry reports whether a file line carries a note, and returns it
// without any bullet marker. Comments and blank lines carry nothing.
func parseEntry(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	for _, bullet := range []string{"- ", "* ", "-", "*"} {
		if strings.HasPrefix(trimmed, bullet) {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, bullet))
			break
		}
	}
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

// normalize folds a note onto one line so it cannot break the file format.
func normalize(entry string) string {
	entry = strings.ReplaceAll(entry, "\r", " ")
	entry = strings.ReplaceAll(entry, "\n", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(entry), " "))
}

func readLines() ([]string, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return lines, nil
}

func writeLines(lines []string) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
