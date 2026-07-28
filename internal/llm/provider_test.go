package llm

import (
	"strings"
	"testing"
)

func TestCurrentInstruction(t *testing.T) {
	messages := []Message{User("list files"), Assistant("ls"), User("only pdfs")}

	last, prior, err := currentInstruction(messages)
	if err != nil {
		t.Fatal(err)
	}
	if last.Content != "only pdfs" {
		t.Errorf("current instruction is %q, want the trailing user turn", last.Content)
	}
	if len(prior) != 2 {
		t.Errorf("got %d prior turns, want 2", len(prior))
	}

	if _, _, err := currentInstruction(nil); err == nil {
		t.Error("empty conversation should be rejected")
	}
	if _, _, err := currentInstruction([]Message{User("x"), Assistant("ls")}); err == nil {
		t.Error("a conversation ending in an assistant turn should be rejected")
	}
}

// The local backend cannot carry roles, so prior turns are folded into the
// system prompt as labelled lines.
func TestBuildSystemPromptFoldsPriorTurns(t *testing.T) {
	got := buildSystemPrompt([]Message{User("list files"), Assistant("ls -la")})

	for _, want := range []string{"instruction: list files", "command: ls -la"} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildSystemPromptWithoutPriorTurns(t *testing.T) {
	got := buildSystemPrompt(nil)

	if strings.Contains(got, "Earlier in this session") {
		t.Errorf("system prompt mentions history with no prior turns:\n%s", got)
	}
	if !strings.Contains(got, "Current directory:") {
		t.Errorf("system prompt lost the working directory:\n%s", got)
	}
}

// A folded turn spanning lines would break the line-per-turn layout.
func TestBuildSystemPromptFlattensMultilineTurns(t *testing.T) {
	got := buildSystemPrompt([]Message{User("do\nthis"), Assistant("ls")})

	if strings.Contains(got, "instruction: do\nthis") {
		t.Errorf("multi-line turn was not flattened:\n%s", got)
	}
	if !strings.Contains(got, "instruction: do this") {
		t.Errorf("flattened turn missing:\n%s", got)
	}
}

func TestStripMarkdown(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare", "ls -la", "ls -la"},
		{"fenced", "```sh\nls -la\n```", "ls -la"},
		{"fenced with prose", "Here you go:\n```\nls -la\n```\nEnjoy", "ls -la"},
		{"backticked", "`ls -la`", "ls -la"},
		{"prompt prefix", "$ ls -la", "ls -la"},
		{"first line wins", "ls -la\nrm -rf /", "ls -la"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripMarkdown(tc.in); got != tc.want {
				t.Errorf("stripMarkdown(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
