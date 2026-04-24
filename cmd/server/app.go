package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/arcgolabs/logx"
	"github.com/samber/oops"
)

func newApp(cfg Config) *dix.App {
	configModule := dix.NewModule("config",
		dix.Providers(
			dix.Value(cfg),
		),
	)

	logModule := dix.NewModule("log",
		dix.Imports(configModule),
		dix.Providers(
			dix.ProviderErr1(newLogger),
		),
		dix.Hooks(
			dix.OnStop(func(_ context.Context, logger *slog.Logger) error {
				return logx.Close(logger)
			}),
		),
	)

	infraModule := dix.NewModule("infra",
		dix.Imports(configModule, logModule),
		dix.Providers(
			dix.ProviderErr2(openStore),
			dix.Provider3(newResolver),
		),
		dix.Hooks(
			dix.OnStop(func(_ context.Context, store *dnsserver.BboltStore) error {
				return store.Close()
			}),
		),
	)

	serverModule := dix.NewModule("server",
		dix.Imports(configModule, logModule, infraModule),
		dix.Providers(
			dix.Provider1(newServerConfig),
			dix.Provider2(newDNSServer),
		),
		dix.Hooks(
			dix.OnStart(func(ctx context.Context, server *dnsserver.Server) error {
				return server.Start(ctx)
			}),
			dix.OnStop(func(ctx context.Context, server *dnsserver.Server) error {
				return server.Stop(ctx)
			}),
		),
	)

	return dix.New(
		"dnsx-server",
		dix.Modules(configModule, logModule, infraModule, serverModule),
	)
}

func defaultLogger() *slog.Logger {
	logger, err := logx.NewDevelopment()
	if err != nil {
		return slog.Default()
	}
	return logger
}

func newLogger(cfg Config) (*slog.Logger, error) {
	options := []logx.Option{
		logx.WithConsole(cfg.Log.Console),
		logx.WithLevelString(cfg.Log.Level),
	}
	if cfg.Log.File != "" {
		options = append(options, logx.WithFile(cfg.Log.File))
	}
	if cfg.Log.Level == "debug" || cfg.Log.Level == "trace" {
		options = append(options, logx.WithCaller(true))
	}

	return logx.New(options...)
}

func openStore(cfg Config, logger *slog.Logger) (*dnsserver.BboltStore, error) {
	store, err := dnsserver.OpenBboltStore(cfg.Storage.Path, logger)
	if err != nil {
		return nil, oops.In("cmd/server").
			With("op", "open_store", "path", cfg.Storage.Path).
			Wrapf(err, "open dns store")
	}

	if cfg.Seed.File == "" {
		return store, nil
	}

	seed, err := dnsserver.LoadSeedData(cfg.Seed.File)
	if err != nil {
		_ = store.Close()
		return nil, oops.In("cmd/server").
			With("op", "load_seed", "path", cfg.Seed.File).
			Wrapf(err, "load seed data")
	}
	if err := dnsserver.ApplySeedData(context.Background(), store, seed); err != nil {
		_ = store.Close()
		return nil, oops.In("cmd/server").
			With("op", "apply_seed", "path", cfg.Seed.File).
			Wrapf(err, "apply seed data")
	}

	return store, nil
}

func newResolver(cfg Config, store *dnsserver.BboltStore, logger *slog.Logger) *dnsserver.Resolver {
	cacheConfig := dnsserver.CacheConfig{
		Capacity:         cfg.Cache.Capacity,
		Algorithm:        dnsserver.CacheAlgorithm(cfg.Cache.Algorithm),
		Janitor:          cfg.Cache.Janitor,
		MissingCapacity:  cfg.Cache.MissingCapacity,
		MissingAlgorithm: dnsserver.CacheAlgorithm(cfg.Cache.MissingAlgorithm),
	}
	return dnsserver.NewResolver(
		store,
		dnsserver.WithResolverLogger(logger),
		dnsserver.WithCache(cacheConfig),
		dnsserver.WithMaxCacheTTL(resolverMaxTTL(logger)),
	)
}

func newServerConfig(cfg Config) dnsserver.Config {
	return dnsserver.Config{
		Listen: cfg.Server.Listen,
	}
}

func newDNSServer(cfg dnsserver.Config, resolver *dnsserver.Resolver) *dnsserver.Server {
	return dnsserver.NewServerWithResolver(cfg, resolver)
}

func isDebug(logger *slog.Logger) bool {
	return logger.Enabled(context.Background(), slog.LevelDebug)
}

func resolverMaxTTL(logger *slog.Logger) time.Duration {
	if isDebug(logger) {
		return 5 * time.Second
	}
	return 30 * time.Second
}
