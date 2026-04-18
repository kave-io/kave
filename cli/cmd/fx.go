package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var fxCmd = &cobra.Command{
	Use:   "fx",
	Short: "Currency rates and conversion",
}

var fxRateCmd = &cobra.Command{
	Use:   "rate",
	Short: "Get one or more cached FX rates from the server",
	Run: func(cmd *cobra.Command, args []string) {
		serverURL := resolveServerURL(cmd)
		base, _ := cmd.Flags().GetString("base")
		quotes, _ := cmd.Flags().GetString("quotes")
		endpoint := fmt.Sprintf("%s/api/v1/fx/rates?base=%s&quotes=%s", strings.TrimRight(serverURL, "/"), url.QueryEscape(strings.ToUpper(base)), url.QueryEscape(strings.ToUpper(quotes)))
		resp, err := http.Get(endpoint)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		var payload struct {
			Rates []struct {
				BaseCurrency  string `json:"base_currency"`
				QuoteCurrency string `json:"quote_currency"`
				Rate          string `json:"rate"`
				AsOfDate      string `json:"as_of_date"`
				FetchedAt     int64  `json:"fetched_at"`
			} `json:"rates"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, rate := range payload.Rates {
			fmt.Printf("%s/%s = %s (as of %s)\n", rate.BaseCurrency, rate.QuoteCurrency, rate.Rate, rate.AsOfDate)
		}
	},
}

var fxConvertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert an amount using the server FX cache",
	Run: func(cmd *cobra.Command, args []string) {
		serverURL := resolveServerURL(cmd)
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		amount, _ := cmd.Flags().GetString("amount")
		endpoint := fmt.Sprintf("%s/api/v1/fx/convert?from=%s&to=%s&amount=%s", strings.TrimRight(serverURL, "/"), url.QueryEscape(strings.ToUpper(from)), url.QueryEscape(strings.ToUpper(to)), url.QueryEscape(amount))
		resp, err := http.Get(endpoint)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "server returned %d\n", resp.StatusCode)
			os.Exit(1)
		}
		var payload struct {
			Output struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"output"`
			Rate struct {
				Rate string `json:"rate"`
			} `json:"rate"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%s %s (rate %s)\n", payload.Output.Amount, payload.Output.Currency, payload.Rate.Rate)
	},
}

var fxRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Force an FX refresh from Frankfurter into the server cache",
	Run: func(cmd *cobra.Command, args []string) {
		serverURL := resolveServerURL(cmd)
		req, err := http.NewRequest(http.MethodPost, strings.TrimRight(serverURL, "/")+"/api/v1/fx/refresh", nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "server returned %d\n", resp.StatusCode)
			os.Exit(1)
		}
		fmt.Println("fx refresh completed")
	},
}

func init() {
	fxCmd.PersistentFlags().String("server", "", "Kave server URL (default: http://localhost:8080)")

	fxRateCmd.Flags().String("base", "USD", "Base currency")
	fxRateCmd.Flags().String("quotes", "EUR,IRR,IRT", "Comma-separated quote currencies")

	fxConvertCmd.Flags().String("from", "USD", "Source currency")
	fxConvertCmd.Flags().String("to", "IRT", "Target currency")
	fxConvertCmd.Flags().String("amount", "", "Amount to convert")
	_ = fxConvertCmd.MarkFlagRequired("amount")

	fxCmd.AddCommand(fxRateCmd, fxConvertCmd, fxRefreshCmd)
	rootCmd.AddCommand(fxCmd)
}

func resolveServerURL(cmd *cobra.Command) string {
	serverURL, _ := cmd.Flags().GetString("server")
	if serverURL == "" {
		serverURL = viper.GetString("server.url")
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	return serverURL
}
