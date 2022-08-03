//Deprecated: in favor of universe.dagger.io/alpha package
package rust

// Test a rust package
#Test: {
	#Container & {
		command: flags: test: true
	}
}
