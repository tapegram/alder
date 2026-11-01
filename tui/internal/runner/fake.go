package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Fake is a Runner for tests. It answers each call from the output scripted with On and records
// the calls it received.
type Fake struct {
	mu    sync.Mutex
	on    map[string]fakeReply
	calls []string
}

type fakeReply struct {
	out  string
	code int
}

// On scripts the reply to a call whose arguments, joined by spaces, equal args.
func (f *Fake) On(args string, out string, code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.on == nil {
		f.on = map[string]fakeReply{}
	}
	f.on[args] = fakeReply{out, code}
}

// Calls returns the arguments of every call so far, each joined by spaces, in order.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *Fake) reply(args []string) (fakeReply, error) {
	key := strings.Join(args, " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, key)
	r, ok := f.on[key]
	if !ok {
		return fakeReply{}, fmt.Errorf("runner.Fake: no reply scripted for %q", key)
	}
	return r, nil
}

func (f *Fake) Run(_ context.Context, _ string, args ...string) (Result, error) {
	r, err := f.reply(args)
	if err != nil {
		return Result{}, err
	}
	return Result{Stdout: []byte(r.out), Code: r.code}, nil
}

func (f *Fake) Stream(_ context.Context, _ string, args ...string) (Stream, error) {
	r, err := f.reply(args)
	if err != nil {
		return nil, err
	}
	var lines []string
	if r.out != "" {
		lines = strings.Split(strings.TrimSuffix(r.out, "\n"), "\n")
	}
	ch := make(chan string, len(lines))
	for _, l := range lines {
		ch <- l
	}
	close(ch)
	return fakeStream{ch: ch, code: r.code}, nil
}

type fakeStream struct {
	ch   chan string
	code int
}

func (s fakeStream) Lines() <-chan string { return s.ch }
func (s fakeStream) Wait() (int, []byte)  { return s.code, nil }
