package fp

func Map[T any, U any](s []T, mapperFunc func(T) U) []U {
	result := make([]U, len(s))

	for i := range s {
		result[i] = mapperFunc(s[i])
	}

	return result
}

// Keys returns all keys of m in unspecified order.
func Keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Values returns all values of m in unspecified order.
func Values[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// MapValues transforms the values of m using fn, returning a new map with the same keys.
func MapValues[K comparable, V any, W any](m map[K]V, fn func(V) W) map[K]W {
	out := make(map[K]W, len(m))
	for k, v := range m {
		out[k] = fn(v)
	}
	return out
}
