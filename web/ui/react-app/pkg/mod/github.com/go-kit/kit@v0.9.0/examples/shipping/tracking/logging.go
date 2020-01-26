package tracking

import (
	"time"

	"github.com/go-kit/kit/log"
)

type loggingService struct {
	logger log.Logger
	Service
}

// NewLoggingService returns a new instance of a logging Service.
func NewLoggingService(logger log.Logger, s Service) Service {
	return &loggingService{logger, s}
}

func (s *loggingService) Track(id string) (c Cargo, err error) {
	defer func(begin time.Time) {
		s.logger.Log("method", "track", "tracking_id", id, "took", time.Since(begin), "err", err)
	}(time.Now())
	return s.Service.Track(id)
}
