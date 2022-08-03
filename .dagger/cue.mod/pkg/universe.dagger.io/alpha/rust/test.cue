package rust

// Test a rust package
#Test: {
	#Container & {
		command: flags: test: true
	}
}
