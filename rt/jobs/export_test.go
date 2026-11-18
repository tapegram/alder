package jobs

// SetNewID replaces the job id source for a test; the returned func restores it.
func SetNewID(f func() string) (restore func()) {
	old := newID
	newID = f
	return func() { newID = old }
}
