package runtime

import (
	"testing"

	"github.com/kave-io/kave/core/store"
)

func TestPageFromProtoPagination(t *testing.T) {
	items := []int{1, 2, 3}
	tests := []struct {
		name       string
		limit      int32
		cursor     string
		wantItems  []int
		wantCursor string
	}{
		{
			name:       "first page",
			limit:      2,
			wantItems:  []int{1, 2},
			wantCursor: "2",
		},
		{
			name:       "second page",
			limit:      2,
			cursor:     "2",
			wantItems:  []int{3},
			wantCursor: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.Paginate(items, pageFromProto(tt.limit, tt.cursor))
			if got := result.Items; len(got) != len(tt.wantItems) {
				t.Fatalf("items length = %d, want %d", len(got), len(tt.wantItems))
			}
			for i, want := range tt.wantItems {
				if result.Items[i] != want {
					t.Fatalf("items[%d] = %d, want %d", i, result.Items[i], want)
				}
			}
			if result.NextCursor != tt.wantCursor {
				t.Fatalf("next cursor = %q, want %q", result.NextCursor, tt.wantCursor)
			}
		})
	}
}
