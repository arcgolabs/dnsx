package dnsserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"

	"github.com/miekg/dns"
	"github.com/samber/oops"
)

func (s *Server) Start(ctx context.Context) error {
	if s == nil {
		return errorBuilder("start_server", CodeServerNil).Wrap(ErrServerNil)
	}
	s.ensureHandler()
	if s.handler == nil {
		return errorBuilder("start_server", CodeHandlerNil, "listen", s.config.Listen).Wrap(ErrHandlerNil)
	}
	if s.udpServer != nil || s.tcpServer != nil {
		return errorBuilder("start_server", CodeServerAlreadyStarted, "listen", s.config.Listen).Wrap(ErrServerAlreadyStarted)
	}

	udpConn, err := listenPacket(ctx, "udp", s.config.Listen)
	if err != nil {
		return err
	}

	tcpListener, err := listen(ctx, "tcp", s.config.Listen)
	if err != nil {
		closeNoErr(s.logger, "close_udp_conn", udpConn.Close)
		return err
	}

	s.udpConn = udpConn
	s.tcpListener = tcpListener
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
	return nil
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
		return oops.In("dnsserver").
			With("op", "stop_server", "listen", s.config.Listen).
			Wrapf(err, "stop dns server")
	}

	if s.udpServer != nil || s.tcpServer != nil {
		s.logger.Info("dns server stopped")
	}

	s.udpServer = nil
	s.tcpServer = nil
	s.udpConn = nil
	s.tcpListener = nil
	return nil
}

func listenPacket(ctx context.Context, network, address string) (net.PacketConn, error) {
	var cfg net.ListenConfig
	conn, err := cfg.ListenPacket(ctx, network, address)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "listen_packet", "network", network, "address", address).
			Wrapf(err, "listen packet")
	}

	return conn, nil
}

func listen(ctx context.Context, network, address string) (net.Listener, error) {
	var cfg net.ListenConfig
	listener, err := cfg.Listen(ctx, network, address)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "listen", "network", network, "address", address).
			Wrapf(err, "listen")
	}

	return listener, nil
}

func closeNoErr(logger *slog.Logger, op string, closeFn func() error) {
	if err := closeFn(); err != nil && logger != nil {
		logger.Error("close resource failed", "op", op, "err", err)
	}
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

func acceptMsg(header dns.Header) dns.MsgAcceptAction {
	opcode := int(header.Bits>>11) & 0xF
	if opcode == dns.OpcodeUpdate {
		return dns.MsgAccept
	}

	return dns.DefaultMsgAcceptFunc(header)
}
