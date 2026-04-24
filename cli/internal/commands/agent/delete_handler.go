package agent

import (
	"context"
	"fmt"

)

type DeleteInput struct {
	ID   string
	Hard bool
}

type DeleteOutput struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

func RunDelete(ctx context.Context, in DeleteInput) (*DeleteOutput, error) {
	_ = ctx
	return &DeleteOutput{ID: in.ID, Deleted: false}, fmt.Errorf("agent delete is not exposed by the HTTP bridge yet")
}
