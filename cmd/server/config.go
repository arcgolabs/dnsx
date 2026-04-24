package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/arcgolabs/configx"
	"github.com/samber/lo"
	"github.com/samber/mo"
	"github.com/spf13/pflag"
)

type Config struct {
	Config  string        `validate:"-"`
	Server  ServerConfig  `validate:"required"`
	Storage StorageConfig `validate:"required"`
	Seed    SeedConfig    `validate:"-"`
	Log     LogConfig     `validate:"required"`
	Cache   CacheConfig   `validate:"required"`
}

type ServerConfig struct {
	Listen string `validate:"required"`
}

type StorageConfig struct {
	Path string `validate:"required"`
}

type SeedConfig struct {
	File string `validate:"-"`
}

type LogConfig struct {
	Console bool   `validate:"-"`
	Level   string `validate:"required,oneof=trace debug info warn error fatal panic"`
	File    string `validate:"-"`
}

type CacheConfig struct {
	Capacity         int    `validate:"required,min=1"`
	Algorithm        string `validate:"required,oneof=lru lfu tinylfu wtinylfu 2q arc fifo sieve"`
	Janitor          bool   `validate:"-"`
	MissingCapacity  int    `validate:"min=0"`
	MissingAlgorithm string `validate:"omitempty,oneof=lru lfu tinylfu wtinylfu 2q arc fifo sieve"`
}

func loadConfig(args []string) (Config, error) {
	flagSet := newFlagSet()
	if err := flagSet.Parse(args); err != nil {
		return Config{}, err
	}

	configFile := findConfigFile(args).
		OrElse(lo.Ternary(os.Getenv("DNSX_CONFIG") != "", os.Getenv("DNSX_CONFIG"), ""))

	options := []configx.Option{
		configx.WithTypedDefaults(defaultConfig()),
		configx.WithEnvPrefix("DNSX"),
		configx.WithEnvSeparator("__"),
		configx.WithFlagSet(flagSet),
		configx.WithValidateLevel(configx.ValidateLevelStruct),
	}
	if configFile != "" {
		options = append(options, configx.WithFiles(configFile))
	}

	cfg, err := configx.LoadTErr[Config](options...)
	if err != nil {
		return Config{}, fmt.Errorf("load config with configx: %w", err)
	}

	cfg.Config = configFile
	return cfg, nil
}

func newFlagSet() *pflag.FlagSet {
	flagSet := pflag.NewFlagSet("dnsxd", pflag.ContinueOnError)
	flagSet.SortFlags = false

	flagSet.String("config", "", "optional config file path")
	flagSet.String("server-listen", "", "DNS listen address for both UDP and TCP")
	flagSet.String("storage-path", "", "bbolt data path")
	flagSet.String("seed-file", "", "optional JSON seed file")
	flagSet.Bool("log-console", false, "enable console logging")
	flagSet.String("log-level", "", "log level: trace|debug|info|warn|error|fatal|panic")
	flagSet.String("log-file", "", "optional log file path")
	flagSet.Int("cache-capacity", 0, "resolver cache capacity")
	flagSet.String("cache-algorithm", "", "resolver cache algorithm: lru|lfu|tinylfu|wtinylfu|2q|arc|fifo|sieve")
	flagSet.Bool("cache-janitor", false, "enable background janitor for resolver cache")
	flagSet.Int("cache-missing-capacity", 0, "missing-key cache capacity")
	flagSet.String("cache-missing-algorithm", "", "missing-key cache algorithm")

	return flagSet
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Listen: ":5354",
		},
		Storage: StorageConfig{
			Path: "dnsx.db",
		},
		Log: LogConfig{
			Console: true,
			Level:   "info",
		},
		Cache: CacheConfig{
			Capacity:  1024,
			Algorithm: "lru",
		},
	}
}

func findConfigFile(args []string) mo.Option[string] {
	for index := 0; index < len(args); index++ {
		switch {
		case args[index] == "--config" && index+1 < len(args):
			return mo.Some(strings.TrimSpace(args[index+1]))
		case strings.HasPrefix(args[index], "--config="):
			return mo.Some(strings.TrimSpace(strings.TrimPrefix(args[index], "--config=")))
		}
	}

	return mo.None[string]()
}
