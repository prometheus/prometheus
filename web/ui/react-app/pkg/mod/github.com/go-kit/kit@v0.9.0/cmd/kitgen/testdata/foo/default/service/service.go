package service

import "context"

import "errors"

type FooService struct {
}

func (f FooService) Bar(ctx context.Context, i int, s string) (string, error) {
	panic(errors.New("not implemented"))
}
