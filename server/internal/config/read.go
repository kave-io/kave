package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/kave-io/kave/core/pkg/constants"
	"github.com/spf13/viper"
)

const envPrefix = "KAVE"

var GlobalConf *Config

func ReadConfig(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigName(constants.ConfigName)
	v.SetConfigType(constants.ConfigFormat)
	v.AddConfigPath(configPath)

	// Explicitly bind every config key to its KAVE_* env var before reading
	// the file. This fixes Viper's well-known limitation: AutomaticEnv() does
	// not resolve nested struct keys that are absent from the config file,
	// making env-only deployments (Docker/K8s) silently produce zero values.
	bindEnvs(v, Config{}, "")

	// Config file is optional — env vars alone are sufficient for container
	// deployments. Only fail if the file exists but cannot be parsed.
	if err := v.ReadInConfig(); err != nil {
		if _, notFound := err.(viper.ConfigFileNotFoundError); !notFound {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
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
