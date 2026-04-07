package cmd

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

func expandPath(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func apiSocketPath() string {
	path := expandPath(viper.GetString("api.socket"))
	if path == "" {
		path = expandPath("~/.kave/kave.sock")
	}
	return path
}

func unixClient() *http.Client {
	socketPath := apiSocketPath()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   2 * time.Second,
	}
}

func daemonRunning() bool {
	client := unixClient()
	resp, err := client.Get("http://unix/status")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}
