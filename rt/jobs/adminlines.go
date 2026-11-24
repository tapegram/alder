package jobs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AdminLine is one JSON line of the app binary's --jobs task (jobs.Admin), which `alder jobs
// --env` reads back with DecodeAdmin. Every field but Silo is omitted when empty; a reader
// ignores the fields it does not know.
type AdminLine struct {
	Silo string `json:"silo"`
	// list
	Counts map[State]int `json:"counts,omitempty"`
	Ticks  []Row         `json:"ticks,omitempty"`
	Dead   []Row         `json:"dead,omitempty"`
	Error  string        `json:"error,omitempty"`
	// retry
	Row         *Row   `json:"row,omitempty"`
	Maintenance string `json:"maintenance,omitempty"`
	// Refused names the silos the task did not act on.
	Refused []string `json:"refused,omitempty"`
	// run
	ID   string `json:"id,omitempty"`
	Kind string `json:"kind,omitempty"`
}

// reportFields are the fields one of which makes a line a report: Silo alone proves nothing.
var reportFields = []string{"counts", "error", "row", "refused", "id"}

// DecodeAdmin splits the output of a --jobs run into its report lines and the other lines
// (the app's log, a warning), which are messages, in order.
func DecodeAdmin(out string) (lines []AdminLine, messages []string) {
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var al AdminLine
		if isReport(l) && json.Unmarshal([]byte(l), &al) == nil {
			lines = append(lines, al)
			continue
		}
		messages = append(messages, l)
	}
	return lines, messages
}

// isReport is whether l is a JSON object with one of the report fields.
func isReport(l string) bool {
	if !strings.HasPrefix(l, "{") {
		return false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(l), &m) != nil {
		return false
	}
	for _, f := range reportFields {
		if _, ok := m[f]; ok {
			return true
		}
	}
	return false
}

// ResolveKind is the kind a name from the command line means: name is a kind's qualified name
// or its part after the last ".". Only a job without parameters can be run from the command
// line.
func ResolveKind(kinds []Kind, name string) (Kind, error) {
	var match []Kind
	var all []string
	for _, k := range kinds {
		all = append(all, k.Name)
		if name == k.Name || name == k.Name[strings.LastIndex(k.Name, ".")+1:] {
			match = append(match, k)
		}
	}
	switch {
	case len(match) == 0 && len(all) == 0:
		return Kind{}, fmt.Errorf("the app has no job %s: it declares no job", name)
	case len(match) == 0:
		return Kind{}, fmt.Errorf("the app has no job %s; its jobs: %s", name, strings.Join(all, ", "))
	case len(match) > 1:
		var names []string
		for _, k := range match {
			names = append(names, k.Name)
		}
		return Kind{}, fmt.Errorf("%s names several jobs: %s; name one with its module", name, strings.Join(names, ", "))
	}
	k := match[0]
	if len(k.Params) > 0 {
		return Kind{}, fmt.Errorf("%s takes parameters (%s): only a job without parameters is run from here; queue it from the app with Job.enqueue", k.Name, strings.Join(k.Params, ", "))
	}
	return k, nil
}
