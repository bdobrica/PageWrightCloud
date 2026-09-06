package codex

func newTestExecutor(binary, dir, key, url string) *Executor {
	e := NewExecutor(binary, dir, key, url)
	e.isolate = false // Unit fixtures only; installed tests use the real namespace.
	return e
}
