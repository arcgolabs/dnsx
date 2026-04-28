package dnsserver

import (
	"log/slog"
	"net"

	"github.com/arcgolabs/dnsx/dnsclient"
	"github.com/miekg/dns"
	"github.com/samber/lo"
)

type Config struct {
	Listen string
}

type Option func(*Server)

type Server struct {
	config      Config
	logger      *slog.Logger
	repo        Repository
	manager     *Manager
	resolver    *Resolver
	handler     dns.Handler
	clientOpts  []dnsclient.Option
	udpConn     net.PacketConn
	tcpListener net.Listener
	udpServer   *dns.Server
	tcpServer   *dns.Server
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

	server.syncManagedComponents()
	return server
}

func NewServerWithResolver(config Config, resolver *Resolver, opts ...Option) *Server {
	server := NewServer(config, nil, opts...)
	server.resolver = resolver
	if resolver != nil {
		server.repo = resolver.Repository()
	}
	server.syncManagedComponents()
	return server
}

func NewServerWithRepository(config Config, repo Repository, opts ...Option) *Server {
	server := NewServer(config, nil, opts...)
	server.repo = repo
	server.syncManagedComponents()
	return server
}

func WithLogger(logger *slog.Logger) Option {
	return func(server *Server) {
		if logger != nil {
			server.logger = logger
		}
	}
}

func WithManager(manager *Manager) Option {
	return func(server *Server) {
		if manager != nil {
			server.manager = manager
			server.repo = manager.Repository()
		}
	}
}

func WithResolver(resolver *Resolver) Option {
	return func(server *Server) {
		if resolver != nil {
			server.resolver = resolver
			server.repo = resolver.Repository()
		}
	}
}

func WithClientOptions(opts ...dnsclient.Option) Option {
	return func(server *Server) {
		server.clientOpts = append(server.clientOpts, opts...)
	}
}

func WithHandler(handler dns.Handler) Option {
	return func(server *Server) {
		if handler != nil {
			server.handler = handler
		}
	}
}

func (s *Server) Resolver() *Resolver {
	if s == nil {
		return nil
	}

	return s.resolver
}

func (s *Server) Repository() Repository {
	if s == nil {
		return nil
	}

	return s.repo
}

func (s *Server) Manager() *Manager {
	if s == nil {
		return nil
	}

	return s.manager
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

func (s *Server) ensureHandler() {
	if s.handler != nil {
		return
	}
	if s.resolver != nil || s.repo != nil {
		s.handler = dns.HandlerFunc(s.serveDNS)
	}
}

func (s *Server) syncManagedComponents() {
	if s == nil {
		return
	}

	if s.repo == nil {
		switch {
		case s.manager != nil:
			s.repo = s.manager.Repository()
		case s.resolver != nil:
			s.repo = s.resolver.Repository()
		}
	}

	if s.manager == nil && s.repo != nil {
		s.manager = NewManager(s.repo, WithManagerLogger(s.logger))
	}
	if s.resolver == nil && s.repo != nil {
		s.resolver = NewResolver(s.repo, WithResolverLogger(s.logger))
	}

	s.ensureHandler()
}

func (s *Server) clientTarget() string {
	if s == nil {
		return "127.0.0.1:5354"
	}

	address := lo.Ternary(s.UDPAddr() != "", s.UDPAddr(), s.config.Listen)
	if address == "" {
		address = ":5354"
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}

	host = lo.Ternary(
		host == "" || host == "0.0.0.0" || host == "::" || host == "[::]",
		"127.0.0.1",
		host,
	)

	return net.JoinHostPort(host, port)
}
