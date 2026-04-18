package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runEvent mirrors core/bus.RunEvent for JSON decoding.
type runEvent struct {
	RunID     string `json:"RunID"`
	ProjectID string `json:"ProjectID"`
	EnvID     string `json:"EnvID"`
	AgentID   string `json:"AgentID"`
	Status    string `json:"Status"`
	SpanID    string `json:"SpanID"`
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
	watchCmd.Flags().String("run", "", "Filter runs by run ID")
	watchCmd.Flags().String("project", "", "Filter runs by project ID")
	watchCmd.Flags().String("env", "", "Filter runs by environment ID")
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
	projectID, _ := cmd.Flags().GetString("project")
	envID, _ := cmd.Flags().GetString("env")

	// Build SSE URL
	endpoint := strings.TrimRight(serverURL, "/") + "/api/v1/spans/stream"
	params := []string{}
	if runID != "" {
		params = append(params, "run_id="+runID)
	}
	if projectID != "" {
		params = append(params, "project_id="+projectID)
	}
	if envID != "" {
		params = append(params, "env_id="+envID)
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

	filters := []string{}
	if runID != "" {
		filters = append(filters, fmt.Sprintf("run: %s", runID[:min(8, len(runID))]))
	}
	if projectID != "" {
		filters = append(filters, fmt.Sprintf("project: %s", projectID[:min(8, len(projectID))]))
	}
	if envID != "" {
		filters = append(filters, fmt.Sprintf("env: %s", envID[:min(8, len(envID))]))
	}
	filterStr := ""
	if len(filters) > 0 {
		filterStr = " (" + strings.Join(filters, ", ") + ")"
	}
	fmt.Printf("watching runs%s — press Ctrl+C to stop\n\n", filterStr)

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

		var event runEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			fmt.Printf("? (unparseable event)\n")
			continue
		}

		printRunEvent(event)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stream error: %v\n", err)
		os.Exit(1)
	}
}

func printRunEvent(e runEvent) {
	// Status indicator
	status := "→"
	if e.Status == "completed" {
		status = "✓"
	} else if e.Status == "failed" {
		status = "✗"
	}

	// Format run ID
	runIDShort := e.RunID
	if len(runIDShort) > 8 {
		runIDShort = runIDShort[:8]
	}

	fmt.Printf("%s  %s  agent:%s  span:%s\n", status, runIDShort, e.AgentID[:min(8, len(e.AgentID))], e.SpanID[:min(8, len(e.SpanID))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
