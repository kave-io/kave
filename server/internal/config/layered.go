package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	yaml "go.yaml.in/yaml/v3"
)

type Source string

const (
	ConfigNameYML  = "kave.yml"
	ConfigNameYAML = "kave.yaml"

	SourceBuiltin Source = "builtin"
	SourceSystem  Source = "system"
	SourceUser    Source = "user"
	SourceProject Source = "project"
	SourceEnv     Source = "env"
)

type LayerFile struct {
	Source Source         `json:"source" yaml:"source"`
	Path   string         `json:"path,omitempty" yaml:"path,omitempty"`
	Raw    map[string]any `json:"raw,omitempty" yaml:"raw,omitempty"`
}

type LoadResult struct {
	Config *Config
	Layers []LayerFile
	Origin map[string]Source
}

type LoadOpts struct {
	ExplicitPath string
	StartDir     string
	Env          map[string]string
}

func Load(opts LoadOpts) (*LoadResult, error) {
	env := opts.Env
	if env == nil {
		env = envMap(os.Environ())
	}

	result := &LoadResult{
		Config: &Config{},
		Origin: map[string]Source{},
	}
	result.Layers = append(result.Layers, LayerFile{Source: SourceBuiltin})

	merged := map[string]any{}

	systemPath := systemConfigPath()
	if raw, err := loadLayerFile(systemPath, SourceSystem, env); err != nil {
		return nil, err
	} else if raw != nil {
		result.Layers = append(result.Layers, *raw)
		mergeLayerMaps(merged, raw.Raw, "", result.Origin, raw.Source)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	userPath := filepath.Join(homeDir, ".kave", ConfigNameYAML)
	if raw, err := loadLayerFile(userPath, SourceUser, env); err != nil {
		return nil, err
	} else if raw != nil {
		result.Layers = append(result.Layers, *raw)
		mergeLayerMaps(merged, raw.Raw, "", result.Origin, raw.Source)
	}

	projectPath := opts.ExplicitPath
	if projectPath == "" {
		projectPath, err = DiscoverProjectConfig(opts.StartDir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			// Discovery failure is non-fatal; simply skip the project layer.
			projectPath = ""
		}
	}

	if projectPath != "" {
		raw, err := loadLayerFile(projectPath, SourceProject, env)
		if err != nil {
			return nil, err
		}
		if raw != nil {
			result.Layers = append(result.Layers, *raw)
			mergeLayerMaps(merged, raw.Raw, "", result.Origin, raw.Source)
		}
	} else if opts.ExplicitPath != "" {
		return nil, fmt.Errorf("config file %q not found", opts.ExplicitPath)
	}

	v := viper.New()
	v.SetConfigType("yaml")
	bindEnvs(v, Config{}, "")
	if err := v.MergeConfigMap(merged); err != nil {
		return nil, fmt.Errorf("merge config: %w", err)
	}

	if err := v.Unmarshal(result.Config); err != nil {
		return nil, fmt.Errorf("decode merged config: %w", err)
	}

	if err := applyEnvOverrides(result.Config, env); err != nil {
		return nil, err
	}

	if err := result.Config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	applyBuiltinOrigins(result.Config, result.Origin, "")
	applyEnvOrigins(result.Config, result.Origin, env, "")

	result.Layers = append(result.Layers, LayerFile{Source: SourceEnv})
	GlobalConf = result.Config
	return result, nil
}

func loadLayerFile(path string, source Source, env map[string]string) (*LayerFile, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config layer %q: %w", path, err)
	}

	expanded, err := Expand(string(raw), path, env)
	if err != nil {
		return nil, err
	}

	layer := &LayerFile{Source: source, Path: path}
	if err := yaml.Unmarshal([]byte(expanded), &layer.Raw); err != nil {
		return nil, fmt.Errorf("parse config layer %q: %w", path, err)
	}
	if layer.Raw == nil {
		layer.Raw = map[string]any{}
	}
	return layer, nil
}

func DiscoverProjectConfig(startDir string) (string, error) {
	dir := startDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	dir = filepath.Clean(dir)
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		homeDir = filepath.Clean(homeDir)
	}

	for {
		for _, name := range []string{ConfigNameYAML, ConfigNameYML} {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}

		if homeDir != "" && samePath(dir, homeDir) {
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", os.ErrNotExist
}

func mergeLayerMaps(dst, src map[string]any, prefix string, origin map[string]Source, source Source) {
	for key, value := range src {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		if existing, ok := dst[key]; ok {
			switch sv := value.(type) {
			case map[string]any:
				ev, ok := existing.(map[string]any)
				if ok {
					mergeLayerMaps(ev, sv, path, origin, source)
					continue
				}
			case []any:
				if ev, ok := existing.([]any); ok && isResourceListPath(path) {
					dst[key] = mergeResourceList(ev, sv, path, origin, source)
					continue
				}
			}
		}

		dst[key] = cloneValue(value)
		recordOriginTree(value, path, origin, source)
	}
}

func mergeResourceList(existing, incoming []any, path string, origin map[string]Source, source Source) []any {
	index := make(map[string]int, len(existing))
	out := make([]any, len(existing))
	copy(out, existing)
	for i, item := range existing {
		if m, ok := item.(map[string]any); ok {
			if name, ok := m["name"].(string); ok && name != "" {
				index[name] = i
			}
		}
	}

	for _, item := range incoming {
		m, ok := item.(map[string]any)
		if !ok {
			out = append(out, cloneValue(item))
			continue
		}
		name, _ := m["name"].(string)
		if name != "" {
			if i, ok := index[name]; ok {
				out[i] = cloneValue(item)
			} else {
				index[name] = len(out)
				out = append(out, cloneValue(item))
			}
			recordOriginTree(item, path+"."+name, origin, source)
			continue
		}
		out = append(out, cloneValue(item))
		recordOriginTree(item, path+"."+fmt.Sprint(len(out)-1), origin, source)
	}
	return out
}

func recordOriginTree(value any, path string, origin map[string]Source, source Source) {
	switch v := value.(type) {
	case map[string]any:
		for key, nested := range v {
			child := key
			if path != "" {
				child = path + "." + key
			}
			recordOriginTree(nested, child, origin, source)
		}
	case []any:
		origin[path] = source
		for i, nested := range v {
			recordOriginTree(nested, fmt.Sprintf("%s.%d", path, i), origin, source)
		}
	default:
		origin[path] = source
	}
}

func applyBuiltinOrigins(cfg *Config, origin map[string]Source, prefix string) {
	if cfg == nil {
		return
	}
	walkConfigValue(reflect.ValueOf(cfg).Elem(), prefix, func(path string, value reflect.Value) {
		if path == "" {
			return
		}
		if _, ok := origin[path]; ok {
			return
		}
		if isZeroValue(value) {
			return
		}
		origin[path] = SourceBuiltin
	})
}

func applyEnvOrigins(cfg *Config, origin map[string]Source, env map[string]string, prefix string) {
	if cfg == nil {
		return
	}
	walkConfigValue(reflect.ValueOf(cfg).Elem(), prefix, func(path string, value reflect.Value) {
		if path == "" {
			return
		}
		key := envPrefix + "_" + strings.ToUpper(strings.ReplaceAll(path, ".", "_"))
		if _, ok := env[key]; !ok {
			return
		}
		origin[path] = SourceEnv
	})
}

func applyEnvOverrides(cfg *Config, env map[string]string) error {
	if cfg == nil {
		return nil
	}
	return overrideStruct(reflect.ValueOf(cfg).Elem(), "", env)
}

func overrideStruct(v reflect.Value, prefix string, env map[string]string) error {
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			continue
		}
		child := tag
		if prefix != "" {
			child = prefix + "." + tag
		}
		fv := v.Field(i)
		if fv.Kind() == reflect.Pointer {
			if fv.IsNil() {
				if fv.CanSet() && fv.Type().Elem().Kind() == reflect.Struct {
					fv.Set(reflect.New(fv.Type().Elem()))
				} else {
					continue
				}
			}
			if fv.Elem().Kind() == reflect.Struct && !isTimeLike(fv.Elem()) {
				if err := overrideStruct(fv.Elem(), child, env); err != nil {
					return err
				}
				continue
			}
		}
		if fv.Kind() == reflect.Struct && !isTimeLike(fv) {
			if err := overrideStruct(fv, child, env); err != nil {
				return err
			}
			continue
		}
		key := envPrefix + "_" + strings.ToUpper(strings.ReplaceAll(child, ".", "_"))
		value, ok := env[key]
		if !ok {
			continue
		}
		if err := setReflectValue(fv, value); err != nil {
			return fmt.Errorf("apply env %s: %w", key, err)
		}
	}
	return nil
}

func setReflectValue(v reflect.Value, raw string) error {
	if !v.CanSet() {
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(n)
	default:
		return nil
	}
	return nil
}

func walkConfigValue(v reflect.Value, prefix string, fn func(path string, value reflect.Value)) {
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		walkConfigValue(v.Elem(), prefix, fn)
		return
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			continue
		}
		child := tag
		if prefix != "" {
			child = prefix + "." + tag
		}
		fv := v.Field(i)
		if fv.Kind() == reflect.Struct && !isTimeLike(fv) {
			walkConfigValue(fv, child, fn)
			continue
		}
		if fv.Kind() == reflect.Pointer && !fv.IsNil() && fv.Elem().Kind() == reflect.Struct && !isTimeLike(fv.Elem()) {
			walkConfigValue(fv.Elem(), child, fn)
			continue
		}
		fn(child, fv)
	}
}

func isZeroValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	default:
		return v.IsZero()
	}
}

func isTimeLike(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	t := v.Type()
	return t.PkgPath() == "time" && t.Name() == "Time"
}

func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, value := range x {
			out[key] = cloneValue(value)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = cloneValue(value)
		}
		return out
	default:
		return v
	}
}

func isResourceListPath(path string) bool {
	switch path {
	case "agents", "policies", "credentials", "connectors":
		return true
	default:
		return false
	}
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtimePathSeparatorInsensitive() {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func runtimePathSeparatorInsensitive() bool {
	return string(os.PathSeparator) == "\\"
}

func envMap(values []string) map[string]string {
	out := make(map[string]string, len(values))
	for _, kv := range values {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		out[k] = v
	}
	return out
}

func currentEnvMap() map[string]string {
	return envMap(os.Environ())
}
