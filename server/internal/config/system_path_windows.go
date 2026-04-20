//go:build windows

package config

import (
	"os"
	"path/filepath"
)

func systemConfigPath() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "kave", "kave.yaml")
}
