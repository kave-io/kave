package main

import (
	"flag"
	"fmt"
	"os"
)

func runMain() {
	only := flag.String("only", "", "run only the named rule (e.g., 'B1-layer-direction')")
	updateOverrides := flag.Bool("update-overrides", false, "regenerate allowlist/overrides.txt from // allow: comments")
	verbose := flag.Bool("verbose", false, "print which rules ran")
	flag.Parse()

	if *updateOverrides {
		fmt.Println("update-overrides not yet implemented")
		os.Exit(2)
	}

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting working directory: %v\n", err)
		os.Exit(2)
	}

	violations := Run(LoadOptions{
		Root:    root,
		Only:    *only,
		Verbose: *verbose,
	})

	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Println(FormatViolation(v))
		}
		os.Exit(1)
	}

	os.Exit(0)
}

func main() {
	runMain()
}
