package dnsserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/arcgolabs/dnsx/dnsclient"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type Config struct {
	Listen string
}

type Option func(*Server)

type Server struct {
	config      Config
	logger      *slog.Logger
	repo        Repository
	resolver    *Resolver
	handler     dns.Handler
	clientOpts  []dnsclient.Option
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
	server := NewServer(config, nil, opts...)
	server.resolver = resolver
	if resolver != nil {
		server.repo = resolver.Repository()
	}
	server.ensureHandler()
	return server
}

func NewServerWithRepository(config Config, repo Repository, opts ...Option) *Server {
	server := NewServer(config, nil, opts...)
	server.repo = repo
	if server.resolver == nil && repo != nil {
		server.resolver = NewResolver(repo)
	}
	server.ensureHandler()
	return server
}

func WithLogger(logger *slog.Logger) Option {
	return func(server *Server) {
		if logger != nil {
			server.logger = logger
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

func (s *Server) Start(context.Context) error {
	if s == nil {
		return oops.In("dnsserver").
			With("op", "start_server").
			New("dns server is nil")
	}
	s.ensureHandler()
	if s.handler == nil {
		return oops.In("dnsserver").
			With("op", "start_server", "listen", s.config.Listen).
			New("dns handler is nil")
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

		s.udpServer = &dns.Server{
			PacketConn:    s.udpConn,
			Handler:       s.handler,
			MsgAcceptFunc: acceptMsg,
		}
		s.tcpServer = &dns.Server{
			Listener:      s.tcpListener,
			Handler:       s.handler,
			MsgAcceptFunc: acceptMsg,
		}

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

func (s *Server) Client(opts ...dnsclient.Option) *dnsclient.Client {
	base := append([]dnsclient.Option(nil), s.clientOpts...)
	if s != nil && s.logger != nil {
		base = append(base, dnsclient.WithLogger(s.logger))
	}
	return dnsclient.NewClient(s.clientTarget(), append(base, opts...)...)
}

func (s *Server) Query(ctx context.Context, name string, qtype uint16, opts ...dnsclient.Option) (*dns.Msg, time.Duration, error) {
	return s.Client(opts...).Exchange(ctx, name, qtype)
}

func (s *Server) LookupA(ctx context.Context, name string, opts ...dnsclient.Option) ([]string, error) {
	return s.Client(opts...).LookupA(ctx, name)
}

func (s *Server) UpsertRecord(ctx context.Context, record Record, opts ...dnsclient.Option) (*dns.Msg, time.Duration, error) {
	normalized, err := NormalizeRecord(record)
	if err != nil {
		return nil, 0, err
	}

	rr, err := normalized.RR()
	if err != nil {
		return nil, 0, err
	}

	return s.Client(opts...).UpdateAdd(ctx, normalized.Zone, rr)
}

func (s *Server) DeleteRecord(ctx context.Context, record Record, opts ...dnsclient.Option) (*dns.Msg, time.Duration, error) {
	normalized, err := NormalizeRecord(record)
	if err != nil {
		return nil, 0, err
	}

	rr, err := normalized.RR()
	if err != nil {
		return nil, 0, err
	}

	return s.Client(opts...).UpdateRemove(ctx, normalized.Zone, rr)
}

func (s *Server) DeleteRRSet(ctx context.Context, zone string, name string, rrtype uint16, opts ...dnsclient.Option) (*dns.Msg, time.Duration, error) {
	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return nil, 0, err
	}

	return s.Client(opts...).UpdateRemoveRRSet(ctx, normalizedZone, dns.Fqdn(name), rrtype)
}

func (s *Server) DeleteName(ctx context.Context, zone string, name string, opts ...dnsclient.Option) (*dns.Msg, time.Duration, error) {
	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return nil, 0, err
	}

	return s.Client(opts...).UpdateRemoveName(ctx, normalizedZone, dns.Fqdn(name))
}

func (s *Server) serve(network string, run func() error) {
	if err := run(); err != nil && !isClosedServerError(err) {
		s.logger.Error("dns server serve failed", "network", network, "err", err)
	}
}

func (s *Server) ensureHandler() {
	if s.handler != nil {
		return
	}
	if s.resolver != nil || s.repo != nil {
		s.handler = dns.HandlerFunc(s.serveDNS)
	}
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

func isClosedServerError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, net.ErrClosed) || strings.Contains(strings.ToLower(err.Error()), "server closed")
}

func acceptMsg(header dns.Header) dns.MsgAcceptAction {
	opcode := int(header.Bits>>11) & 0xF
	if opcode == dns.OpcodeUpdate {
		return dns.MsgAccept
	}

	return dns.DefaultMsgAcceptFunc(header)
}
