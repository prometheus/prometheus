package dotnet

// Test a dotnet package
#Test: {
	// Package to test
	package: *"." | string

	#Container & {
		command: {
			args: [package]
			flags: test: true
		}
	}
}
