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
	out, stderr string
	code        int
}

// On scripts the reply to a call whose arguments, joined by spaces, equal args: out on stdout
// and the exit status code.
func (f *Fake) On(args string, out string, code int) { f.set(args, fakeReply{out: out, code: code}) }

// OnStderr scripts a reply that writes nothing to stdout, stderr to stderr, and exits code.
func (f *Fake) OnStderr(args string, stderr string, code int) {
	f.set(args, fakeReply{stderr: stderr, code: code})
}

func (f *Fake) set(args string, r fakeReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.on == nil {
		f.on = map[string]fakeReply{}
	}
	f.on[args] = r
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

// Run answers as Exec would: ctx.Err() once ctx has ended, else the scripted reply.
func (f *Fake) Run(ctx context.Context, _ string, args ...string) (Result, error) {
	r, err := f.reply(args)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return Result{Stdout: []byte(r.out), Stderr: []byte(r.stderr), Code: r.code}, nil
}

// Stream answers as Exec would: its lines are the scripted stdout's, split on "\n" with a
// trailing "\r" cut, and its Wait gives ctx.Err() once ctx has ended, else the scripted exit
// status and stderr.
func (f *Fake) Stream(ctx context.Context, _ string, args ...string) (Stream, error) {
	r, err := f.reply(args)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var lines []string
	if r.out != "" {
		lines = strings.Split(strings.TrimSuffix(r.out, "\n"), "\n")
	}
	ch := make(chan string, len(lines))
	for _, l := range lines {
		ch <- strings.TrimSuffix(l, "\r")
	}
	close(ch)
	return fakeStream{ctx: ctx, ch: ch, reply: r}, nil
}

type fakeStream struct {
	ctx   context.Context
	ch    chan string
	reply fakeReply
}

func (s fakeStream) Lines() <-chan string { return s.ch }

func (s fakeStream) Wait() (int, string, error) {
	if err := s.ctx.Err(); err != nil {
		return -1, "", err
	}
	return s.reply.code, s.reply.stderr, nil
}
