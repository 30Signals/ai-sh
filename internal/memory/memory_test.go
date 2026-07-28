package memory

import (
	"os"
	"strings"
	"testing"
)

// isolate points HOME at a scratch directory so the tests never touch the real
// ~/.ai-sh; config.Dir resolves through os.UserHomeDir, which honours HOME.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestLoadMissingFile(t *testing.T) {
	isolate(t)

	entries, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("want no entries, got %v", entries)
	}
	if Prompt() != "" {
		t.Fatalf("want empty prompt, got %q", Prompt())
	}
}

func TestAddNormalizesAndDedupes(t *testing.T) {
	isolate(t)

	if added, err := Add("  prefer  rg\nover grep "); err != nil || !added {
		t.Fatalf("Add: added=%v err=%v", added, err)
	}
	if added, err := Add("PREFER RG over grep"); err != nil || added {
		t.Fatalf("duplicate should not be added: added=%v err=%v", added, err)
	}

	entries, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 1 || entries[0] != "prefer rg over grep" {
		t.Fatalf("got %q", entries)
	}
	if _, err := Add("   "); err == nil {
		t.Fatal("want error for empty note")
	}
	if _, err := Add(strings.Repeat("x", MaxEntryLen+1)); err == nil {
		t.Fatal("want error for over-long note")
	}
}

func TestForgetPreservesComments(t *testing.T) {
	isolate(t)

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(strings.TrimSuffix(path, "/memory.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A hand-written file: mixed bullets, a comment, a blank line.
	if err := os.WriteFile(path, []byte("# mine\n- one\n\n* two\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{"one", "two", "three"}; strings.Join(entries, ",") != strings.Join(want, ",") {
		t.Fatalf("got %q want %q", entries, want)
	}

	removed, err := Forget(2)
	if err != nil || removed != "two" {
		t.Fatalf("Forget(2) = %q, %v", removed, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "# mine\n- one\n\nthree\n" {
		t.Fatalf("rewrite dropped context: %q", got)
	}

	if _, err := Forget(9); err == nil {
		t.Fatal("want error for out-of-range note")
	}
}

func TestPromptListsEntries(t *testing.T) {
	isolate(t)

	if _, err := Add("prefer rg over grep"); err != nil {
		t.Fatal(err)
	}
	prompt := Prompt()
	if !strings.Contains(prompt, "User notes") || !strings.Contains(prompt, "- prefer rg over grep\n") {
		t.Fatalf("got %q", prompt)
	}
}

func TestClear(t *testing.T) {
	isolate(t)

	if _, err := Add("something"); err != nil {
		t.Fatal(err)
	}
	if err := Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if entries, _ := Load(); len(entries) != 0 {
		t.Fatalf("want empty after Clear, got %v", entries)
	}
	// Clearing an already-empty store is not an error.
	if err := Clear(); err != nil {
		t.Fatalf("Clear twice: %v", err)
	}
}
