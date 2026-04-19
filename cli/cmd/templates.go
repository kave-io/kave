package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

const helpTemplate = `NAME
  {{.CommandPath}}{{if .Short}} — {{.Short}}{{end}}

USAGE
  {{.UseLine}}

DESCRIPTION
  {{if .Long}}{{.Long}}{{else}}{{.Short}}{{end}}

FLAGS
{{flagUsagesOrNone .}}

EXAMPLES
{{exampleOrNone .}}

SEE ALSO
{{seeAlsoOrNone .}}
`

func configureTemplates() {
	cobra.AddTemplateFunc("flagUsagesOrNone", flagUsagesOrNone)
	cobra.AddTemplateFunc("exampleOrNone", exampleOrNone)
	cobra.AddTemplateFunc("seeAlsoOrNone", seeAlsoOrNone)
	rootCmd.SetHelpTemplate(helpTemplate)
}

func flagUsagesOrNone(cmd *cobra.Command) string {
	if cmd == nil {
		return "  (none)"
	}
	usage := strings.TrimSpace(cmd.Flags().FlagUsagesWrapped(28))
	if usage == "" {
		return "  (none)"
	}
	return indentLines(usage, "  ")
}

func exampleOrNone(cmd *cobra.Command) string {
	if cmd == nil || strings.TrimSpace(cmd.Example) == "" {
		return "  (none)"
	}
	return indentLines(strings.TrimSpace(cmd.Example), "  ")
}

func seeAlsoOrNone(cmd *cobra.Command) string {
	if cmd == nil {
		return "  (none)"
	}
	seeAlso := strings.TrimSpace(cmd.Annotations["kave.see_also"])
	if seeAlso == "" {
		return "  (none)"
	}
	return "  " + seeAlso
}

func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
