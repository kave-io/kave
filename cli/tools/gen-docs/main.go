package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kave-io/kave/cli/cmd"
)

func main() {
	dir := defaultDocsDir()
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := cmd.GenerateDocs(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func defaultDocsDir() string {
	candidates := []string{
		filepath.Join("docs", "src", "content", "docs", "cli", "reference"),
		filepath.Join("..", "docs", "src", "content", "docs", "cli", "reference"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Dir(candidate)); err == nil {
			return candidate
		}
	}

	return candidates[0]
}
