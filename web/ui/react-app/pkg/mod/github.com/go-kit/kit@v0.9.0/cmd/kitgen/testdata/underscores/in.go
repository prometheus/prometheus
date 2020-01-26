package underscores

import "context"

type Service interface {
	Foo(_ context.Context, _ int) (int, error)
}
