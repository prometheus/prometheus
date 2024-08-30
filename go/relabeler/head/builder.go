package head

import "github.com/prometheus/prometheus/pp/go/relabeler"

type BuildFunc func() (relabeler.UpgradableHeadInterface, error)

func (fn BuildFunc) Build() (relabeler.UpgradableHeadInterface, error) {
	return fn()
}
