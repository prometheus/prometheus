package codecov

import (
	"dagger.io/dagger"
)

dagger.#Plan & {
	actions: test: #Image & {}
}
