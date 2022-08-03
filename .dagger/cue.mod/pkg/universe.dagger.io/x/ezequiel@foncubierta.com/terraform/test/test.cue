//Deprecated: in favor of universe.dagger.io/alpha package
package terraform

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/x/ezequiel@foncubierta.com/terraform"
)

dagger.#Plan & {
	actions: test: {
		tfSource: core.#Source & {
			path: "./data"
		}

		applyWorkflow: {
			init: terraform.#Init & {
				source: tfSource.output
			}

			validate: terraform.#Validate & {
				source: init.output
			}

			plan: terraform.#Plan & {
				source: validate.output
			}

			apply: terraform.#Apply & {
				source: plan.output
			}

			verify: #AssertFile & {
				input:    apply.output
				path:     "./out.txt"
				contents: "Hello, world!"
			}
		}

		destroyWorkflow: {
			destroy: terraform.#Destroy & {
				source: applyWorkflow.apply.output
			}

			// TODO assert out.txt doesn't exist
		}
	}
}

// Make an assertion on the contents of a file
#AssertFile: {
	input:    dagger.#FS
	path:     string
	contents: string

	_read: core.#ReadFile & {
		"input": input
		"path":  path
	}

	actual: _read.contents

	// Assertion
	contents: actual
}
