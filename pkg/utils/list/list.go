package list

// Map returns a list constructed by appling a function f
// to all items in the list l.
func Map[T, V any](l []T, f func(T) V) []V {
	m := make([]V, 0, len(l))
	for _, e := range l {
		m = append(m, f(e))
	}
	return m
}
