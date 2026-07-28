package llm

import "testing"

func TestFormatReply(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		isProse bool
	}{
		{"bare command", "ls -la", "ls -la", false},
		{"fenced command", "```sh\nls -la\n```", "ls -la", false},
		{"prompt sign stripped", "$ ls -la", "ls -la", false},
		{"answer", "ANSWER: chmod 755 sets rwxr-xr-x.", "chmod 755 sets rwxr-xr-x.", true},
		{"answer lowercase", "answer: hello there.", "hello there.", true},
		{"answer after preamble", "Sure!\nANSWER: it lists files.", "it lists files.", true},
		{"answer wrapped in markdown", "**ANSWER:** it lists files.", "it lists files.", true},
		{"answer over two lines", "ANSWER: it lists files\nin the directory.", "it lists files in the directory.", true},
	}

	for _, tc := range cases {
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
