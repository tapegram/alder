package rt

// Job is a job value (#168): Kind is the job's qualified name ("Signup.SendWelcome") and Params
// its parameters as JSON, encoded by the job's generated codec. The generated constructor builds
// it; Job.enqueue stores the pair and the worker looks Kind up to decode Params and run it.
// Priority is the kind's `priority` option (#283), which the constructor sets and Job.enqueue
// writes to the row; it is a function of Kind, so `==` and Dict keys are still those of the pair,
// as Route's are, and Show prints the pair only.
type Job struct {
	Kind, Params string
	Priority     int
}

// AlderShow prints the job as the record of its kind and params, without its priority.
func (j Job) AlderShow() (string, bool) {
	return "{ kind: " + quote(j.Kind) + ", params: " + quote(j.Params) + " }", false
}
