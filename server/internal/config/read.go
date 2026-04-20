package config

import (
	"github.com/spf13/viper"
	"os"
	"reflect"
	"strings"
)

const envPrefix = "KAVE"

var GlobalConf *Config

func ReadConfig(configPath string) (*Config, error) {
	opts := LoadOpts{}
	if configPath != "" {
		if info, err := os.Stat(configPath); err == nil && info.IsDir() {
			opts.StartDir = configPath
		} else {
			opts.ExplicitPath = configPath
		}
	}
	result, err := Load(opts)
	if err != nil {
		return nil, err
	}
	GlobalConf = result.Config
	return result.Config, nil
}

// bindEnvs walks cfg's mapstructure tags recursively and calls v.BindEnv for
// every leaf field, mapping "parent.child" → "KAVE_PARENT_CHILD".
//
// This must be called before ReadInConfig. Viper's precedence guarantees that
// env vars always win over file values, regardless of call order.
func bindEnvs(v *viper.Viper, cfg interface{}, prefix string) {
	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			continue
		}

		key := tag
		if prefix != "" {
			key = prefix + "." + tag
		}

		ft := field.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		if ft.Kind() == reflect.Struct {
			bindEnvs(v, reflect.New(ft).Elem().Interface(), key)
		} else {
			envVar := envPrefix + "_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
			_ = v.BindEnv(key, envVar)
		}
	}
}

func MustReadConfig(path string) *Config {
	cfg, err := ReadConfig(path)
	if err != nil {
		panic(err)
	}

	GlobalConf = cfg
	return cfg
}
