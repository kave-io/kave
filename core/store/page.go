package store

import "strconv"

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

func pageLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func pageOffset(cursor string) int {
	return PageOffset(cursor)
}

// PageOffset decodes a pagination cursor into a row offset. Returns 0 for empty/invalid cursors.
func PageOffset(cursor string) int {
	if cursor == "" {
		return 0
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

// PageNextCursor encodes a row offset as a pagination cursor string.
func PageNextCursor(offset int) string {
	return strconv.Itoa(offset)
}

// Paginate slices a full item list using the page cursor as an offset.
func Paginate[T any](items []T, page Page) PageResult[T] {
	limit := pageLimit(page.Limit)
	offset := pageOffset(page.Cursor)
	if offset >= len(items) {
		return PageResult[T]{Items: []T{}}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	out := items[offset:end]
	result := PageResult[T]{Items: out}
	if end < len(items) {
		result.NextCursor = strconv.Itoa(end)
	}
	return result
}
