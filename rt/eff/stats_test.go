package eff

import (
	"testing"
	"time"
)

// Stats counts a request's statements by kind and adds up their time.
func TestStats_Add(t *testing.T) {
	var s Stats
	s.Add(false, 3*time.Millisecond)
	s.Add(true, time.Millisecond)
	s.Add(true, 2*time.Millisecond)
	if s.Exec != 1 || s.Query != 2 || s.ExecDur != 3*time.Millisecond || s.QueryDur != 3*time.Millisecond {
		t.Errorf("%+v", s)
	}
}
