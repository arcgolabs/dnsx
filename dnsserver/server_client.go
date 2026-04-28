package dnsserver

import (
	"context"
	"time"

	"github.com/arcgolabs/dnsx/dnsclient"
	"github.com/miekg/dns"
	"github.com/samber/oops"
)

func (s *Server) Client(opts ...dnsclient.Option) *dnsclient.Client {
	if s == nil {
		return dnsclient.NewClient("127.0.0.1:5354", opts...)
	}

	base := append([]dnsclient.Option(nil), s.clientOpts...)
	if s.logger != nil {
		base = append(base, dnsclient.WithLogger(s.logger))
	}

	return dnsclient.NewClient(s.clientTarget(), append(base, opts...)...)
}

func (s *Server) Query(ctx context.Context, name string, qtype uint16, opts ...dnsclient.Option) (*dns.Msg, time.Duration, error) {
	response, rtt, err := s.Client(opts...).Exchange(ctx, name, qtype)
	if err != nil {
		return nil, 0, oops.In("dnsserver").
			With("op", "server_query", "name", name, "type", qtype).
			Wrapf(err, "query dns server")
	}

	return response, rtt, nil
}

func (s *Server) LookupA(ctx context.Context, name string, opts ...dnsclient.Option) ([]string, error) {
	answers, err := s.Client(opts...).LookupA(ctx, name)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "server_lookup_a", "name", name).
			Wrapf(err, "lookup A records")
	}

	return answers, nil
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

	response, rtt, err := s.Client(opts...).UpdateAdd(ctx, normalized.Zone, rr)
	if err != nil {
		return nil, 0, oops.In("dnsserver").
			With("op", "server_upsert_record", "zone", normalized.Zone, "name", normalized.Name, "type", normalized.Type).
			Wrapf(err, "upsert record via dns update")
	}

	return response, rtt, nil
}

func (s *Server) UpsertRRSet(
	ctx context.Context,
	zone, name string,
	rrtype uint16,
	records []Record,
	opts ...dnsclient.Option,
) (*dns.Msg, time.Duration, error) {
	normalizedRecords, err := normalizeRRSetRecords(zone, name, rrtype, records)
	if err != nil {
		return nil, 0, err
	}

	rrs, err := recordsToRRs(normalizedRecords)
	if err != nil {
		return nil, 0, err
	}

	response, rtt, err := s.Client(opts...).UpdateAdd(ctx, normalizedRecords[0].Zone, rrs...)
	if err != nil {
		return nil, 0, oops.In("dnsserver").
			With("op", "server_upsert_rrset", "zone", normalizedRecords[0].Zone, "name", normalizedRecords[0].Name, "type", rrtype).
			Wrapf(err, "upsert rrset via dns update")
	}

	return response, rtt, nil
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

	response, rtt, err := s.Client(opts...).UpdateRemove(ctx, normalized.Zone, rr)
	if err != nil {
		return nil, 0, oops.In("dnsserver").
			With("op", "server_delete_record", "zone", normalized.Zone, "name", normalized.Name, "type", normalized.Type).
			Wrapf(err, "delete record via dns update")
	}

	return response, rtt, nil
}

func (s *Server) DeleteRRSet(ctx context.Context, zone, name string, rrtype uint16, opts ...dnsclient.Option) (*dns.Msg, time.Duration, error) {
	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return nil, 0, err
	}

	response, rtt, err := s.Client(opts...).UpdateRemoveRRSet(ctx, normalizedZone, dns.Fqdn(name), rrtype)
	if err != nil {
		return nil, 0, oops.In("dnsserver").
			With("op", "server_delete_rrset", "zone", normalizedZone, "name", name, "type", rrtype).
			Wrapf(err, "delete rrset via dns update")
	}

	return response, rtt, nil
}

func (s *Server) DeleteName(ctx context.Context, zone, name string, opts ...dnsclient.Option) (*dns.Msg, time.Duration, error) {
	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return nil, 0, err
	}

	response, rtt, err := s.Client(opts...).UpdateRemoveName(ctx, normalizedZone, dns.Fqdn(name))
	if err != nil {
		return nil, 0, oops.In("dnsserver").
			With("op", "server_delete_name", "zone", normalizedZone, "name", name).
			Wrapf(err, "delete name via dns update")
	}

	return response, rtt, nil
}
