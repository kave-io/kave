package flags

import "github.com/spf13/cobra"

type TimeRangeInput struct {
	From string
	To   string
}

type FileInput struct {
	Files []string
}

type ApplyInput struct {
	FileInput
	DryRun bool
	Prune  bool
	Wait   bool
}

type ResourceInput struct {
	Name        string
	Description string
	Status      string
}

type TargetInput struct {
	Agent  string
	Policy string
}

type TailInput struct {
	Follow bool
}

func AddTimeRangeFlags(cmd *cobra.Command, t *TimeRangeInput) {
	cmd.Flags().StringVar(&t.From, "from", "", "Start time or date")
	cmd.Flags().StringVar(&t.To, "to", "", "End time or date")
}

func AddFormatFlag(cmd *cobra.Command, format *string) {
	cmd.Flags().StringVar(format, "format", "json", "Output format (json, mermaid, dot, jsonl, otlp, parquet)")
}

func AddFileFlags(cmd *cobra.Command, f *FileInput) {
	cmd.Flags().StringArrayVarP(&f.Files, "file", "f", nil, "File path (repeatable; use - for stdin)")
}

func AddResourceFlags(cmd *cobra.Command, r *ResourceInput) {
	cmd.Flags().StringVar(&r.Name, "name", "", "Resource name")
	cmd.Flags().StringVar(&r.Description, "description", "", "Free-form description")
	cmd.Flags().StringVar(&r.Status, "status", "", "Resource status")
}

func AddApplyFlags(cmd *cobra.Command, a *ApplyInput) {
	AddFileFlags(cmd, &a.FileInput)
	cmd.Flags().BoolVar(&a.DryRun, "dry-run", false, "Do not mutate; print the diff")
	cmd.Flags().BoolVar(&a.Prune, "prune", false, "Delete resources missing from the file")
	cmd.Flags().BoolVar(&a.Wait, "wait", false, "Block until the new state is live")
}

func AddTailFlags(cmd *cobra.Command, t *TailInput) {
	cmd.Flags().BoolVar(&t.Follow, "follow", true, "Stream until interrupted")
}

func AddTargetFlags(cmd *cobra.Command, t *TargetInput) {
	cmd.Flags().StringVar(&t.Agent, "agent", "", "Agent name or ID")
	cmd.Flags().StringVar(&t.Policy, "policy", "", "Policy name or ID")
}
