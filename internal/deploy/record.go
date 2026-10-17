package deploy

import (
	"encoding/json"
	"time"
)

// Record is what a deploy writes about a build: alder-deploy.json in the artifact (before it
// ships) and builds/<id>/deployed.json at the target (after the switch, with When, By and what
// the ship did). It carries names only, never a value of the environment: the `requires` list
// is the variables' names, and the backup URL is the location the declaration gives (E0030
// refuses one with credentials in it).
type Record struct {
	Build    string     `json:"build"`
	App      string     `json:"app"`
	Env      string     `json:"env"`
	Target   string     `json:"target"`
	Version  string     `json:"version"`
	Commit   string     `json:"commit,omitempty"`
	Platform string     `json:"platform"`
	Gate     GateResult `json:"gate"`
	Plan     Plan       `json:"plan"`
	Requires []string   `json:"requires"`

	// AllowDataLoss and UnsafeSkipTests are the command-line flags the deploy ran with.
	AllowDataLoss   bool `json:"allowDataLoss,omitempty"`
	UnsafeSkipTests bool `json:"unsafeSkipTests,omitempty"`

	// BackupURL is the declaration's `backup` location (ALDER_BACKUP_URL at the target).
	BackupURL string `json:"backupURL,omitempty"`

	// Set at the target, after the switch.
	When     *time.Time `json:"when,omitempty"`
	By       string     `json:"by,omitempty"`     // "<ssh user> (local <USER>)"
	Backup   string     `json:"backup,omitempty"` // the pre-migration copy: "builds/<id>/backup.db"
	Applied  []string   `json:"applied,omitempty"`
	Previous string     `json:"previous,omitempty"` // the build that served before
	Port     int        `json:"port,omitempty"`
}

// JSON is the record as written to a file: indented, ending in a newline.
func (r Record) JSON() []byte {
	b, _ := json.MarshalIndent(r, "", "  ")
	return append(b, '\n')
}

// GateResult is what the gate ran and found.
type GateResult struct {
	Check    string        `json:"check"` // "ok"
	Warnings int           `json:"warnings,omitempty"`
	Build    string        `json:"build"` // "ok"
	Tests    *TestResult   `json:"tests,omitempty"`
	Mutate   *MutateResult `json:"mutate,omitempty"`
	Checks   []CheckResult `json:"checks,omitempty"`
	// Skipped: --unsafe-skip-tests skipped the tests, the mutation score and the checks.
	Skipped bool `json:"skipped,omitempty"`
}

// TestResult is `alder test`'s count.
type TestResult struct {
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

// MutateResult is `alder test --mutate`'s score against the floor.
type MutateResult struct {
	Score     float64 `json:"score"`
	Floor     float64 `json:"floor"`
	FloorFrom string  `json:"floorFrom"` // "declared", "last deploy" or "none yet"
	Mutants   int     `json:"mutants"`
	Survived  int     `json:"survived"`
}

// CheckResult is one declared check's run.
type CheckResult struct {
	Command string `json:"command"`
	Exit    int    `json:"exit"`
}
