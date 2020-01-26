package foo

type Service interface {
	Concat(ctx context.Context, a, b string) (string, error)
	Count(ctx context.Context, s string) (count int)
}
