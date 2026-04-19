package cmd

import (
	"strings"
	"testing"
)

func TestDocOutputPath(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"kave.md":                  "index.md",
		"kave_agent.md":            "agent/index.md",
		"kave_agent_list.md":       "agent/list.md",
		"kave_admin_store_get.md":  "admin/store/get.md",
		"kave_completion_zsh.md":   "completion/zsh.md",
		"kave_version_version.md":  "version/version.md",
		"./kave_agent_list.md":     "agent/list.md",
		"/tmp/kave_agent_list.md":  "agent/list.md",
		"nested/kave_agent_get.md": "agent/get.md",
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			got, err := docOutputPath(input)
			if err != nil {
				t.Fatalf("docOutputPath returned error: %v", err)
			}
			if got != want {
				t.Fatalf("docOutputPath(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestDocLinkHandler(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"kave.md":                 "/cli/reference/",
		"kave_agent.md":           "/cli/reference/agent/",
		"kave_agent_list.md":      "/cli/reference/agent/list/",
		"kave_admin_store_get.md": "/cli/reference/admin/store/get/",
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			got := docLinkHandler(input)
			if got != want {
				t.Fatalf("docLinkHandler(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestDocFrontmatter(t *testing.T) {
	t.Parallel()

	agentList := docFrontmatter("kave_agent_list.md")
	for _, want := range []string{
		"title: \"List Agents\"",
		"description: \"Lists agents with pagination.\"",
		"sidebar:",
		"label: \"List Agents\"",
	} {
		if !strings.Contains(agentList, want) {
			t.Fatalf("frontmatter missing %q:\n%s", want, agentList)
		}
	}

	root := docFrontmatter("kave.md")
	if !strings.Contains(root, "title: \"Kave\"") {
		t.Fatalf("root frontmatter missing title:\n%s", root)
	}
}
