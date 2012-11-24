package utility

type Set map[interface{}]bool

func (s Set) Add(v interface{}) {
	s[v] = true
}

func (s Set) Remove(v interface{}) {
	delete(s, v)
}

func (s Set) Elements() []interface{} {
	result := make([]interface{}, 0, len(s))

	for k, _ := range s {
		result = append(result, k)
	}

	return result
}

func (s Set) Has(v interface{}) bool {
	_, p := s[v]

	return p
}

func (s Set) Intersection(o Set) Set {
	result := make(Set)

	for k, _ := range s {
		if o.Has(k) {
			result[k] = true
		}
	}

	return result
}
