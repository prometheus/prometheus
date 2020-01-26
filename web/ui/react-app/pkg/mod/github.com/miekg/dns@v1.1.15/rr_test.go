package dns

// testRR is a helper that wraps a call to NewRR and panics if the error is non-nil.
func testRR(s string) RR {
	r, err := NewRR(s)
	if err != nil {
		panic(err)
	}

	return r
}
