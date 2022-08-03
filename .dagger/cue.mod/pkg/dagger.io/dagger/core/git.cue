package core

import "dagger.io/dagger"

// Push a directory to a git remote
#GitPush: {
	@dagger(notimplemented)
	$dagger: task: _name: "GitPush"

	input:  dagger.#FS
	remote: string
	ref:    string
}

// Pull a directory from a git remote
// Warning: do NOT embed credentials in the remote url as this will expose them in logs.
// By using username and password Dagger will handle this for you in a secure manner.
#GitPull: {
	$dagger: task: _name: "GitPull"
	remote:     string
	ref:        string
	keepGitDir: true | *false
	auth?:      {
		username: string
		password: dagger.#Secret // can be password or personal access token
	} | {
		authToken: dagger.#Secret
	} | {
		authHeader: dagger.#Secret
	}
	output: dagger.#FS @dagger(generated)
}
