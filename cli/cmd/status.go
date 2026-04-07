package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the Kave daemon",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := unixClient().Get("http://unix/status")
		if err != nil {
			fmt.Println("Kave daemon is not running. Try 'kave start'")
			os.Exit(1)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Daemon Status: %s\n", strings.TrimSpace(string(body)))
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
