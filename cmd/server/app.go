package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/dnsx/dnsserver"
	bboltstore "github.com/arcgolabs/dnsx/dnsserver/store/bbolt"
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
			dix.Provider2(newManager),
		),
		dix.Hooks(
			dix.OnStop(func(_ context.Context, store *bboltstore.Store) error {
				return store.Close()
			}),
		),
	)

	serverModule := dix.NewModule("server",
		dix.Imports(configModule, logModule, infraModule),
		dix.Providers(
			dix.Provider1(newServerConfig),
			dix.Provider4(newDNSServer),
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

	adminModule := dix.NewModule("admin",
		dix.Imports(configModule, logModule, infraModule),
		dix.Providers(
			dix.Provider1(newHTTPConfig),
			dix.Provider3(newAdminServer),
		),
		dix.Hooks(
			dix.OnStart(func(ctx context.Context, server *adminServer) error {
				return server.Start(ctx)
			}),
			dix.OnStop(func(ctx context.Context, server *adminServer) error {
				return server.Stop(ctx)
			}),
		),
	)

	return dix.New(
		"dnsx-server",
		dix.Modules(configModule, logModule, infraModule, serverModule, adminModule),
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

	logger, err := logx.New(options...)
	if err != nil {
		return nil, oops.In("cmd/server").
			With("op", "new_logger", "level", cfg.Log.Level, "file", cfg.Log.File).
			Wrapf(err, "create logger")
	}

	return logger, nil
}

func openStore(cfg Config, logger *slog.Logger) (*bboltstore.Store, error) {
	store, err := bboltstore.Open(cfg.Storage.Path, logger)
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
		if closeErr := store.Close(); closeErr != nil {
			logger.Error("close store after seed load failure", "err", closeErr)
		}
		return nil, oops.In("cmd/server").
			With("op", "load_seed", "path", cfg.Seed.File).
			Wrapf(err, "load seed data")
	}
	if err := dnsserver.ApplySeedData(context.Background(), store, seed); err != nil {
		if closeErr := store.Close(); closeErr != nil {
			logger.Error("close store after seed apply failure", "err", closeErr)
		}
		return nil, oops.In("cmd/server").
			With("op", "apply_seed", "path", cfg.Seed.File).
			Wrapf(err, "apply seed data")
	}

	return store, nil
}

func newResolver(cfg Config, store *bboltstore.Store, logger *slog.Logger) *dnsserver.Resolver {
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

func newManager(store *bboltstore.Store, logger *slog.Logger) *dnsserver.Manager {
	return dnsserver.NewManager(store, dnsserver.WithManagerLogger(logger))
}

func newServerConfig(cfg Config) dnsserver.Config {
	return dnsserver.Config{
		Listen: cfg.Server.Listen,
	}
}

func newHTTPConfig(cfg Config) HTTPConfig {
	return cfg.HTTP
}

func newDNSServer(
	cfg dnsserver.Config,
	resolver *dnsserver.Resolver,
	manager *dnsserver.Manager,
	logger *slog.Logger,
) *dnsserver.Server {
	return dnsserver.NewServerWithResolver(
		cfg,
		resolver,
		dnsserver.WithManager(manager),
		dnsserver.WithLogger(logger),
	)
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
