package head

import "github.com/prometheus/prometheus/pp/go/relabeler"

type BuildFunc func() relabeler.UpgradableHeadInterface

func (fn BuildFunc) Build() relabeler.UpgradableHeadInterface {
	return fn()
}
