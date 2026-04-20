package config

import (
	"fmt"
	"strings"
)

type ExpandError struct {
	Path   string
	Line   int
	Reason string
}

func (e *ExpandError) Error() string {
	if e == nil {
		return "config expansion failed"
	}
	if e.Path == "" {
		return fmt.Sprintf("expand line %d: %s", e.Line, e.Reason)
	}
	return fmt.Sprintf("expand %s:%d: %s", e.Path, e.Line, e.Reason)
}

func Expand(text, path string, env map[string]string) (string, error) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		expanded, err := expandLine(line, path, i+1, env)
		if err != nil {
			return "", err
		}
		lines[i] = expanded
	}
	return strings.Join(lines, "\n"), nil
}

func expandLine(line, path string, lineNo int, env map[string]string) (string, error) {
	var b strings.Builder
	b.Grow(len(line))
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch != '$' {
			b.WriteByte(ch)
			continue
		}
		if i+1 >= len(line) {
			b.WriteByte(ch)
			continue
		}
		next := line[i+1]
		switch next {
		case '$':
			b.WriteByte('$')
			i++
		case '{':
			end := strings.IndexByte(line[i+2:], '}')
			if end < 0 {
				b.WriteByte(ch)
				continue
			}
			expr := line[i+2 : i+2+end]
			value, err := expandExpression(expr, path, lineNo, env)
			if err != nil {
				return "", err
			}
			b.WriteString(value)
			i += end + 2
		default:
			b.WriteByte(ch)
		}
	}
	return b.String(), nil
}

func expandExpression(expr, path string, lineNo int, env map[string]string) (string, error) {
	name := expr
	op := ""
	arg := ""
	if idx := strings.Index(expr, ":-"); idx >= 0 {
		name = expr[:idx]
		op = ":-"
		arg = expr[idx+2:]
	} else if idx := strings.Index(expr, ":?"); idx >= 0 {
		name = expr[:idx]
		op = ":?"
		arg = expr[idx+2:]
	}

	if name == "" {
		return "", &ExpandError{Path: path, Line: lineNo, Reason: "empty expansion name"}
	}
	value, ok := env[name]
	switch op {
	case ":-":
		if ok && value != "" {
			return value, nil
		}
		return arg, nil
	case ":?":
		if ok && value != "" {
			return value, nil
		}
		if arg == "" {
			arg = fmt.Sprintf("%s is required", name)
		}
		return "", &ExpandError{Path: path, Line: lineNo, Reason: arg}
	default:
		if !ok {
			return "", &ExpandError{Path: path, Line: lineNo, Reason: fmt.Sprintf("%s is unset", name)}
		}
		return value, nil
	}
}
