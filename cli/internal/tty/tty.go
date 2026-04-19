package tty

import "os"

func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	mode, err := f.Stat()
	if err != nil {
		return false
	}
	return (mode.Mode() & os.ModeCharDevice) != 0
}
