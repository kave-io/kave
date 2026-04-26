package flags

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

type PageInput struct {
	Limit  int
	Cursor string
	All    bool
}

func AddPageFlags(cmd *cobra.Command, p *PageInput) {
	cmd.Flags().IntVar(&p.Limit, "limit", 20, "Page size")
	cmd.Flags().StringVar(&p.Cursor, "cursor", "", "Opaque pagination cursor")
	cmd.Flags().BoolVar(&p.All, "all", false, "Auto-iterate through all pages")
}

// PaginateAll iterates through paginated list responses, accumulating items.
// It calls the runOnce function with the cursor, starting with initialCursor.
// When all=false, it returns the first page only. When all=true, it loops until
// nextCursor is empty, capping iterations at maxPageCount to avoid runaway loops.
func PaginateAll[T any](ctx context.Context, all bool, initialCursor string, maxPageCount int, runOnce func(cursor string) (items []T, nextCursor string, err error)) ([]T, string, error) {
	if maxPageCount <= 0 {
		maxPageCount = 1000
	}

	var allItems []T
	cursor := initialCursor
	pageCount := 0

	for {
		items, nextCursor, err := runOnce(cursor)
		if err != nil {
			return nil, "", err
		}

		allItems = append(allItems, items...)
		cursor = nextCursor

		if !all || cursor == "" {
			// Return the next cursor for the CLI to display pagination info
			return allItems, cursor, nil
		}

		pageCount++
		if pageCount >= maxPageCount {
			return nil, "", fmt.Errorf("pagination exceeded maximum page count of %d; possible server misconfiguration", maxPageCount)
		}
	}
}
