// Package fill fills a typed hole with a model (issue #46, book chapter 13 §10): the prompt
// (Prompt, the embedded card.md), the backends that answer it (the Anthropic Messages API, or a
// local command that doubles as the tests' fake), and the loop (Run) that checks each answer
// exactly as `alder fill -` checks an expression on standard input, retrying with the
// checker's errors. Run never writes: `alder fill --ai` applies its edits, the language server
// turns them into a workspace edit.
package fill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Message is one turn of the conversation: Role "user" or "assistant".
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Backend answers a conversation: the system prompt and the turns so far, the model's text.
type Backend interface {
	Complete(ctx context.Context, system string, messages []Message) (string, error)
}

// BackendFunc is a function as a Backend (tests).
type BackendFunc func(ctx context.Context, system string, messages []Message) (string, error)

func (f BackendFunc) Complete(ctx context.Context, system string, messages []Message) (string, error) {
	return f(ctx, system, messages)
}

// NoBackend is the error of a fill with neither backend configured.
var NoBackend = errors.New("set ANTHROPIC_API_KEY, or ALDER_FILL_COMMAND, in the environment the editor was started from")

// NoRetry marks a backend error that a retry cannot fix (the model declined, or its answer did
// not end): Run gives up at once.
type NoRetry struct{ Msg string }

func (e *NoRetry) Error() string { return e.Msg }

// DefaultModel is the model of the Anthropic backend when ALDER_FILL_MODEL is not set.
const DefaultModel = "claude-opus-5-5"

// FromEnv is the backend the environment configures, and the name the callers report it by:
// ALDER_FILL_COMMAND (a command, "command"), else ANTHROPIC_API_KEY (the Messages API at
// ALDER_ANTHROPIC_API, the model ALDER_FILL_MODEL, ALDER_FILL_REFERENCE prepended to the system
// prompt). Neither is a nil Backend. The key is read here and nowhere else.
func FromEnv(getenv func(string) string) (Backend, string) {
	if c := getenv("ALDER_FILL_COMMAND"); strings.TrimSpace(c) != "" {
		return &Command{Cmd: c}, "command"
	}
	key := getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, ""
	}
	a := NewAnthropic(getenv)
	return a, a.Model
}

// System is the system prompt as the first attempt sends it: the card, after the text of the
// reference file ALDER_FILL_REFERENCE names when getenv gives one (the Anthropic backend sends
// it as a block of its own).
func System(getenv func(string) string) (string, error) {
	ref := getenv("ALDER_FILL_REFERENCE")
	if ref == "" {
		return Card, nil
	}
	b, err := os.ReadFile(ref)
	if err != nil {
		return "", fmt.Errorf("ALDER_FILL_REFERENCE: %v", err)
	}
	return strings.TrimRight(string(b), "\n") + "\n\n" + Card, nil
}

// Env is FromEnv over the process's environment.
func Env() (Backend, string) { return FromEnv(os.Getenv) }

// Transcript is a conversation as plain text: the system prompt, a blank line, then the turns,
// a later turn after a blank line, the model's own under `Your answer:`. It is what the command
// backend reads on standard input, and what `alder fill --prompt` prints.
func Transcript(system string, messages []Message) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(system, "\n"))
	b.WriteString("\n")
	for _, m := range messages {
		b.WriteString("\n")
		if m.Role == "assistant" {
			b.WriteString("Your answer:\n")
		}
		b.WriteString(strings.TrimRight(m.Content, "\n"))
		b.WriteString("\n")
	}
	return b.String()
}
