package rt

// Job is a job value (#168): Kind is the job's qualified name ("Signup.SendWelcome") and Params
// its parameters as JSON, encoded by the job's generated codec. The generated constructor builds
// it; Job.enqueue stores the pair and the worker looks Kind up to decode Params and run it. `==`,
// Show and Dict keys are those of the pair, as Route's are.
type Job struct{ Kind, Params string }
