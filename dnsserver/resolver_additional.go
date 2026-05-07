package dnsserver

import (
	"context"
	"strings"

	"github.com/arcgolabs/collectionx/set"
	"github.com/miekg/dns"
)

func (r *Resolver) additionalRecords(ctx context.Context, zone string, qclass uint16, answer []dns.RR) ([]dns.RR, error) {
	targets := additionalTargets(zone, answer)
	if targets.Len() == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(answer))
	for _, rr := range answer {
		seen[rr.String()] = struct{}{}
	}

	extra := make([]dns.RR, 0, targets.Len())
	for _, target := range targets.Values() {
		targetExtra, err := r.lookupAddressRRs(ctx, zone, target, qclass)
		if err != nil {
			return nil, err
		}

		for _, rr := range targetExtra {
			key := rr.String()
			if _, ok := seen[key]; ok {
				continue
			}

			seen[key] = struct{}{}
			extra = append(extra, rr)
		}
	}

	return extra, nil
}

func additionalTargets(zone string, answer []dns.RR) *set.OrderedSet[string] {
	targets := set.NewOrderedSetWithCapacity[string](len(answer))
	for _, rr := range answer {
		target := additionalTarget(rr)
		normalized := normalizeAdditionalTarget(target)
		if normalized == "" || !dns.IsSubDomain(zone, normalized) {
			continue
		}

		targets.Add(normalized)
	}

	return targets
}

func additionalTarget(rr dns.RR) string {
	switch typed := rr.(type) {
	case *dns.CNAME:
		return typed.Target
	case *dns.NS:
		return typed.Ns
	case *dns.MX:
		return typed.Mx
	case *dns.SRV:
		return typed.Target
	default:
		return ""
	}
}

func normalizeAdditionalTarget(target string) string {
	normalized := dns.Fqdn(strings.ToLower(strings.TrimSpace(target)))
	if normalized == "." {
		return ""
	}

	return normalized
}

func (r *Resolver) lookupAddressRRs(ctx context.Context, zone, target string, qclass uint16) ([]dns.RR, error) {
	records := make([]Record, 0)
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		found, err := r.lookup(ctx, zone, target, qtype, qclass)
		if err != nil {
			return nil, err
		}

		records = append(records, found...)
	}
	if len(records) == 0 {
		return nil, nil
	}

	return recordsToRRs(records)
}
