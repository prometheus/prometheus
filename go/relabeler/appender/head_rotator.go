package appender

import "github.com/prometheus/prometheus/pp/go/relabeler"

type Storage interface {
	Add(head relabeler.UpgradableHeadInterface)
}

type HeadBuilder interface {
	Build() (relabeler.UpgradableHeadInterface, error)
}

type HeadRotator struct {
	storage     Storage
	headBuilder HeadBuilder
}

func NewHeadRotator(storage Storage, headBuilder HeadBuilder) *HeadRotator {
	return &HeadRotator{
		storage:     storage,
		headBuilder: headBuilder,
	}
}

func (r *HeadRotator) Rotate(head relabeler.UpgradableHeadInterface) (relabeler.UpgradableHeadInterface, error) {
	rotatedHead, err := r.headBuilder.Build()
	if err != nil {
		return nil, err
	}

	r.storage.Add(head)
	return rotatedHead, nil
}
