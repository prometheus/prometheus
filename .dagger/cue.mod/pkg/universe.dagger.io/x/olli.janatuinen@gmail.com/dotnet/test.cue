//Deprecated: in favor of universe.dagger.io/alpha package
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
