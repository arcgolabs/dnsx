package dnsserver

import (
	"context"

	"github.com/arcgolabs/dnsx/dnsclient"
	"github.com/miekg/dns"
	"github.com/samber/oops"
)

type MXRecord = dnsclient.MXRecord

type SRVRecord = dnsclient.SRVRecord

func (s *Server) Lookup(ctx context.Context, name string, qtype uint16, opts ...dnsclient.Option) ([]dns.RR, error) {
	answers, err := s.Client(opts...).Lookup(ctx, name, qtype)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "server_lookup", "name", name, "type", qtype).
			Wrapf(err, "lookup records")
	}

	return answers, nil
}

func (s *Server) LookupA(ctx context.Context, name string, opts ...dnsclient.Option) ([]string, error) {
	return lookupServerValues(ctx, s, "server_lookup_a", name, dns.TypeA, (*dnsclient.Client).LookupA, opts...)
}

func (s *Server) LookupAAAA(ctx context.Context, name string, opts ...dnsclient.Option) ([]string, error) {
	return lookupServerValues(ctx, s, "server_lookup_aaaa", name, dns.TypeAAAA, (*dnsclient.Client).LookupAAAA, opts...)
}

func (s *Server) LookupCNAME(ctx context.Context, name string, opts ...dnsclient.Option) ([]string, error) {
	return lookupServerValues(ctx, s, "server_lookup_cname", name, dns.TypeCNAME, (*dnsclient.Client).LookupCNAME, opts...)
}

func (s *Server) LookupNS(ctx context.Context, name string, opts ...dnsclient.Option) ([]string, error) {
	return lookupServerValues(ctx, s, "server_lookup_ns", name, dns.TypeNS, (*dnsclient.Client).LookupNS, opts...)
}

func (s *Server) LookupTXT(ctx context.Context, name string, opts ...dnsclient.Option) ([]string, error) {
	return lookupServerValues(ctx, s, "server_lookup_txt", name, dns.TypeTXT, (*dnsclient.Client).LookupTXT, opts...)
}

func (s *Server) LookupMX(ctx context.Context, name string, opts ...dnsclient.Option) ([]MXRecord, error) {
	return lookupServerValues(ctx, s, "server_lookup_mx", name, dns.TypeMX, (*dnsclient.Client).LookupMX, opts...)
}

func (s *Server) LookupSRV(ctx context.Context, name string, opts ...dnsclient.Option) ([]SRVRecord, error) {
	return lookupServerValues(ctx, s, "server_lookup_srv", name, dns.TypeSRV, (*dnsclient.Client).LookupSRV, opts...)
}

func lookupServerValues[T any](
	ctx context.Context,
	server *Server,
	op string,
	name string,
	qtype uint16,
	lookup func(*dnsclient.Client, context.Context, string) ([]T, error),
	opts ...dnsclient.Option,
) ([]T, error) {
	values, err := lookup(server.Client(opts...), ctx, name)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", op, "name", name, "type", qtype).
			Wrapf(err, "lookup records")
	}

	return values, nil
}
