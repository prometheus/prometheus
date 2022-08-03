package dagger

// A reference to a filesystem tree.
// For example:
//  - The root filesystem of a container
//  - A source code repository
//  - A directory containing binary artifacts
// Rule of thumb: if it fits in a tar archive, it fits in a #FS.
#FS: {
	$dagger: fs: _id: string | null
}

// A reference to an external secret, for example:
//  - A password
//  - A SSH private key
//  - An API token
// Secrets are never merged in the Cue tree. They can only be used
// by a special filesystem mount designed to minimize leak risk.
#Secret: {
	$dagger: secret: _id: string
}

// A reference to a network socket, for example:
//  - A UNIX socket
//  - A TCP or UDP port
//  - A Windows named pipe
#Socket: {
	$dagger: socket: _id: string
}
