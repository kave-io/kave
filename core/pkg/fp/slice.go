package fp

// Filter returns a new slice containing only the elements for which keep returns true.
func Filter[T any](s []T, keep func(T) bool) []T {
	out := make([]T, 0, len(s))
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// Reduce folds s into a single value using accumulator acc and fn.
func Reduce[T any, A any](s []T, acc A, fn func(A, T) A) A {
	for _, v := range s {
		acc = fn(acc, v)
	}
	return acc
}

// Contains reports whether s contains at least one element for which match returns true.
func Contains[T any](s []T, match func(T) bool) bool {
	for _, v := range s {
		if match(v) {
			return true
		}
	}
	return false
}

// Find returns the first element for which match returns true and ok=true.
// Returns the zero value and ok=false if no element matches.
func Find[T any](s []T, match func(T) bool) (val T, found bool) {
	for _, v := range s {
		if match(v) {
			return v, true
		}
	}
	return val, false
}

// Unique returns a new slice with duplicate elements removed (order preserved).
// Two elements are considered equal when their key values are equal.
func Unique[T any, K comparable](s []T, key func(T) K) []T {
	seen := make(map[K]struct{}, len(s))
	out := make([]T, 0, len(s))
	for _, v := range s {
		k := key(v)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// GroupBy groups elements of s by the key returned by keyFn.
func GroupBy[T any, K comparable](s []T, keyFn func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, v := range s {
		k := keyFn(v)
		out[k] = append(out[k], v)
	}
	return out
}

// Chunk splits s into consecutive sub-slices of at most size elements.
func Chunk[T any](s []T, size int) [][]T {
	if size <= 0 {
		return nil
	}
	out := make([][]T, 0, (len(s)+size-1)/size)
	for len(s) > 0 {
		if len(s) < size {
			size = len(s)
		}
		out = append(out, s[:size])
		s = s[size:]
	}
	return out
}
