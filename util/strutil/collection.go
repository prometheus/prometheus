package strutil

func MergeStrings(a, b []string) []string {
	maxl := len(a)
	if len(b) > len(a) {
		maxl = len(b)
	}
	res := make([]string, 0, maxl*10/9)

	for len(a) > 0 && len(b) > 0 {
		switch {
		case a[0] == b[0]:
			res = append(res, a[0])
			a, b = a[1:], b[1:]
		case a[0] < b[0]:
			res = append(res, a[0])
			a = a[1:]
		default:
			res = append(res, b[0])
			b = b[1:]
		}
	}

	// Append all remaining elements.
	res = append(res, a...)
	res = append(res, b...)
	return res
}
