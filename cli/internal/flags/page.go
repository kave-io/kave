package flags

import "github.com/spf13/cobra"

type PageInput struct {
	Limit  int
	Cursor string
	All    bool
}

func AddPageFlags(cmd *cobra.Command, p *PageInput) {
	cmd.Flags().IntVar(&p.Limit, "limit", 20, "Page size")
	cmd.Flags().StringVar(&p.Cursor, "cursor", "", "Opaque pagination cursor")
	cmd.Flags().BoolVar(&p.All, "all", false, "Auto-iterate through all pages")
}
