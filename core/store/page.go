package store

// Page is a cursor-based pagination request.
// Limit <= 0 is treated as an implementation-defined default. Implementations
// should also enforce an upper bound (typically 500).
// Cursor is opaque to callers; it is produced by a prior PageResult.NextCursor.
type Page struct {
	Limit  int
	Cursor string
}

// PageResult is a paginated slice of items plus a cursor for the next page.
// NextCursor is empty when no more pages remain.
type PageResult[T any] struct {
	Items      []T
	NextCursor string
}
