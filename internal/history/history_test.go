package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRedact(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"export AWS_SECRET_ACCESS_KEY=abc123", "export AWS_SECRET_ACCESS_KEY=<redacted>"},
		{"MY_API_KEY=zzz make deploy", "MY_API_KEY=<redacted> make deploy"},
		{`curl -H "Authorization: Bearer sk-live-9" https://x`, `curl -H "Authorization: Bearer <redacted>" https://x`},
		{"mysql --password=hunter2 -u root", "mysql --password=<redacted> -u root"},
		{"ls -la /tmp", "ls -la /tmp"},
	}

	for _, tc := range cases {
		if got := Redact(tc.in); got != tc.want {
			t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestClipElidesMiddle(t *testing.T) {
	got := clip("abcdefghij", 5)
	if len([]rune(got)) != 5 {
		t.Fatalf("clip returned %q, want 5 runes", got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("clip(%q) = %q, want an elision marker", "abcdefghij", got)
	}
	if got := clip("short", 50); got != "short" {
		t.Errorf("clip left a short string alone as %q", got)
	}
	// Multi-byte input must not be cut mid-rune.
	if got := clip(strings.Repeat("é", 40), 10); !isValidRunes(got) {
		t.Errorf("clip produced invalid runes: %q", got)
	}
}

func isValidRunes(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// The newest turn is the one the user is following up on, so the budget must
// never drop it in favour of older context.
func TestApplyBudgetDropsOldestFirst(t *testing.T) {
	big := strings.Repeat("x", 1500)
	records := []Record{
		{Prompt: "oldest", Command: big},
		{Prompt: "middle", Command: big},
		{Prompt: "newest", Command: big},
	}

	got := applyBudget(records)

	if len(got) == 0 {
		t.Fatal("applyBudget returned nothing")
	}
	if got[len(got)-1].Prompt != "newest" {
		t.Errorf("last kept record is %q, want newest", got[len(got)-1].Prompt)
	}
	if len(got) == len(records) {
		t.Error("applyBudget kept everything, expected the oldest to be dropped")
	}
	total := 0
	for _, rec := range got {
		total += len(rec.Prompt) + len(rec.Command)
	}
	if total > maxBlockChars {
		t.Errorf("kept %d chars, over the %d budget", total, maxBlockChars)
	}
}

// A single oversized record is truncated rather than dropped: dropping it would
// silently break the follow-up the user is in the middle of.
func TestApplyBudgetTruncatesOversizedNewest(t *testing.T) {
	records := []Record{{Prompt: "p", Command: strings.Repeat("y", maxBlockChars*2)}}

	got := applyBudget(records)

	if len(got) != 1 {
		t.Fatalf("got %d records, want the oversized one kept", len(got))
	}
	if len(got[0].Command) > maxBlockChars {
		t.Errorf("command still %d chars, want it truncated", len(got[0].Command))
	}
}

func TestAppendLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for _, prompt := range []string{"list files", "only pdfs"} {
		if err := Append(Record{Cwd: cwd, Prompt: prompt, Command: "ls " + prompt, Ran: true}); err != nil {
			t.Fatalf("Append(%q): %v", prompt, err)
		}
	}

	got := Load(DefaultTurns)
	if len(got) != 2 {
		t.Fatalf("Load returned %d records, want 2", len(got))
	}
	if got[0].Prompt != "list files" || got[1].Prompt != "only pdfs" {
		t.Errorf("Load returned %q then %q, want chronological order", got[0].Prompt, got[1].Prompt)
	}
}

func TestAppendRedactsBeforeWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd, _ := os.Getwd()

	if err := Append(Record{Cwd: cwd, Prompt: "set the key", Command: "export API_KEY=supersecret"}); err != nil {
		t.Fatal(err)
	}

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "supersecret") {
		t.Error("the secret reached disk; redaction must happen on write")
	}
}

// Another directory's turns are dropped, so changing directory starts clean.
func TestLoadFiltersOtherDirectories(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd, _ := os.Getwd()

	if err := Append(Record{Cwd: "/somewhere/else", Prompt: "elsewhere", Command: "ls"}); err != nil {
		t.Fatal(err)
	}
	if err := Append(Record{Cwd: cwd, Prompt: "here", Command: "ls"}); err != nil {
		t.Fatal(err)
	}

	got := Load(DefaultTurns)
	if len(got) != 1 || got[0].Prompt != "here" {
		t.Errorf("Load returned %+v, want only the current directory's turn", got)
	}
}

func TestLoadDropsExpiredTurns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd, _ := os.Getwd()

	if err := Append(Record{Time: time.Now().Add(-2 * sessionTTL), Cwd: cwd, Prompt: "stale", Command: "ls"}); err != nil {
		t.Fatal(err)
	}

	if got := Load(DefaultTurns); len(got) != 0 {
		t.Errorf("Load returned %+v, want expired turns dropped", got)
	}
}

// A corrupt or half-written line must not take the whole transcript with it.
func TestLoadSkipsCorruptLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd, _ := os.Getwd()

	if err := Append(Record{Cwd: cwd, Prompt: "good", Command: "ls"}); err != nil {
		t.Fatal(err)
	}
	path, _ := Path()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString("{not json at all\n")
	file.Close()

	got := Load(DefaultTurns)
	if len(got) != 1 || got[0].Prompt != "good" {
		t.Errorf("Load returned %+v, want the one readable record", got)
	}
}

func TestAppendTrimsToMaxRecords(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd, _ := os.Getwd()

	for i := 0; i < maxRecords+10; i++ {
		if err := Append(Record{Cwd: cwd, Prompt: "p", Command: "ls"}); err != nil {
			t.Fatal(err)
		}
	}

	path, _ := Path()
	if got := len(read(path)); got != maxRecords {
		t.Errorf("transcript holds %d records, want it trimmed to %d", got, maxRecords)
	}
}

func TestClearRemovesTranscript(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd, _ := os.Getwd()

	if err := Append(Record{Cwd: cwd, Prompt: "p", Command: "ls"}); err != nil {
		t.Fatal(err)
	}
	if err := Clear(); err != nil {
		t.Fatal(err)
	}
	if got := Load(DefaultTurns); len(got) != 0 {
		t.Errorf("Load returned %+v after Clear", got)
	}
	// Clearing an already-clear session is not an error.
	if err := Clear(); err != nil {
		t.Errorf("second Clear: %v", err)
	}
}

// The session id must distinguish a reused PID from the shell that held it.
func TestSessionIDIncludesStartToken(t *testing.T) {
	id, err := sessionID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(id, "-") {
		t.Fatalf("session id %q has no start token", id)
	}
	if filepath.Base(id) != id {
		t.Errorf("session id %q is not usable as a filename", id)
	}
}
