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
	return nil, &output.CommandError{Code: "command.unavailable", Message: "agent token issue is not exposed by the HTTP bridge yet", Exit: 1}
}
