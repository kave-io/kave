package cmd

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

const defaultDocsReferenceDir = "docs/src/content/docs/cli/reference"

func GenerateDocs(dir string) error {
	if dir == "" {
		dir = defaultDocsReferenceDir
	}

	targetDir := filepath.Clean(dir)
	tempDir, err := os.MkdirTemp("", "kave-cli-docs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := doc.GenMarkdownTreeCustom(rootCmd, tempDir, docFrontmatter, docLinkHandler); err != nil {
		return err
	}

	if err := os.RemoveAll(targetDir); err != nil {
		return err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	return filepath.WalkDir(tempDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(tempDir, path)
		if err != nil {
			return err
		}

		dstRel, err := docOutputPath(rel)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(targetDir, dstRel)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, 0o644)
	})
}

func docFrontmatter(filename string) string {
	cmd := docCommandForFile(filename)
	title := docTitleForCommand(cmd)
	description := docDescriptionForCommand(cmd)
	sidebarLabel := title

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("title: %q\n", title))
	buf.WriteString(fmt.Sprintf("description: %q\n", description))
	buf.WriteString("sidebar:\n")
	buf.WriteString(fmt.Sprintf("  label: %q\n", sidebarLabel))
	buf.WriteString("---\n\n")
	return buf.String()
}

func docLinkHandler(link string) string {
	rel, err := docOutputPath(link)
	if err != nil {
		return link
	}

	sitePath := filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
	sitePath = strings.TrimSuffix(sitePath, "/index")
	if sitePath == "index" {
		return "/cli/reference/"
	}
	return "/cli/reference/" + strings.TrimPrefix(sitePath, "/") + "/"
}

func docOutputPath(name string) (string, error) {
	base := strings.TrimSuffix(filepath.Base(name), ".md")
	if base == "" {
		return "", fmt.Errorf("invalid markdown filename: %q", name)
	}

	parts := strings.Split(base, "_")
	if len(parts) > 0 && parts[0] == "kave" {
		parts = parts[1:]
	}

	switch len(parts) {
	case 0:
		return "index.md", nil
	case 1:
		return filepath.Join(parts[0], "index.md"), nil
	default:
		return filepath.Join(filepath.Join(parts[:len(parts)-1]...), parts[len(parts)-1]+".md"), nil
	}
}

func docCommandForFile(filename string) *cobra.Command {
	commandPath := strings.ReplaceAll(strings.TrimSuffix(filepath.Base(filename), ".md"), "_", " ")
	for _, command := range indexCommands(rootCmd) {
		if command.CommandPath() == commandPath {
			return command
		}
	}
	return nil
}

func indexCommands(cmd *cobra.Command) []*cobra.Command {
	var commands []*cobra.Command
	var visit func(*cobra.Command)

	visit = func(current *cobra.Command) {
		commands = append(commands, current)
		for _, child := range current.Commands() {
			visit(child)
		}
	}

	visit(cmd)
	return commands
}

func docTitleForCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return "Kave"
	}
	if cmd.CommandPath() == "kave" {
		return "Kave"
	}
	if title := titleCase(cmd.Short); title != "" {
		return title
	}
	if name := strings.TrimPrefix(cmd.CommandPath(), "kave "); name != "" {
		return titleCase(strings.ReplaceAll(name, " ", " "))
	}
	return "Kave"
}

func docDescriptionForCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	if desc := strings.TrimSpace(cmd.Long); desc != "" {
		return desc
	}
	return strings.TrimSpace(cmd.Short)
}

func titleCase(value string) string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == '_' || r == '/'
	})
	if len(fields) == 0 {
		return ""
	}

	for i, field := range fields {
		fields[i] = strings.ToUpper(field[:1]) + strings.ToLower(field[1:])
	}
	return strings.Join(fields, " ")
}
