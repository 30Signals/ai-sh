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

func TestFormatReply(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		isProse bool
	}{
		{"bare command", "ls -la", "ls -la", false},
		{"fenced command", "```sh\nls -la\n```", "ls -la", false},
		{"fenced with prose", "Here you go:\n```\nls -la\n```\nEnjoy", "ls -la", false},
		{"backticked", "`ls -la`", "ls -la", false},
		{"prompt sign stripped", "$ ls -la", "ls -la", false},
		{"first line wins", "ls -la\nrm -rf /", "ls -la", false},
		{"answer", "ANSWER: chmod 755 sets rwxr-xr-x.", "chmod 755 sets rwxr-xr-x.", true},
		{"answer lowercase", "answer: hello there.", "hello there.", true},
		{"answer after preamble", "Sure!\nANSWER: it lists files.", "it lists files.", true},
		{"answer wrapped in markdown", "**ANSWER:** it lists files.", "it lists files.", true},
		{"answer over two lines", "ANSWER: it lists files\nin the directory.", "it lists files in the directory.", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reply := formatReply(tc.raw)
			text, ok := SplitAnswer(reply)
			if ok != tc.isProse {
				t.Fatalf("SplitAnswer prose = %v, want %v (reply %q)", ok, tc.isProse, reply)
			}
			got := reply
			if ok {
				got = text
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A command that merely mentions the word answer must not be treated as prose.
func TestFormatReplyCommandMentioningAnswer(t *testing.T) {
	reply := formatReply("grep -rn 'answer:' .")
	if _, ok := SplitAnswer(reply); ok {
		t.Errorf("command was misread as prose: %q", reply)
	}
}
