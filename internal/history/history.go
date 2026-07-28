// Package history keeps recent ai-sh turns per terminal session, so a follow-up
// instruction can refer back to what came before.
//
// Everything here is best-effort. A missing, truncated, or corrupt transcript
// degrades to no history; it must never turn into an error the user sees, since
// a broken side file is not a reason to stop generating commands.
package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// DefaultTurns is how many past turns feed the next prompt.
	DefaultTurns = 6

	// Write-time caps. Nothing longer than this reaches disk, which is what
	// keeps a single pathological record from ever dominating the budget.
	maxPromptChars  = 500
	maxCommandChars = 500

	// maxRecords bounds the file so the read path can stay a plain full read:
	// no reverse scanner, no rotation.
	maxRecords = 200

	// maxBlockChars caps the assembled context. Records are ~40 tokens, so
	// this is generous for the turn counts involved; it exists to bound the
	// pathological case, not the normal one.
	maxBlockChars = 4000

	// sessionTTL expires a transcript. It also backstops PID reuse when the
	// shell start time cannot be read.
	sessionTTL = 2 * time.Hour
)

// Record is one instruction and the command it produced.
type Record struct {
	Time    time.Time `json:"ts"`
	Cwd     string    `json:"cwd"`
	Prompt  string    `json:"prompt"`
	Command string    `json:"command"`
	Ran     bool      `json:"ran"`
	Exit    int       `json:"exit,omitempty"`
}

// redactors blank out secrets before they are written. Redaction happens on
// write, never on read: a value that never lands in the file cannot be leaked
// later by a change to how the file is consumed.
var redactors = []*regexp.Regexp{
	// FOO_TOKEN=..., AWS_SECRET_ACCESS_KEY=..., password=...
	regexp.MustCompile(`(?i)([A-Z0-9_]*(?:SECRET|TOKEN|PASSWORD|PASSWD|API_?KEY|CREDENTIALS?|PRIVATE_KEY)[A-Z0-9_]*\s*=\s*)([^\s"']+)`),
	// Authorization: Bearer ...
	regexp.MustCompile(`(?i)(authorization:\s*(?:bearer\s+|basic\s+)?)([^\s"']+)`),
	// --password x, --token=x
	regexp.MustCompile(`(?i)(--(?:password|token|api-?key|secret)(?:=|\s+))([^\s"']+)`),
}

// Redact replaces secret-shaped values with a placeholder.
func Redact(s string) string {
	for _, re := range redactors {
		s = re.ReplaceAllString(s, "${1}<redacted>")
	}
	return s
}

// Path returns the transcript file for the calling terminal.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	key, err := sessionID()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ai-sh", "sessions", key+".jsonl"), nil
}

// Append records one turn, trimming the file back to maxRecords.
func Append(rec Record) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	rec.Prompt = clip(Redact(rec.Prompt), maxPromptChars)
	rec.Command = clip(Redact(rec.Command), maxCommandChars)
	if rec.Time.IsZero() {
		rec.Time = time.Now()
	}

	records := append(read(path), rec)
	if len(records) > maxRecords {
		records = records[len(records)-maxRecords:]
	}

	if err := write(path, records); err != nil {
		return err
	}
	prune()
	return nil
}

// Load returns the turns that should feed the next prompt, oldest first.
//
// Records from another directory are dropped: the working directory is already
// part of the prompt, and mixing directories produces more confusion than
// context. Changing directory therefore starts a clean slate.
func Load(turns int) []Record {
	if turns <= 0 {
		turns = DefaultTurns
	}
	path, err := Path()
	if err != nil {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}

	cutoff := time.Now().Add(-sessionTTL)
	var kept []Record
	for _, rec := range read(path) {
		if rec.Time.Before(cutoff) || rec.Cwd != cwd || rec.Command == "" {
			continue
		}
		kept = append(kept, rec)
	}

	if len(kept) > turns {
		kept = kept[len(kept)-turns:]
	}
	return applyBudget(kept)
}

// Clear drops the calling terminal's transcript.
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

// applyBudget trims to maxBlockChars newest-first, so the most recent turn --
// the one the user is actually following up on -- is never the one dropped. A
// single oversized record is truncated rather than discarded, since dropping it
// would silently break that follow-up.
func applyBudget(records []Record) []Record {
	total := 0
	for i := len(records) - 1; i >= 0; i-- {
		cost := len(records[i].Prompt) + len(records[i].Command)
		if total+cost <= maxBlockChars {
			total += cost
			continue
		}
		if i == len(records)-1 {
			half := maxBlockChars / 2
			records[i].Prompt = clip(records[i].Prompt, half)
			records[i].Command = clip(records[i].Command, half)
			return records[i:]
		}
		return records[i+1:]
	}
	return records
}

// read parses a transcript, skipping lines it cannot understand. A half-written
// final line is expected after an interrupted write and is not an error.
func read(path string) []Record {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var records []Record
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records
}

// write replaces the transcript atomically, so a crash mid-write cannot leave
// the file shorter than it was.
func write(path string, records []Record) error {
	var sb strings.Builder
	for _, rec := range records {
		data, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// prune deletes transcripts past the TTL, including those of terminals that
// exited without cleaning up.
func prune() {
	path, err := Path()
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-sessionTTL)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		os.Remove(filepath.Join(dir, entry.Name()))
	}
}

// clip shortens a string to max runes, eliding the middle.
func clip(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	head := (max - 1) / 2
	tail := max - 1 - head
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

// sessionID identifies the calling terminal by its shell process. The shell
// start time is included because PIDs are reused: without it, a new shell
// inheriting an old PID would silently adopt a stranger's transcript.
func sessionID() (string, error) {
	ppid := os.Getppid()
	if ppid <= 0 {
		return "", fmt.Errorf("no parent process")
	}
	token := shellStartToken(ppid)
	if token == "" {
		// Unknown start time: the TTL is the only guard left against PID
		// reuse, which is why it is short.
		token = "unknown"
	}
	return fmt.Sprintf("%d-%s", ppid, safeToken(token)), nil
}

// safeToken keeps the start token usable as a filename.
func safeToken(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			return r
		default:
			return -1
		}
	}, s)
}
