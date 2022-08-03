package core

import "dagger.io/dagger"

// Execute a command in a container
#Exec: {
	$dagger: task: _name: "Exec"

	// Container filesystem
	input: dagger.#FS

	// Transient filesystem mounts
	//   Key is an arbitrary name, for example "app source code"
	//   Value is mount configuration
	mounts: [name=string]: #Mount

	// Command to execute
	// Example: ["echo", "hello, world!"]
	args: [...string]

	// Environment variables
	env: [key=string]: string | dagger.#Secret

	// Working directory
	workdir: string | *"/"

	// User ID or name
	user: string | *"root:root"

	// If set, always execute even if the operation could be cached
	always: true | *false

	// Inject hostname resolution into the container
	// key is hostname, value is IP
	hosts: [hostname=string]: string

	// Modified filesystem
	output: dagger.#FS @dagger(generated)

	// Command exit code
	// Currently this field can only ever be zero.
	// If the command fails, DAG execution is immediately terminated.
	// FIXME: expand API to allow custom handling of failed commands
	exit: int & 0
}

// Start a command in a container asynchronously
#Start: {
	$dagger: task: _name: "Start"

	// Container filesystem
	input: dagger.#FS

	// Transient filesystem mounts
	//   Key is an arbitrary name, for example "app source code"
	//   Value is mount configuration
	mounts: [name=string]: #Mount

	// Command to execute
	// Example: ["echo", "hello, world!"]
	args: [...string]

	// Working directory
	workdir: string | *"/"

	// User ID or name
	user: string | *"root:root"

	// Inject hostname resolution into the container
	// key is hostname, value is IP
	hosts: [hostname=string]: string

	// Environment variables
	env: [key=string]: string

	_id: string | null @dagger(generated)
}

// Stop an asynchronous command created by #Start by sending SIGKILL. If
// the optional timeout is specified, the command will be given that duration
// to exit on its own before being sent SIGKILL.
#Stop: {
	$dagger: task: _name: "Stop"

	input: #Start

	// Time to wait for the command to exit on its own before sending SIGKILL.
	timeout: int64 | *0

	// Command exit code
	exit: uint8 @dagger(generated)
}

// A transient filesystem mount.
#Mount: {
	dest: string
	type: string
	{
		type:     "cache"
		contents: #CacheDir
	} | {
		type:     "tmp"
		contents: #TempDir
	} | {
		type:     "socket"
		contents: dagger.#Socket
	} | {
		type:     "fs"
		contents: dagger.#FS
		source?:  string
		ro?:      true | *false
	} | {
		type:     "secret"
		contents: dagger.#Secret
		uid:      int | *0
		gid:      int | *0
		mask:     int | *0o400
	} | {
		type:        "file"
		contents:    string
		permissions: *0o644 | int
	}
}

// A (best effort) persistent cache dir
#CacheDir: {
	id:          string
	concurrency: *"shared" | "private" | "locked"
}

// A temporary directory for command execution
#TempDir: {
	size: int64 | *0
}

// Send a signal to a command created by #Start. SIGKILL is not
// supported here. Instead, #Stop should be used to forcibly stop
// a command.
#SendSignal: {
	$dagger: task: _name: "SendSignal"

	input: #Start

	signal: or([ for name, signal in _signals {signal}])
}

_signals: {
	SIGHUP:    1
	SIGINT:    2
	SIGQUIT:   3
	SIGILL:    4
	SIGTRAP:   5
	SIGABRT:   6
	SIGBUS:    7
	SIGFPE:    8
	SIGUSR1:   10
	SIGSEGV:   11
	SIGUSR2:   12
	SIGPIPE:   13
	SIGALRM:   14
	SIGTERM:   15
	SIGTKFLT:  16
	SIGCHLD:   17
	SIGCONT:   18
	SIGSTOP:   19
	SIGTSTP:   20
	SIGTTIN:   21
	SIGTTOU:   22
	SIGURG:    23
	SIGXCPU:   24
	SIGXFSZ:   25
	SIGVTALRM: 26
	SIGPROF:   27
	SIGWINCH:  28
	SIGIO:     29
	SIGPWR:    30
	SIGSYS:    31
}

_signals
