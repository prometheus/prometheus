//Deprecated: in favor of universe.dagger.io/alpha package
package kubernetes

// Execute `kubectl apply` in a container
// See `_#base` in `./base.cue` for spec details
#Apply: {
	_#base & {
		action: "apply"
	}
}
