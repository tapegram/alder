package fill

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// overloadedBackoff is the wait before the one retry of a 429 or 529 answer (a variable for
// tests).
var overloadedBackoff = 2 * time.Second

// maxErrorText bounds the API's error text quoted in an error.
const maxErrorText = 300

// Anthropic is the Messages API backend, over net/http (as internal/deploy's Render client;
// no SDK). The key travels only in the x-api-key header: it is in no error, log or String().
type Anthropic struct {
	Base      string // no trailing slash: https://api.anthropic.com
	Key       string
	Model     string
	Reference string // a file whose text is prepended to the system prompt, "" for none
	Client    *http.Client
	// Log, when set, receives each request body and each answer (ALDER_LSP_LOG): never headers.
	Log io.Writer

	refOnce sync.Once
	ref     string
	refErr  error
}

// NewAnthropic reads ANTHROPIC_API_KEY, ALDER_ANTHROPIC_API (default https://api.anthropic.com),
// ALDER_FILL_MODEL (default DefaultModel) and ALDER_FILL_REFERENCE with getenv.
func NewAnthropic(getenv func(string) string) *Anthropic {
	base := strings.TrimRight(getenv("ALDER_ANTHROPIC_API"), "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	model := getenv("ALDER_FILL_MODEL")
	if model == "" {
		model = DefaultModel
	}
	return &Anthropic{Base: base, Key: getenv("ANTHROPIC_API_KEY"), Model: model, Reference: getenv("ALDER_FILL_REFERENCE"), Client: noRedirects()}
}

// noRedirects is the backend's HTTP client: it follows no redirect, since Go re-sends custom
// headers (x-api-key among them) to a redirect's host, which may be another one. A redirect is
// then an answer like any other non-2xx.
func noRedirects() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// checkBase refuses a base URL the key would travel to in clear text: https, or http only to
// the loopback host (a local test server).
func checkBase(base string) error {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return fmt.Errorf("ALDER_ANTHROPIC_API is not a URL: %q", base)
	}
	switch h := u.Hostname(); {
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && (h == "localhost" || h == "127.0.0.1" || h == "::1"):
		return nil
	}
	return fmt.Errorf("ALDER_ANTHROPIC_API must be an https URL (http only for localhost): the key would travel in clear text to %s", u.Host)
}

// String and GoString never show the key.
func (a *Anthropic) String() string   { return "Anthropic{Base: " + a.Base + ", Model: " + a.Model + "}" }
func (a *Anthropic) GoString() string { return a.String() }

type textBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl map[string]string `json:"cache_control,omitempty"`
}

type messagesRequest struct {
	Model     string      `json:"model"`
	MaxTokens int         `json:"max_tokens"`
	System    []textBlock `json:"system"`
	Messages  []Message   `json:"messages"`
}

type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// reference is the reference file's text, read once.
func (a *Anthropic) reference() (string, error) {
	a.refOnce.Do(func() {
		if a.Reference == "" {
			return
		}
		b, err := os.ReadFile(a.Reference)
		if err != nil {
			a.refErr = fmt.Errorf("ALDER_FILL_REFERENCE: %v", err)
			return
		}
		a.ref = string(b)
	})
	return a.ref, a.refErr
}

// Complete posts one Messages request: the system prompt as a cached block (after the
// reference's own cached block, when set), the turns as they are. An answer that stopped for
// another reason than end_turn fails with NoRetry.
func (a *Anthropic) Complete(ctx context.Context, system string, messages []Message) (string, error) {
	if a.Key == "" {
		return "", NoBackend
	}
	if err := checkBase(a.Base); err != nil {
		return "", err
	}
	ephemeral := map[string]string{"type": "ephemeral"}
	req := messagesRequest{Model: a.Model, MaxTokens: 4096, Messages: messages}
	ref, err := a.reference()
	if err != nil {
		return "", err
	}
	if ref != "" {
		req.System = append(req.System, textBlock{Type: "text", Text: ref, CacheControl: ephemeral})
	}
	req.System = append(req.System, textBlock{Type: "text", Text: system, CacheControl: ephemeral})
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	if a.Log != nil {
		fmt.Fprintf(a.Log, "alder fill: POST %s/v1/messages %s\n", a.Base, body)
	}
	status, rb, err := a.post(ctx, body)
	if err == nil && (status == 429 || status == 529) { // rate-limited or overloaded: once more, shortly
		t := time.NewTimer(overloadedBackoff)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return "", ctx.Err()
		}
		status, rb, err = a.post(ctx, body)
	}
	if err != nil {
		return "", err
	}
	var r messagesResponse
	jerr := json.Unmarshal(rb, &r)
	if status/100 != 2 {
		msg := strings.TrimSpace(string(rb))
		if jerr == nil && r.Error != nil {
			msg = r.Error.Message
		}
		if len(msg) > maxErrorText {
			msg = msg[:maxErrorText] + "…"
		}
		return "", fmt.Errorf("the Anthropic API answered %d: %s", status, msg)
	}
	if jerr != nil {
		return "", fmt.Errorf("the Anthropic API: %v", jerr)
	}
	switch r.StopReason {
	case "end_turn":
	case "refusal":
		return "", &NoRetry{"the model declined"}
	case "max_tokens":
		return "", &NoRetry{"the answer did not end"}
	default:
		return "", &NoRetry{"the answer stopped: " + r.StopReason}
	}
	var text strings.Builder
	for _, c := range r.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	return text.String(), nil
}

// post sends one request body and reads the answer (at most 4 MB). The error never quotes a
// header, so never the key.
func (a *Anthropic) post(ctx context.Context, body []byte) (status int, answer []byte, err error) {
	hreq, err := http.NewRequestWithContext(ctx, "POST", a.Base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	hreq.Header.Set("content-type", "application/json")
	hreq.Header.Set("x-api-key", a.Key)
	hreq.Header.Set("anthropic-version", "2023-06-01")
	client := a.Client
	if client == nil {
		client = noRedirects()
	}
	resp, err := client.Do(hreq)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return 0, nil, fmt.Errorf("the Anthropic API: %v", strings.ReplaceAll(err.Error(), a.Key, "…"))
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, nil, err
	}
	if a.Log != nil {
		fmt.Fprintf(a.Log, "alder fill: answer %d %s\n", resp.StatusCode, rb)
	}
	return resp.StatusCode, rb, nil
}
