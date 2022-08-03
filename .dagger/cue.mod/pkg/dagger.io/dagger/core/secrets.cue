package core

import "dagger.io/dagger"

// Decode the contents of a secrets without leaking it.
// Supported formats: json, yaml
#DecodeSecret: {
	$dagger: task: _name: "DecodeSecret"

	// A dagger.#Secret whose plain text is a JSON or YAML string
	input: dagger.#Secret

	format: "json" | "yaml"

	// A new secret or (map of secrets) derived from unmarshaling the input secret's plain text
	output: _#decodedOutput @dagger(generated)
}

_#decodedOutput: dagger.#Secret | *{[!~"\\$dagger"]: _#decodedOutput}

// Create a new a secret from a filesystem tree
#NewSecret: {
	$dagger: task: _name: "NewSecret"

	// Filesystem tree holding the secret
	input: dagger.#FS
	// Path of the secret to read
	path: string
	// Whether to trim leading and trailing space characters from secret value
	trimSpace: *true | false
	// Contents of the secret
	output: dagger.#Secret @dagger(generated)
}

// Trim leading and trailing space characters from a secret
#TrimSecret: {
	$dagger: task: _name: "TrimSecret"

	// Original secret
	input: dagger.#Secret

	// New trimmed secret
	output: dagger.#Secret @dagger(generated)
}
