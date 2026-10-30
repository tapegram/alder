package fill

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"strings"
	"testing"
)

// The final security review of #46.

// TestAnthropic_NoRedirect: a redirect is not followed, so the key never reaches the host it
// names; the redirect is reported as the API's answer.
func TestAnthropic_NoRedirect(t *testing.T) {
	var leaked []string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = append(leaked, r.Header.Get("x-api-key"))
		io.WriteString(w, okAnswer)
	}))
	defer other.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v1/messages", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	a := NewAnthropic(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "secret-key", "ALDER_ANTHROPIC_API": redirect.URL}))
	_, err := a.Complete(context.Background(), "s", []Message{{"user", "x"}})
	if len(leaked) != 0 {
		t.Fatalf("the redirect was followed with the key: %v", leaked)
	}
	if err == nil || !strings.HasPrefix(err.Error(), "the Anthropic API answered 307") {
		t.Errorf("err %v", err)
	}
}

// TestAnthropic_HTTPSOnly: the key goes in clear text only to the loopback host.
func TestAnthropic_HTTPSOnly(t *testing.T) {
	for base, ok := range map[string]bool{
		"http://api.example.com":     false,
		"http://10.0.0.5:8080":       false,
		"ftp://api.anthropic.com":    false,
		"https://api.anthropic.com":  true,
		"http://localhost:9":         true,
		"http://127.0.0.1:9":         true,
		"http://[::1]:9":             true,
		"http://localhost.evil.com":  false,
		"http://127.0.0.1.nip.io:80": false,
	} {
		if err := checkBase(base); (err == nil) != ok {
			t.Errorf("checkBase(%q) = %v", base, err)
		}
	}
	srv, reqs, _ := fakeAPI(t, 200, okAnswer)
	_ = srv
	a := NewAnthropic(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "k", "ALDER_ANTHROPIC_API": "http://api.example.com"}))
	if _, err := a.Complete(context.Background(), "s", []Message{{"user", "x"}}); err == nil || !strings.Contains(err.Error(), "must be an https URL") || len(*reqs) != 0 {
		t.Errorf("err %v", err)
	}
}

// TestCommandBackend_Reference: the command reads the reference before the card, as
// `alder fill --prompt` prints it (System).
func TestCommandBackend_Reference(t *testing.T) {
	dir := t.TempDir()
	ref, in := filepath.Join(dir, "ref.md"), filepath.Join(dir, "stdin")
	os.WriteFile(ref, []byte("THE REFERENCE\n"), 0o644)
	b, _ := FromEnv(fakeEnv(map[string]string{"ALDER_FILL_COMMAND": "cat > " + in + "; echo x", "ALDER_FILL_REFERENCE": ref}))
	msgs := []Message{{"user", "fill it"}}
	if _, err := b.Complete(context.Background(), Card, msgs); err != nil {
		t.Fatal(err)
	}
	system, _ := System(fakeEnv(map[string]string{"ALDER_FILL_REFERENCE": ref}))
	if got, _ := os.ReadFile(in); string(got) != Transcript(system, msgs) {
		t.Errorf("the command read %.60q…, not what --prompt prints", got)
	}
}
