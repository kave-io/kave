package shared

import "strings"

// SplitSSEDataLines extracts JSON payload lines from an SSE response body.
func SplitSSEDataLines(body []byte) []string {
	lines := strings.Split(string(body), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		out = append(out, line)
	}
	return out
}
