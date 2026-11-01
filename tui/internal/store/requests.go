package store

import "alder/tui/internal/feed"

// Request is one answered request: its access record and the seq of the event that carried it.
type Request struct {
	feed.Record
	Seq int64
}

// Requests is the table of answered requests, oldest first, newest last. The access records at
// debug level (quiet static and dev hits) are left out.
type Requests struct {
	ring ring[Request]
}

// NewRequests is an empty table keeping the last max requests.
func NewRequests(max int) *Requests { return &Requests{ring: newRing[Request](max)} }

// Add appends the request a log event carries; any other event is ignored.
func (q *Requests) Add(e feed.Event) {
	if e.Type != "log" || e.Log == nil || e.Log.Record == nil {
		return
	}
	r := e.Log.Record
	if !r.HasAccess || r.Level == "DEBUG" {
		return
	}
	q.ring.add(Request{Record: *r, Seq: e.Seq})
}

// All is the requests, oldest first.
func (q *Requests) All() []Request {
	out := make([]Request, 0, q.ring.n)
	q.ring.each(func(r Request) { out = append(out, r) })
	return out
}

// Len is the number of requests in the table.
func (q *Requests) Len() int { return q.ring.n }

// At is the i-th request, oldest first; i is in [0, Len()).
func (q *Requests) At(i int) Request { return q.ring.at(i) }

// ByRid is the newest request with the id rid.
func (q *Requests) ByRid(rid string) (Request, bool) {
	for i := q.ring.n - 1; i >= 0; i-- {
		if r := q.ring.at(i); r.Rid == rid {
			return r, true
		}
	}
	return Request{}, false
}

// Reset empties the table.
func (q *Requests) Reset() { q.ring.reset() }
