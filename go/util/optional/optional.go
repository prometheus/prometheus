package optional

type Optional[T any] struct {
	isSet bool
	value T
}

func (o *Optional[T]) IsNil() bool {
	return !o.isSet
}

func (o *Optional[T]) Value() T {
	return o.value
}

func (o *Optional[T]) RawValue() *T {
	if o.isSet {
		return &o.value
	}
	return nil
}

func (o *Optional[T]) Set(value T) {
	o.isSet = true
	o.value = value
}

func WithRawValue[T any](rawValue *T) Optional[T] {
	o := Optional[T]{}
	if rawValue != nil {
		o.isSet = true
		o.value = *rawValue
	}
	return o
}
