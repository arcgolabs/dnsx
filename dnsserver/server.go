package dnsserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

type Config struct {
	Listen string
}

type Option func(*Server)

type Server struct {
	config      Config
	logger      *slog.Logger
	handler     dns.Handler
	udpConn     net.PacketConn
	tcpListener net.Listener
	udpServer   *dns.Server
	tcpServer   *dns.Server
	startOnce   sync.Once
}

func NewServer(config Config, handler dns.Handler, opts ...Option) *Server {
	if config.Listen == "" {
		config.Listen = ":5354"
	}

	server := &Server{
		config:  config,
		logger:  slog.Default(),
		handler: handler,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(server)
		}
	}

	return server
}

func NewServerWithResolver(config Config, resolver *Resolver, opts ...Option) *Server {
	return NewServer(config, resolver, opts...)
}

func WithLogger(logger *slog.Logger) Option {
	return func(server *Server) {
		if logger != nil {
			server.logger = logger
		}
	}
}

func (s *Server) Start(context.Context) error {
	if s == nil {
		return errors.New("dns server is nil")
	}
	if s.handler == nil {
		return errors.New("dns handler is nil")
	}

	var startErr error
	s.startOnce.Do(func() {
		s.udpConn, startErr = net.ListenPacket("udp", s.config.Listen)
		if startErr != nil {
			return
		}

		s.tcpListener, startErr = net.Listen("tcp", s.config.Listen)
		if startErr != nil {
			_ = s.udpConn.Close()
			s.udpConn = nil
			return
		}

		s.udpServer = &dns.Server{PacketConn: s.udpConn, Handler: s.handler}
		s.tcpServer = &dns.Server{Listener: s.tcpListener, Handler: s.handler}

		go s.serve("udp", func() error { return s.udpServer.ActivateAndServe() })
		go s.serve("tcp", func() error { return s.tcpServer.ActivateAndServe() })

		s.logger.Info("dns server started", "udp", s.udpConn.LocalAddr().String(), "tcp", s.tcpListener.Addr().String())
	})

	return startErr
}

func (s *Server) Stop(context.Context) error {
	if s == nil {
		return nil
	}

	var errs []error
	if s.udpServer != nil {
		errs = append(errs, s.udpServer.Shutdown())
	}
	if s.tcpServer != nil {
		errs = append(errs, s.tcpServer.Shutdown())
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	if s.udpServer != nil || s.tcpServer != nil {
		s.logger.Info("dns server stopped")
	}
	return nil
}

func (s *Server) UDPAddr() string {
	if s == nil || s.udpConn == nil {
		return ""
	}
	return s.udpConn.LocalAddr().String()
}

func (s *Server) TCPAddr() string {
	if s == nil || s.tcpListener == nil {
		return ""
	}
	return s.tcpListener.Addr().String()
}

func (s *Server) serve(network string, run func() error) {
	if err := run(); err != nil && !isClosedServerError(err) {
		s.logger.Error("dns server serve failed", "network", network, "err", err)
	}
}

func isClosedServerError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, net.ErrClosed) || strings.Contains(strings.ToLower(err.Error()), "server closed")
}
