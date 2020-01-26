package service

import "context"

import "errors"

type Service struct {
}

func (s Service) Foo(ctx context.Context, i int) (int, error) {
	panic(errors.New("not implemented"))
}
