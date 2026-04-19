package daemon

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"github.com/kave-io/kave/server/internal/config"
)

// ConfigDiffReport is the patch-ish view of config differences.
type ConfigDiffReport struct {
	Added   map[string]any `json:"added"`
	Removed map[string]any `json:"removed"`
	Changed map[string]any `json:"changed"`
}

func diffConfigs(live, disk *config.Config) ConfigDiffReport {
	liveMap := configSnapshot(live)
	diskMap := configSnapshot(disk)
	return ConfigDiffReport{
		Added:   diffAdded(liveMap, diskMap),
		Removed: diffRemoved(liveMap, diskMap),
		Changed: diffChanged(liveMap, diskMap),
	}
}

func diffConfigPaths(live, disk *config.Config) []string {
	liveMap := configSnapshot(live)
	diskMap := configSnapshot(disk)
	var paths []string
	collectDiffPaths(&paths, "", liveMap, diskMap)
	sort.Strings(paths)
	return dedupeStrings(paths)
}

func redactConfigValue(v any) map[string]any {
	raw, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return redactMap(out)
}

func redactMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for key, value := range m {
		if isSecretKey(key) {
			out[key] = "***"
			continue
		}
		switch nested := value.(type) {
		case map[string]any:
			out[key] = redactMap(nested)
		case []any:
			out[key] = redactSlice(nested)
		default:
			out[key] = value
		}
	}
	return out
}

func redactSlice(items []any) []any {
	out := make([]any, len(items))
	for i, item := range items {
		switch nested := item.(type) {
		case map[string]any:
			out[i] = redactMap(nested)
		case []any:
			out[i] = redactSlice(nested)
		default:
			out[i] = item
		}
	}
	return out
}

func isSecretKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "key")
}

func collectDiffPaths(paths *[]string, prefix string, live, disk any) {
	liveMap, liveOK := live.(map[string]any)
	diskMap, diskOK := disk.(map[string]any)
	if liveOK && diskOK {
		keys := make(map[string]struct{}, len(liveMap)+len(diskMap))
		for key := range liveMap {
			keys[key] = struct{}{}
		}
		for key := range diskMap {
			keys[key] = struct{}{}
		}
		for key := range keys {
			child := key
			if prefix != "" {
				child = prefix + "." + key
			}
			collectDiffPaths(paths, child, liveMap[key], diskMap[key])
		}
		return
	}
	if !reflect.DeepEqual(live, disk) {
		*paths = append(*paths, prefix)
	}
}

func diffAdded(live, disk map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range disk {
		if _, ok := live[key]; ok {
			if lm, lok := live[key].(map[string]any); lok {
				if dm, dok := value.(map[string]any); dok {
					if nested := diffAdded(lm, dm); len(nested) > 0 {
						out[key] = nested
					}
				}
			}
			continue
		}
		out[key] = value
	}
	return out
}

func diffRemoved(live, disk map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range live {
		if _, ok := disk[key]; ok {
			if lm, lok := value.(map[string]any); lok {
				if dm, dok := disk[key].(map[string]any); dok {
					if nested := diffRemoved(lm, dm); len(nested) > 0 {
						out[key] = nested
					}
				}
			}
			continue
		}
		out[key] = value
	}
	return out
}

func diffChanged(live, disk map[string]any) map[string]any {
	out := map[string]any{}
	for key, liveValue := range live {
		diskValue, ok := disk[key]
		if !ok {
			continue
		}
		if lm, lok := liveValue.(map[string]any); lok {
			if dm, dok := diskValue.(map[string]any); dok {
				if nested := diffChanged(lm, dm); len(nested) > 0 {
					out[key] = nested
				}
				continue
			}
		}
		if !reflect.DeepEqual(liveValue, diskValue) {
			out[key] = map[string]any{
				"live": liveValue,
				"disk": diskValue,
			}
		}
	}
	return out
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	var prev string
	for i, value := range values {
		if i == 0 || value != prev {
			out = append(out, value)
			prev = value
		}
	}
	return out
}

