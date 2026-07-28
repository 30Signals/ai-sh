package llm

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/ai-sh/internal/config"
)

// Roles for Message. Only these two appear: the system prompt is the
// provider's business, since each backend passes it differently.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is one turn of the exchange. Assistant turns hold a bare command.
type Message struct {
	Role    string
	Content string
}

// User builds a user turn.
func User(content string) Message { return Message{Role: RoleUser, Content: content} }

// Assistant builds an assistant turn.
func Assistant(content string) Message { return Message{Role: RoleAssistant, Content: content} }

// Provider turns a natural language instruction into a shell command.
type Provider interface {
	// Generate returns a single command for the conversation, whose last
	// message must be the user's current instruction. Earlier messages are
	// context: prior instructions, the commands they produced, and any
	// refinements.
	Generate(messages []Message) (string, error)
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

// currentInstruction splits the trailing user turn from its context.
func currentInstruction(messages []Message) (Message, []Message, error) {
	if len(messages) == 0 {
		return Message{}, nil, fmt.Errorf("no instruction to send")
	}
	last := messages[len(messages)-1]
	if last.Role != RoleUser {
		return Message{}, nil, fmt.Errorf("last message must be a user instruction, got %q", last.Role)
	}
	return last, messages[:len(messages)-1], nil
}

// buildSystemPrompt constructs the system prompt with shell context. Prior
// turns are folded in as text for backends that cannot carry real roles; pass
// nil when the backend sends them as messages instead.
func buildSystemPrompt(prior []Message) string {
	var sb strings.Builder
	sb.WriteString("Convert the instruction to a single POSIX sh command. Output ONLY the command, no explanation, no markdown, no backticks.\n")

	if cwd, err := os.Getwd(); err == nil {
		sb.WriteString("Current directory: " + cwd + "\n")
	}

	if len(prior) > 0 {
		sb.WriteString("\nEarlier in this session, oldest first. The instruction may refer back to it or correct it:\n")
		for _, msg := range prior {
			switch msg.Role {
			case RoleUser:
				sb.WriteString("instruction: " + oneLine(msg.Content) + "\n")
			case RoleAssistant:
				sb.WriteString("command: " + oneLine(msg.Content) + "\n")
			}
		}
	}

	return sb.String()
}

// oneLine keeps folded-in context from breaking the line-per-turn layout.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
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
