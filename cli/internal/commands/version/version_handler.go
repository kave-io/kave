package version

import (
	"context"
	"runtime/debug"
)

type VersionInput struct {
}

type VersionOutput struct {
	Data any `json:"data"`
}

func RunVersion(ctx context.Context, in VersionInput) (*VersionOutput, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return &VersionOutput{Data: map[string]any{"version": "unknown"}}, nil
	}
	out := map[string]any{
		"path":    info.Path,
		"version": info.Main.Version,
	}
	return &VersionOutput{Data: out}, nil
}
