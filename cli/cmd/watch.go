package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// spanRow mirrors server/core/store.SpanRow for JSON decoding.
type spanRow struct {
	ID           string  `json:"ID"`
	RunID        string  `json:"RunID"`
	ActionID     string  `json:"ActionID"`
	Name         string  `json:"Name"`
	StartedAt    int64   `json:"StartedAt"`
	EndedAt      *int64  `json:"EndedAt"`
	DurationMs   int64   `json:"DurationMs"`
	Error        *string `json:"Error"`
	InputTokens  *int    `json:"InputTokens"`
	OutputTokens *int    `json:"OutputTokens"`
	Model        *string `json:"Model"`
	CostUSD      *float64 `json:"CostUSD"`
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Stream live agent spans from the Kave server",
	Long: `Connects to the Kave server and streams new spans in real time.
Useful for observing agent activity as it happens.

Examples:
  kave watch
  kave watch --run <run-id>
  kave watch --server http://localhost:8080`,
	Run: runWatch,
}

func init() {
	watchCmd.Flags().String("server", "", "Kave server URL (default: http://localhost:8080)")
	watchCmd.Flags().String("run", "", "Filter spans by run ID")
	watchCmd.Flags().String("action", "", "Filter spans by action ID")
	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) {
	serverURL := cmd.Flag("server").Value.String()
	if serverURL == "" {
		serverURL = viper.GetString("server.url")
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	runID, _ := cmd.Flags().GetString("run")
	actionID, _ := cmd.Flags().GetString("action")

	// Build SSE URL
	endpoint := strings.TrimRight(serverURL, "/") + "/api/v1/spans/stream"
	params := []string{}
	if runID != "" {
		params = append(params, "run_id="+runID)
	}
	if actionID != "" {
		params = append(params, "action_id="+actionID)
	}
	if len(params) > 0 {
		endpoint += "?" + strings.Join(params, "&")
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{Timeout: 0} // no timeout for streaming
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot connect to %s: %v\n", serverURL, err)
		fmt.Fprintln(os.Stderr, "Is the server running? Try: kave start")
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "server returned %d\n", resp.StatusCode)
		os.Exit(1)
	}

	filter := ""
	if runID != "" {
		filter = fmt.Sprintf(" (run: %s)", runID[:min(8, len(runID))])
	}
	fmt.Printf("watching%s — press Ctrl+C to stop\n\n", filter)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" || strings.HasPrefix(line, ":") {
			continue // heartbeat or comment
		}

		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}

		if strings.HasPrefix(data, "event: error") {
			fmt.Fprintf(os.Stderr, "server error: %s\n", data)
			os.Exit(1)
		}

		var span spanRow
		if err := json.Unmarshal([]byte(data), &span); err != nil {
			fmt.Printf("? (unparseable span)\n")
			continue
		}

		printSpan(span)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stream error: %v\n", err)
		os.Exit(1)
	}
}

func printSpan(s spanRow) {
	ts := time.UnixMilli(s.StartedAt).Format("15:04:05.000")

	// Status indicator
	status := "✓"
	if s.Error != nil && *s.Error != "" {
		status = "✗"
	}

	// Duration
	dur := ""
	if s.DurationMs > 0 {
		dur = fmt.Sprintf(" %dms", s.DurationMs)
	}

	// Token / cost info
	tokens := ""
	if s.InputTokens != nil && s.OutputTokens != nil {
		tokens = fmt.Sprintf(" [%d→%d tok]", *s.InputTokens, *s.OutputTokens)
	}

	cost := ""
	if s.CostUSD != nil && *s.CostUSD > 0 {
		cost = fmt.Sprintf(" $%.6f", *s.CostUSD)
	}

	model := ""
	if s.Model != nil && *s.Model != "" {
		model = fmt.Sprintf(" (%s)", *s.Model)
	}

	name := s.Name
	if name == "" {
		name = s.ActionID
	}

	fmt.Printf("%s %s  %s%s%s%s%s\n", ts, status, name, dur, model, tokens, cost)

	if s.Error != nil && *s.Error != "" {
		fmt.Printf("         error: %s\n", *s.Error)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
