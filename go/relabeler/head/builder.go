package head

import "github.com/prometheus/prometheus/pp/go/relabeler"

type BuildFunc func() (relabeler.Head, error)

func (fn BuildFunc) Build() (relabeler.Head, error) {
	return fn()
}
