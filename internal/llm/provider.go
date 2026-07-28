package llm

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/user/ai-sh/internal/config"
)

// Provider turns a natural language instruction into a shell command.
type Provider interface {
	// Generate returns a single command for the given instruction.
	Generate(prompt string) (string, error)
	// Describe names the backend for error messages and status output.
	Describe() string
}

// New builds the provider selected by cfg.
func New(cfg config.Config) (Provider, error) {
	resolved, err := cfg.Resolve()
	if err != nil {
		return nil, err
	}

	if resolved.Provider == "local" {
		return newLocal()
	}
	return newCloud(resolved)
}

// AnswerPrefix marks a reply that is prose rather than a command. The system
// prompt asks for it whenever the request is not a shell task, so the caller
// can print the text instead of offering to execute it.
const AnswerPrefix = "ANSWER:"

// SplitAnswer reports whether a reply is prose and returns the text without
// the marker.
func SplitAnswer(reply string) (string, bool) {
	trimmed := strings.TrimSpace(reply)
	if len(trimmed) < len(AnswerPrefix) || !strings.EqualFold(trimmed[:len(AnswerPrefix)], AnswerPrefix) {
		return "", false
	}
	return strings.TrimSpace(trimmed[len(AnswerPrefix):]), true
}

// buildSystemPrompt constructs the system prompt: command generation rules
// first, then the prose escape hatch, then this machine's context. Small local
// models follow it better with the examples at the end, closest to the reply.
func buildSystemPrompt() string {
	var sb strings.Builder

	sb.WriteString(`You are a command-line assistant. The user types a request in a terminal and you reply with one line, nothing else.

If the request is something the shell can do, reply with a single command:
- Output only the command. No explanation, no markdown, no backticks, no leading "$".
- One line only. Chain steps with pipes, && or ; instead of several lines.
- Write POSIX sh. Use common tools: ls, find, grep, sed, awk, xargs, du, tar, curl, ps, git.
- Never invent flags. If unsure, use a simpler form that you know works.
- Quote paths that may contain spaces, and use -- before user-supplied names when the tool supports it.
- Add sudo only when the task truly needs root.
- Assume the request is about the current directory unless it names another path.
- Destructive requests are fine (the user confirms before anything runs), but stay scoped: no extra rm, reset, or force flags beyond what was asked.

If the request is NOT a shell task - a factual question, "what does this flag do", "explain X", or small talk - do not invent a command. Reply with "` + AnswerPrefix + ` " followed by a one or two sentence answer on the same line.

`)

	sb.WriteString("Context:\n")
	if cwd, err := os.Getwd(); err == nil {
		sb.WriteString("- Current directory: " + cwd + "\n")
	}
	sb.WriteString("- OS: " + osName() + ", " + userlandNote() + "\n")
	if shell := os.Getenv("SHELL"); shell != "" {
		sb.WriteString("- Login shell: " + shell + " (commands run under /bin/sh)\n")
	}

	sb.WriteString(`
Examples:
show the 20 biggest files here -> du -ah . | sort -rh | head -20
which branch am i on -> git rev-parse --abbrev-ref HEAD
kill whatever is on port 3000 -> lsof -ti:3000 | xargs kill -9
find todo in go files -> grep -rn TODO --include='*.go' .
what does chmod 755 mean -> ` + AnswerPrefix + ` It makes a file readable and executable by everyone and writable only by its owner.
`)

	return sb.String()
}

// osName reports a human label for the host OS.
func osName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

// userlandNote tells the model which flavour of the standard tools it is
// targeting, since GNU and BSD flags differ in ways that silently break
// commands (sed -i, grep -P, du -h ordering).
func userlandNote() string {
	if runtime.GOOS == "darwin" {
		return "BSD userland (sed -i needs an empty argument: sed -i '' ...; no grep -P, no GNU long flags)"
	}
	return "GNU coreutils (GNU sed, grep and find flags are available)"
}

// formatReply turns raw model output into either a prose answer or a bare
// command. Prose keeps its marker so callers can tell the two apart.
func formatReply(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := answerIndex(raw); i != -1 {
		rest := strings.TrimLeft(raw[i+len(AnswerPrefix):], " \t*`")
		text := strings.Join(strings.Fields(rest), " ")
		return AnswerPrefix + " " + text
	}
	return stripMarkdown(raw)
}

// answerIndex finds the answer marker at the start of any line, tolerating the
// commentary small models like to put in front of it.
func answerIndex(s string) int {
	for offset := 0; offset < len(s); {
		line := s[offset:]
		if nl := strings.IndexByte(line, '\n'); nl != -1 {
			line = line[:nl]
		}
		trimmed := strings.TrimLeft(line, " \t`*")
		if len(trimmed) >= len(AnswerPrefix) && strings.EqualFold(trimmed[:len(AnswerPrefix)], AnswerPrefix) {
			return offset + len(line) - len(trimmed)
		}
		offset += len(line) + 1
	}
	return -1
}

// stripMarkdown extracts a bare command from the output.
// If a fenced code block exists anywhere, returns its first line.
// Falls back to the first non-empty line.
func stripMarkdown(s string) string {
	if idx := strings.Index(s, "```"); idx != -1 {
		rest := s[idx+3:]
		if nl := strings.Index(rest, "\n"); nl != -1 {
			rest = rest[nl+1:]
		}
		if end := strings.Index(rest, "```"); end != -1 {
			rest = rest[:end]
		}
		for _, line := range strings.Split(rest, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return line
			}
		}
	}

	s = strings.Trim(s, "`")
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "$ ")
		if line != "" {
			return line
		}
	}

	return strings.TrimSpace(s)
}

// missingLocalRuntime reports the install hint shown when llama.cpp or a model
// is absent, pointing at cloud setup as the lighter alternative.
func missingLocalRuntime(what string) error {
	return fmt.Errorf("%s\n\nInstall the local runtime:\n  curl -fsSL https://raw.githubusercontent.com/30Signals/ai-sh/main/install.sh | bash\nOr use a cloud model instead:\n  ai --setup", what)
}
