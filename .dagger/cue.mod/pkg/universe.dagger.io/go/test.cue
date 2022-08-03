package go

// Test a go package
#Test: {
	package: string | *"." @deprecated("use `packages` instead")

	// Packages to test
	packages: [...string] | *[package]

	#Container & {
		command: {
			name: "go"
			args: packages
			flags: {
				test: true
				"-v": true
			}
		}
	}
}
