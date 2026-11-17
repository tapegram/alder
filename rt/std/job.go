package std

// The Job module (#168): Job.enqueue, enqueueAt, enqueueOnce and status as effect values over
// rt/jobs, the Go types of Job.State and Job.Status, and the pieces the generated job codecs
// are made of (gen's jobs.go): a job's parameters are JSON (rt.JsonValue), a record as an
// object of its fields, Maybe as null or the value, a custom type as {"tag": …, "args": […]},
// a Time as its milliseconds since the epoch and a Date as "YYYY-MM-DD".

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"alder/rt"
	"alder/rt/eff"
	"alder/rt/jobs"
)

// JobStateTag is the tag of a Job.State: its constructor.
type JobStateTag uint8

// The constructors of Job.State, in declaration order.
const (
	JobQueued JobStateTag = iota
	JobRunning
	JobDone
	JobDead
)

// JobState is the stdlib `Job.State`: Queued | Running | Done | Dead.
type JobState struct{ Tag JobStateTag }

// JobStatusRecord is the stdlib `Job.Status` (fields as gen's Field names them).
type JobStatusRecord struct {
	State    JobState
	Attempts int64
	Error    rt.Maybe[string]
	RunAt    rt.Maybe[time.Time]
}

func init() {
	rt.RegisterUnion(reflect.TypeOf(JobStateTag(0)), []string{"Queued", "Running", "Done", "Dead"})
}

// JobEnqueue is Job.enqueue: j, due now, in the running Task's transaction (jobs.Enqueue). It is
// a write: it never runs inside a Query.
func JobEnqueue(j rt.Job) eff.Task[string] {
	return eff.Call("Job.enqueue", true, func(e *eff.Env) (string, error) { return jobs.Enqueue(e, j, now(e), nil) })
}

// JobEnqueueAt is Job.enqueueAt: j, not due before at.
func JobEnqueueAt(at time.Time, j rt.Job) eff.Task[string] {
	return eff.Call("Job.enqueueAt", true, func(e *eff.Env) (string, error) { return jobs.Enqueue(e, j, at, nil) })
}

// JobEnqueueOnce is Job.enqueueOnce: j, due now, unless a job of its kind with key is queued or
// running, whose id it gives instead.
func JobEnqueueOnce(key string, j rt.Job) eff.Task[string] {
	return eff.Call("Job.enqueueOnce", true, func(e *eff.Env) (string, error) { return jobs.Enqueue(e, j, now(e), &key) })
}

// JobStatus is Job.status: the status of job id in the store the Query is bound to, Nothing
// when it has no such row.
func JobStatus(id string) eff.Query[rt.Maybe[JobStatusRecord]] {
	return eff.Q(eff.Call("Job.status", false, func(e *eff.Env) (rt.Maybe[JobStatusRecord], error) {
		s, err := jobs.StatusOf(e, id)
		if err != nil || s == nil {
			return rt.Nothing[JobStatusRecord](), err
		}
		out := JobStatusRecord{Attempts: int64(s.Attempts), Error: rt.Nothing[string](), RunAt: rt.Nothing[time.Time]()}
		switch s.State {
		case jobs.Queued:
			out.State.Tag = JobQueued
		case jobs.Running:
			out.State.Tag = JobRunning
		case jobs.Done:
			out.State.Tag = JobDone
		default:
			out.State.Tag = JobDead
		}
		if s.Error != nil {
			out.Error = rt.Just(*s.Error)
		}
		if s.RunAt != nil {
			out.RunAt = rt.Just(*s.RunAt)
		}
		return rt.Just(out), nil
	}))
}

// JobParse reads a job's stored parameters as a JSON tree: a number without a fraction or an
// exponent is an Int, any other a Float; an object keeps its members in order.
func JobParse(s string) (rt.JsonValue, error) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	v, err := jobParse(d)
	if err != nil {
		return v, err
	}
	if _, err := d.Token(); err != io.EOF {
		return v, errors.New("text after the JSON value")
	}
	return v, nil
}

func jobParse(d *json.Decoder) (rt.JsonValue, error) {
	tok, err := d.Token()
	if err != nil {
		return rt.JsonValue{}, err
	}
	switch t := tok.(type) {
	case nil:
		return rt.JsonValue{}, nil
	case bool:
		return JsonBool(t), nil
	case string:
		return JsonString(t), nil
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return JsonInt(n), nil
		}
		f, err := t.Float64()
		return JsonFloat(f), err
	case json.Delim:
		if t == '[' {
			v := rt.JsonValue{Kind: rt.JsonArray}
			for d.More() {
				x, err := jobParse(d)
				if err != nil {
					return v, err
				}
				v.Items = append(v.Items, x)
			}
			_, err := d.Token()
			return v, err
		}
		v := rt.JsonValue{Kind: rt.JsonObject}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return v, err
			}
			x, err := jobParse(d)
			if err != nil {
				return v, err
			}
			v.Members = append(v.Members, rt.JsonMember{Key: k.(string), Value: x})
		}
		_, err := d.Token()
		return v, err
	}
	return rt.JsonValue{}, fmt.Errorf("unexpected %v", tok)
}

// JobEncTime is a Time as a job parameter: its milliseconds since the epoch.
func JobEncTime(t time.Time) rt.JsonValue { return JsonInt(t.UnixMilli()) }

// JobEncDate is a Date as a job parameter: "YYYY-MM-DD".
func JobEncDate(d rt.Date) rt.JsonValue { return JsonString(DateToText(d)) }

// JobEncList is a List as a job parameter: an array of its elements, each encoded by f.
func JobEncList[T any](xs []T, f func(T) rt.JsonValue) rt.JsonValue {
	v := rt.JsonValue{Kind: rt.JsonArray, Items: make([]rt.JsonValue, len(xs))}
	for i, x := range xs {
		v.Items[i] = f(x)
	}
	return v
}

// JobEncMaybe is a Maybe as a job parameter: null for Nothing, else the value encoded by f.
func JobEncMaybe[T any](m rt.Maybe[T], f func(T) rt.JsonValue) rt.JsonValue {
	if !m.Ok {
		return rt.JsonValue{}
	}
	return f(m.Val)
}

// JobEncTagged is a custom type's value as a job parameter: {"tag": tag, "args": [args…]}.
func JobEncTagged(tag string, args ...rt.JsonValue) rt.JsonValue {
	return rt.JsonValue{Kind: rt.JsonObject, Members: []rt.JsonMember{
		{Key: "tag", Value: JsonString(tag)},
		{Key: "args", Value: rt.JsonValue{Kind: rt.JsonArray, Items: append([]rt.JsonValue{}, args...)}},
	}}
}

// jobKinds names a JsonKind in a decode error.
var jobKinds = [...]string{rt.JsonNull: "null", rt.JsonBool: "a Bool", rt.JsonInt: "an Int", rt.JsonFloat: "a Float", rt.JsonString: "a Text", rt.JsonArray: "a list", rt.JsonObject: "an object"}

// jobWant is the decode error of v at path, which is not the want it should be.
func jobWant(path, want string, v rt.JsonValue) error {
	return fmt.Errorf("%s: expected %s, got %s", path, want, jobKinds[v.Kind])
}

// JobDecInt decodes an Int job parameter at path.
func JobDecInt(v rt.JsonValue, path string) (int64, error) {
	if v.Kind != rt.JsonInt {
		return 0, jobWant(path, "an Int", v)
	}
	return v.Int, nil
}

// JobDecFloat decodes a Float (an Int is read as a Float).
func JobDecFloat(v rt.JsonValue, path string) (float64, error) {
	switch v.Kind {
	case rt.JsonFloat:
		return v.Float, nil
	case rt.JsonInt:
		return float64(v.Int), nil
	}
	return 0, jobWant(path, "a Float", v)
}

// JobDecBool decodes a Bool.
func JobDecBool(v rt.JsonValue, path string) (bool, error) {
	if v.Kind != rt.JsonBool {
		return false, jobWant(path, "a Bool", v)
	}
	return v.Bool, nil
}

// JobDecText decodes a Text.
func JobDecText(v rt.JsonValue, path string) (string, error) {
	if v.Kind != rt.JsonString {
		return "", jobWant(path, "a Text", v)
	}
	return v.Text, nil
}

// JobDecTime decodes a Time from its milliseconds since the epoch.
func JobDecTime(v rt.JsonValue, path string) (time.Time, error) {
	if v.Kind != rt.JsonInt {
		return time.Time{}, jobWant(path, "a Time (milliseconds)", v)
	}
	return time.UnixMilli(v.Int).UTC(), nil
}

// JobDecDate decodes a Date from "YYYY-MM-DD".
func JobDecDate(v rt.JsonValue, path string) (rt.Date, error) {
	if v.Kind == rt.JsonString {
		if d := DateFromText(v.Text); d.Ok {
			return d.Val, nil
		}
	}
	return rt.Date{}, jobWant(path, "a Date", v)
}

// JobDecList decodes a List whose elements d decodes.
func JobDecList[T any](d func(rt.JsonValue, string) (T, error)) func(rt.JsonValue, string) ([]T, error) {
	return func(v rt.JsonValue, path string) ([]T, error) {
		if v.Kind != rt.JsonArray {
			return nil, jobWant(path, "a list", v)
		}
		out := make([]T, len(v.Items))
		for i, x := range v.Items {
			var err error
			if out[i], err = d(x, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
}

// JobDecMaybe decodes a Maybe: null is Nothing, anything else Just what d decodes.
func JobDecMaybe[T any](d func(rt.JsonValue, string) (T, error)) func(rt.JsonValue, string) (rt.Maybe[T], error) {
	return func(v rt.JsonValue, path string) (rt.Maybe[T], error) {
		if v.Kind == rt.JsonNull {
			return rt.Nothing[T](), nil
		}
		x, err := d(v, path)
		if err != nil {
			return rt.Nothing[T](), err
		}
		return rt.Just(x), nil
	}
}

// JobField decodes field key of the object v (at path) with d. A field the object lacks is an
// error naming it, a Maybe field too (the encoder writes Nothing as null): a field renamed
// since the row was written makes it undecodable, never silently Nothing.
func JobField[T any](v rt.JsonValue, path, key string, d func(rt.JsonValue, string) (T, error)) (T, error) {
	var zero T
	if v.Kind != rt.JsonObject {
		return zero, jobWant(path, "an object", v)
	}
	at := key
	if path != "" {
		at = path + "." + key
	}
	for _, m := range v.Members {
		if m.Key == key {
			return d(m.Value, at)
		}
	}
	return zero, fmt.Errorf("%s: missing", at)
}

// JobTagged reads a custom type's value at path: its constructor's name and arguments, of which
// it must have n.
func JobTagged(v rt.JsonValue, path string, n map[string]int) (string, []rt.JsonValue, error) {
	tag, err := JobField(v, path, "tag", JobDecText)
	if err != nil {
		return "", nil, err
	}
	want, ok := n[tag]
	if !ok {
		return "", nil, fmt.Errorf("%s: no constructor %q", path, tag)
	}
	var args []rt.JsonValue
	for _, m := range v.Members {
		if m.Key == "args" {
			if m.Value.Kind != rt.JsonArray {
				return "", nil, jobWant(path+".args", "a list", m.Value)
			}
			args = m.Value.Items
		}
	}
	if len(args) != want {
		return "", nil, fmt.Errorf("%s: %s takes %d arguments, got %d", path, tag, want, len(args))
	}
	return tag, args, nil
}
