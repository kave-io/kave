package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Kave daemon",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := unixClient().Post("http://unix/stop", "text/plain", nil)
		if err != nil {
			fmt.Println("Kave daemon is not running.")
			os.Exit(1)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			fmt.Printf("Unexpected stop response: %s\n", resp.Status)
			os.Exit(1)
		}
		fmt.Println("Kave daemon stopped.")
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
