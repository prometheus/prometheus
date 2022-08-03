package core

// A core action that does nothing
// Useful to work around bugs in the DAG resolver.
//  See for example https://github.com/dagger/dagger/issues/1789
#Nop: {
	$dagger: task: _name: "Nop"
	input:  _
	output: _ @dagger(generated)
}
