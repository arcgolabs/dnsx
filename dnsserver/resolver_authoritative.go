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

	return r.successResolution(ctx, zone, qclass, answer)
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

	resolution, err := r.successResolution(ctx, zone, qclass, answer)
	if err != nil {
		return Resolution{}, false, err
	}

	return resolution, true, nil
}

func (r *Resolver) resolveCNAMEChain(ctx context.Context, zone, name string, qtype, qclass uint16) (Resolution, error) {
	current := name
	visited := map[string]struct{}{current: {}}
	answer := make([]dns.RR, 0)

	for range r.maxCNAMEChain {
		step, err := r.nextCNAMEStep(ctx, zone, current, qtype, qclass)
		if err != nil {
			return Resolution{}, err
		}
		if !step.found {
			return r.emptyCNAMEStepResponse(ctx, zone, name, current, qclass, answer)
		}

		answer = append(answer, step.answer...)
		if step.done {
			return r.successResolution(ctx, zone, qclass, answer)
		}
		if seenCNAME(step.target, visited) {
			return serverFailureResolution(), nil
		}

		current = step.target
	}

	return serverFailureResolution(), nil
}

type cnameStep struct {
	found  bool
	done   bool
	target string
	answer []dns.RR
}

func (r *Resolver) nextCNAMEStep(ctx context.Context, zone, current string, qtype, qclass uint16) (cnameStep, error) {
	cnames, err := r.lookup(ctx, zone, current, dns.TypeCNAME, qclass)
	if err != nil {
		return cnameStep{}, err
	}
	if len(cnames) == 0 {
		return cnameStep{}, nil
	}

	answer, err := recordsToRRs(cnames)
	if err != nil {
		return cnameStep{}, err
	}

	target := lo.LastOrEmpty(cnames).CNAME()
	if target == "" || qtype == dns.TypeCNAME || !dns.IsSubDomain(zone, target) {
		return cnameStep{found: true, done: true, answer: answer}, nil
	}

	targetAnswer, err := r.lookupTargetAnswer(ctx, zone, target, qtype, qclass)
	if err != nil {
		return cnameStep{}, err
	}
	if len(targetAnswer) > 0 {
		return cnameStep{found: true, done: true, answer: append(answer, targetAnswer...)}, nil
	}

	return cnameStep{found: true, target: target, answer: answer}, nil
}

func (r *Resolver) lookupTargetAnswer(ctx context.Context, zone, target string, qtype, qclass uint16) ([]dns.RR, error) {
	targetRecords, err := r.lookup(ctx, zone, target, qtype, qclass)
	if err != nil {
		return nil, err
	}
	if len(targetRecords) == 0 {
		return nil, nil
	}

	return recordsToRRs(targetRecords)
}

func (r *Resolver) emptyCNAMEStepResponse(
	ctx context.Context,
	zone, originalName, current string,
	qclass uint16,
	answer []dns.RR,
) (Resolution, error) {
	if current == originalName {
		return r.negativeResponse(ctx, zone, originalName, qclass)
	}

	return r.cnameTerminalNoDataResponse(ctx, zone, qclass, answer)
}

func seenCNAME(target string, visited map[string]struct{}) bool {
	if _, ok := visited[target]; ok {
		return true
	}

	visited[target] = struct{}{}
	return false
}

func (r *Resolver) successResolution(ctx context.Context, zone string, qclass uint16, answer []dns.RR) (Resolution, error) {
	extra, err := r.additionalRecords(ctx, zone, qclass, answer)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{
		RCode:         dns.RcodeSuccess,
		Answer:        answer,
		Extra:         extra,
		Authoritative: true,
	}, nil
}

func serverFailureResolution() Resolution {
	return Resolution{
		RCode:         dns.RcodeServerFailure,
		Authoritative: true,
	}
}

func (r *Resolver) cnameTerminalNoDataResponse(ctx context.Context, zone string, qclass uint16, answer []dns.RR) (Resolution, error) {
	authority, err := r.authoritySOA(ctx, zone, qclass)
	if err != nil {
		return Resolution{}, err
	}
	extra, err := r.additionalRecords(ctx, zone, qclass, answer)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{
		RCode:         dns.RcodeSuccess,
		Answer:        answer,
		Authority:     authority,
		Extra:         extra,
		Authoritative: true,
	}, nil
}

func (r *Resolver) negativeResponse(ctx context.Context, zone, name string, qclass uint16) (Resolution, error) {
	all, err := r.lookupAll(ctx, zone, name, qclass)
	if err != nil {
		return Resolution{}, err
	}

	authority, err := r.authoritySOA(ctx, zone, qclass)
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

func (r *Resolver) authoritySOA(ctx context.Context, zone string, qclass uint16) ([]dns.RR, error) {
	soa, err := r.lookup(ctx, zone, zone, dns.TypeSOA, qclass)
	if err != nil {
		return nil, err
	}

	authority, err := recordsToRRs(soa)
	if err != nil {
		return nil, err
	}

	return authority, nil
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
