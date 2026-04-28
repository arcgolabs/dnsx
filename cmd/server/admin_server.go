package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type adminServer struct {
	logger   *slog.Logger
	http     *http.Server
	listener net.Listener
}

func newAdminServer(cfg HTTPConfig, logger *slog.Logger, manager *dnsserver.Manager) *adminServer {
	server := &adminServer{
		logger: lo.Ternary(logger != nil, logger, slog.Default()),
	}

	if strings.TrimSpace(cfg.Listen) == "" {
		return server
	}

	server.http = &http.Server{
		Addr:              cfg.Listen,
		Handler:           newAdminHandler(server.logger, manager),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return server
}

func (s *adminServer) Start(ctx context.Context) error {
	if s == nil || s.http == nil || s.listener != nil {
		return nil
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.http.Addr)
	if err != nil {
		return oops.In("cmd/server").
			With("op", "start_admin_server", "listen", s.http.Addr).
			Wrapf(err, "listen for admin server")
	}

	s.listener = listener
	s.logger.Info("dns admin server listening", "listen", listener.Addr().String())

	go func() {
		if serveErr := s.http.Serve(listener); !isHTTPClosedError(serveErr) {
			s.logger.Error("dns admin server exited", "err", serveErr)
		}
	}()

	return nil
}

func (s *adminServer) Stop(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}

	err := s.http.Shutdown(ctx)
	s.listener = nil
	if isHTTPClosedError(err) {
		return nil
	}
	if err != nil {
		return oops.In("cmd/server").
			With("op", "stop_admin_server").
			Wrapf(err, "shutdown admin server")
	}

	return nil
}

func isHTTPClosedError(err error) bool {
	return err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed)
}
