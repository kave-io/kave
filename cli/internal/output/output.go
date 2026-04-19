package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	yaml "go.yaml.in/yaml/v3"
)

const SchemaVersion = 1

type Format string

const (
	FormatAuto  Format = "auto"
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

func ParseFormat(raw string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(FormatAuto):
		return FormatAuto, nil
	case string(FormatTable):
		return FormatTable, nil
	case string(FormatJSON):
		return FormatJSON, nil
	case string(FormatYAML):
		return FormatYAML, nil
	default:
		return "", fmt.Errorf("unsupported output format %q", raw)
	}
}

func ResolveFormat(requested Format, stdout *os.File) Format {
	if requested != FormatAuto {
		return requested
	}
	if isTerminal(stdout) {
		return FormatTable
	}
	return FormatJSON
}

func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

type Envelope struct {
	SchemaVersion int       `json:"schema_version" yaml:"schema_version"`
	Kind          string    `json:"kind" yaml:"kind"`
	Data          any       `json:"data" yaml:"data"`
	Page          *Page     `json:"page" yaml:"page"`
	Warnings      []Warning `json:"warnings" yaml:"warnings"`
}

type Page struct {
	NextCursor any `json:"next_cursor" yaml:"next_cursor"`
	Limit      int `json:"limit" yaml:"limit"`
}

type Warning struct {
	Code    string `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`
}

type ErrorPayload struct {
	Code    string `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`
	Details any    `json:"details,omitempty" yaml:"details,omitempty"`
}

type ErrorEnvelope struct {
	SchemaVersion int          `json:"schema_version" yaml:"schema_version"`
	Kind          string       `json:"kind" yaml:"kind"`
	Error         ErrorPayload `json:"error" yaml:"error"`
}

type CommandError struct {
	Code    string
	Message string
	Details any
	Exit    int
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func (e *CommandError) ExitCode() int {
	if e == nil || e.Exit == 0 {
		return 1
	}
	return e.Exit
}

func NotImplemented(command string) *CommandError {
	return &CommandError{
		Code:    "command.not_implemented",
		Message: fmt.Sprintf("%s is not implemented yet", command),
		Details: map[string]any{"command": command},
		Exit:    1,
	}
}

func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func WriteYAML(w io.Writer, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func WriteTable(w io.Writer, message string) error {
	_, err := fmt.Fprintln(w, message)
	return err
}

func WriteError(w io.Writer, format Format, err error) error {
	var commandErr *CommandError
	if errors.As(err, &commandErr) {
		switch format {
		case FormatJSON:
			return WriteJSON(w, ErrorEnvelope{
				SchemaVersion: SchemaVersion,
				Kind:          "Error",
				Error: ErrorPayload{
					Code:    commandErr.Code,
					Message: commandErr.Message,
					Details: commandErr.Details,
				},
			})
		case FormatYAML:
			return WriteYAML(w, ErrorEnvelope{
				SchemaVersion: SchemaVersion,
				Kind:          "Error",
				Error: ErrorPayload{
					Code:    commandErr.Code,
					Message: commandErr.Message,
					Details: commandErr.Details,
				},
			})
		default:
			return WriteTable(w, commandErr.Message)
		}
	}

	switch format {
	case FormatJSON:
		return WriteJSON(w, ErrorEnvelope{
			SchemaVersion: SchemaVersion,
			Kind:          "Error",
			Error: ErrorPayload{
				Code:    "command.failed",
				Message: err.Error(),
			},
		})
	case FormatYAML:
		return WriteYAML(w, ErrorEnvelope{
			SchemaVersion: SchemaVersion,
			Kind:          "Error",
			Error: ErrorPayload{
				Code:    "command.failed",
				Message: err.Error(),
			},
		})
	default:
		return WriteTable(w, err.Error())
	}
}

// Render dispatches output based on command flags and TTY detection.
// Wraps data in an Envelope for structured output.
func Render(cmd *cobra.Command, data any, kind string) error {
	formatStr := "auto"
	if format := cmd.Flags().Lookup("output"); format != nil {
		formatStr = format.Value.String()
	}
	format, err := ParseFormat(formatStr)
	if err != nil {
		return err
	}
	format = ResolveFormat(format, os.Stdout)

	envelope := Envelope{
		SchemaVersion: SchemaVersion,
		Kind:          kind,
		Data:          data,
		Warnings:      []Warning{},
	}

	switch format {
	case FormatJSON:
		return WriteJSON(os.Stdout, envelope)
	case FormatYAML:
		return WriteYAML(os.Stdout, envelope)
	default:
		// For table output, format data as text
		// Handlers can customize this by implementing a String() method
		return WriteTable(os.Stdout, fmt.Sprintf("%v", data))
	}
}
