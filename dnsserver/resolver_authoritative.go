package dnsserver

import (
	"context"

	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

func (r *Resolver) resolveAuthoritative(ctx context.Context, zone, name string, qtype, qclass uint16) (Resolution, error) {
	if qclass != dns.ClassINET && qclass != dns.ClassANY {
		return Resolution{
			RCode:         dns.RcodeNotImplemented,
			Authoritative: true,
		}, nil
	}

	if qtype == dns.TypeANY {
		return r.resolveAny(ctx, zone, name, qclass)
	}

	resolution, matched, err := r.resolveExact(ctx, zone, name, qtype, qclass)
	if err != nil || matched {
		return resolution, err
	}

	return r.resolveCNAMEChain(ctx, zone, name, qtype, qclass)
}

func (r *Resolver) resolveAny(ctx context.Context, zone, name string, qclass uint16) (Resolution, error) {
	records, err := r.lookupAll(ctx, zone, name, qclass)
	if err != nil {
		return Resolution{}, err
	}
	if len(records) == 0 {
		return r.negativeResponse(ctx, zone, name, qclass)
	}

	answer, err := recordsToRRs(records)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{
		RCode:         dns.RcodeSuccess,
		Answer:        answer,
		Authoritative: true,
	}, nil
}

func (r *Resolver) resolveExact(ctx context.Context, zone, name string, qtype, qclass uint16) (Resolution, bool, error) {
	exact, err := r.lookup(ctx, zone, name, qtype, qclass)
	if err != nil {
		return Resolution{}, false, err
	}
	if len(exact) == 0 {
		return Resolution{}, false, nil
	}

	answer, err := recordsToRRs(exact)
	if err != nil {
		return Resolution{}, false, err
	}

	return Resolution{
		RCode:         dns.RcodeSuccess,
		Answer:        answer,
		Authoritative: true,
	}, true, nil
}

func (r *Resolver) resolveCNAMEChain(ctx context.Context, zone, name string, qtype, qclass uint16) (Resolution, error) {
	cnames, err := r.lookup(ctx, zone, name, dns.TypeCNAME, qclass)
	if err != nil {
		return Resolution{}, err
	}
	if len(cnames) == 0 {
		return r.negativeResponse(ctx, zone, name, qclass)
	}

	answer, err := recordsToRRs(cnames)
	if err != nil {
		return Resolution{}, err
	}

	answer, err = r.appendCNAMETargetRecords(ctx, zone, qtype, qclass, cnames, answer)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{
		RCode:         dns.RcodeSuccess,
		Answer:        answer,
		Authoritative: true,
	}, nil
}

func (r *Resolver) appendCNAMETargetRecords(
	ctx context.Context,
	zone string,
	qtype, qclass uint16,
	cnames []Record,
	answer []dns.RR,
) ([]dns.RR, error) {
	target := lo.LastOrEmpty(cnames).CNAME()
	if target == "" || qtype == dns.TypeCNAME || !dns.IsSubDomain(zone, target) {
		return answer, nil
	}

	targetRecords, err := r.lookup(ctx, zone, target, qtype, qclass)
	if err != nil {
		return nil, err
	}
	if len(targetRecords) == 0 {
		return answer, nil
	}

	targetAnswer, err := recordsToRRs(targetRecords)
	if err != nil {
		return nil, err
	}

	return append(answer, targetAnswer...), nil
}

func (r *Resolver) negativeResponse(ctx context.Context, zone, name string, qclass uint16) (Resolution, error) {
	all, err := r.lookupAll(ctx, zone, name, qclass)
	if err != nil {
		return Resolution{}, err
	}

	soa, err := r.lookup(ctx, zone, zone, dns.TypeSOA, qclass)
	if err != nil {
		return Resolution{}, err
	}

	authority, err := recordsToRRs(soa)
	if err != nil {
		return Resolution{}, err
	}

	rcode := dns.RcodeSuccess
	if len(all) == 0 {
		rcode = dns.RcodeNameError
	}

	return Resolution{
		RCode:         rcode,
		Authority:     authority,
		Authoritative: true,
	}, nil
}

func (r *Resolver) lookup(ctx context.Context, zone, name string, qtype, qclass uint16) ([]Record, error) {
	records, err := r.repo.Lookup(ctx, zone, name, qtype, qclass)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "resolver_lookup", "zone", zone, "name", name, "type", qtype, "class", qclass).
			Wrapf(err, "lookup records")
	}

	return records, nil
}

func (r *Resolver) lookupAll(ctx context.Context, zone, name string, qclass uint16) ([]Record, error) {
	records, err := r.repo.LookupAll(ctx, zone, name, qclass)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "resolver_lookup_all", "zone", zone, "name", name, "class", qclass).
			Wrapf(err, "lookup all records")
	}

	return records, nil
}

func (response cachedResponse) materialize() (Resolution, error) {
	answer, err := parseRRStrings(response.Answer)
	if err != nil {
		return Resolution{}, err
	}

	authority, err := parseRRStrings(response.Authority)
	if err != nil {
		return Resolution{}, err
	}

	extra, err := parseRRStrings(response.Extra)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{
		RCode:         response.RCode,
		Answer:        answer,
		Authority:     authority,
		Extra:         extra,
		Authoritative: response.Authoritative,
	}, nil
}

func parseRRStrings(lines []string) ([]dns.RR, error) {
	result := make([]dns.RR, 0, len(lines))
	for _, line := range lines {
		rr, err := dns.NewRR(line)
		if err != nil {
			return nil, oops.In("dnsserver").
				With("op", "parse_rr_strings", "line", line).
				Wrapf(err, "parse cached rr")
		}

		result = append(result, rr)
	}

	return result, nil
}
