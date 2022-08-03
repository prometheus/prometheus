package core

// HTTP operations

// Raw buildkit API
//
// package llb // import "github.com/moby/buildkit/client/llb"
//
// func HTTP(url string, opts ...HTTPOption) State
//
// type HTTPOption interface {
//         SetHTTPOption(*HTTPInfo)
// }
// func Checksum(dgst digest.Digest) HTTPOption
// func Chmod(perm os.FileMode) HTTPOption
// func Chown(uid, gid int) HTTPOption
// func Filename(name string) HTTPOption

import "dagger.io/dagger"

// Fetch a file over HTTP
#HTTPFetch: {
	$dagger: task: _name: "HTTPFetch"

	// Source url
	// Example: https://www.dagger.io/index.html
	source: string

	// Destination path of the downloaded file
	// Example: "/downloads/index.html"
	dest: string

	// Optionally verify the file checksum
	// FIXME: what is the best format to encode checksum?
	checksum?: string

	// Optionally set file permissions on the downloaded file
	// FIXME: find a more developer-friendly way to input file permissions
	permissions?: int

	// Optionally set UID of the downloaded file
	uid?: int

	// Optionally set GID of the downloaded file
	gid?: int

	// New filesystem state containing the downloaded file
	output: dagger.#FS @dagger(generated)
}
