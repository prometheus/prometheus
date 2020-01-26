package service

import "context"

import "errors"

type Service struct {
}

func (s Service) Concat(ctx context.Context, a string, b string) (string, error) {
	panic(errors.New("not implemented"))
}
func (s Service) Count(ctx context.Context, string1 string) int {
	panic(errors.New("not implemented"))
}
