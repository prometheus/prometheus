package appender

import "github.com/prometheus/prometheus/pp/go/relabeler"

type Storage interface {
	Add(head relabeler.UpgradableHeadInterface)
}

type HeadBuilder interface {
	Build() relabeler.UpgradableHeadInterface
}

type HeadRotator struct {
	storage     Storage
	headBuilder HeadBuilder
}

func (r *HeadRotator) Rotate(head relabeler.UpgradableHeadInterface) relabeler.UpgradableHeadInterface {
	r.storage.Add(head)
	return r.headBuilder.Build()
}
