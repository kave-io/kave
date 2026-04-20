//go:build !windows

package config

func systemConfigPath() string {
	return "/etc/kave/kave.yaml"
}
