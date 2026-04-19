package token

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type IssueInput struct {
	Agent string
}

type IssueOutput struct {
	TokenID string `json:"token_id"`
	Token   string `json:"token"`
}

func RunIssue(ctx context.Context, in IssueInput) (*IssueOutput, error) {
	return nil, output.NotImplemented("agent token issue")
}
