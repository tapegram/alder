package jobs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// WorkersEnv is the environment variable of the number of job workers an instance runs.
const WorkersEnv = "ALDER_JOBS"

// DefaultWorkers is the number of job workers when WorkersEnv is unset or empty.
const DefaultWorkers = 4

// JobsOnlyWorkers is the number of job workers of `app --jobs-only` when WorkersEnv is unset or
// empty (spec §7a).
const JobsOnlyWorkers = 8

// WorkersFromEnv is WorkersEnv's number of job workers: DefaultWorkers when it is unset or
// empty, 0 for none; an error for a value that is not a whole number below 65536.
func WorkersFromEnv() (int, error) { return workersFromEnv(os.Getenv, DefaultWorkers) }

// JobsOnlyWorkersFromEnv is WorkersFromEnv with JobsOnlyWorkers for unset or empty: the
// generated main reads it under --jobs-only.
func JobsOnlyWorkersFromEnv() (int, error) { return workersFromEnv(os.Getenv, JobsOnlyWorkers) }

func workersFromEnv(getenv func(string) string, def int) (int, error) {
	v := strings.TrimSpace(getenv(WorkersEnv))
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseUint(v, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%s=%s: want a whole number of job workers below 65536 (0: none)", WorkersEnv, v)
	}
	return int(n), nil
}
