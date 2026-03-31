// Package pointer provides generic helpers for working with pointer values.
// It eliminates the boilerplate of taking the address of literals and
// dereferencing nullable pointers safely.
package pointer

// To returns a pointer to v. Useful for taking the address of literals:
//
//	p := pointer.To("hello")  // *string
//	p := pointer.To(42)       // *int
func To[T any](v T) *T {
	return &v
}

// From returns *p if p is non-nil, otherwise the supplied default value.
//
//	name := pointer.From(user.NickName, "anonymous")
func From[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// FromZero returns *p if p is non-nil, otherwise the zero value of T.
//
//	count := pointer.FromZero(resp.Count) // 0 if nil
func FromZero[T any](p *T) T {
	if p == nil {
		var z T
		return z
	}
	return *p
}

// Coalesce returns the value of the first non-nil pointer in ptrs.
// Returns the zero value of T if all pointers are nil.
//
//	name := pointer.Coalesce(user.DisplayName, user.FullName, pointer.To("guest"))
func Coalesce[T any](ptrs ...*T) T {
	for _, p := range ptrs {
		if p != nil {
			return *p
		}
	}
	var z T
	return z
}

// Map applies fn to *p and returns a pointer to the result.
// Returns nil if p is nil.
//
//	upper := pointer.Map(user.Name, strings.ToUpper)
func Map[T any, U any](p *T, fn func(T) U) *U {
	if p == nil {
		return nil
	}
	u := fn(*p)
	return &u
}

// If returns a pointer to v when cond is true, otherwise nil.
// Useful for building optional fields in request/response structs.
//
//	pointer.If(user.IsAdmin, adminToken)
func If[T any](cond bool, v T) *T {
	if !cond {
		return nil
	}
	return &v
}

// Equal reports whether two pointers point to equal values.
// Two nil pointers are considered equal; a nil and a non-nil pointer are not.
func Equal[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
