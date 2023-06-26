package maps

func Merge[K comparable, T any](m, n map[K]T) map[K]T {
	r := make(map[K]T, len(m)+len(n))
	for k, v := range m {
		r[k] = v
	}
	for k, v := range n {
		r[k] = v
	}
	return r
}

func MapValues[K comparable, T, V any](m map[K]T, f func(T) V) map[K]V {
	r := make(map[K]V, len(m))
	for k, v := range m {
		r[k] = f(v)
	}
	return r
}
