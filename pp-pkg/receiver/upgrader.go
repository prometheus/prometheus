package receiver

import "github.com/prometheus/prometheus/pp/go/relabeler"

type DistributorConfigureFunc func(distributor relabeler.Distributor) error

func (fn DistributorConfigureFunc) Configure(distributor relabeler.Distributor) error {
	return fn(distributor)
}

type HeadConfigureFunc func(head relabeler.Head) error

func (fn HeadConfigureFunc) Configure(head relabeler.Head) error {
	return fn(head)
}
