package util

// FnWriter is a helper functional wrapper to create unusuall writers
type FnWriter func([]byte) (int, error)

// Write implements io.Writer interface
func (fn FnWriter) Write(p []byte) (int, error) {
	return fn(p)
}
