package agent

import (
	"context"
	"fmt"

)

type RestoreInput struct {
	ID string
}

type RestoreOutput struct {
	ID      string `json:"id"`
	Restored bool  `json:"restored"`
}

func RunRestore(ctx context.Context, in RestoreInput) (*RestoreOutput, error) {
	_ = ctx
	return &RestoreOutput{ID: in.ID, Restored: false}, fmt.Errorf("agent restore is not exposed by the HTTP bridge yet")
}
