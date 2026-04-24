package dnsserver

import (
	"time"

	"github.com/miekg/dns"
	"github.com/samber/lo"
)

func freezeResolution(resolution Resolution) cachedResponse {
	return cachedResponse{
		RCode:         resolution.RCode,
		Answer:        lo.Map(resolution.Answer, func(rr dns.RR, _ int) string { return rr.String() }),
		Authority:     lo.Map(resolution.Authority, func(rr dns.RR, _ int) string { return rr.String() }),
		Extra:         lo.Map(resolution.Extra, func(rr dns.RR, _ int) string { return rr.String() }),
		Authoritative: resolution.Authoritative,
	}
}

func (r *Resolver) cacheTTLFor(resolution Resolution) time.Duration {
	minTTL := uint32(0)
	all := append(append([]dns.RR(nil), resolution.Answer...), resolution.Authority...)
	all = append(all, resolution.Extra...)

	for _, rr := range all {
		ttl := rr.Header().Ttl
		if minTTL == 0 || ttl < minTTL {
			minTTL = ttl
		}
	}

	if minTTL == 0 {
		return r.negativeTTL
	}

	ttl := time.Duration(minTTL) * time.Second
	if r.maxCacheTTL > 0 && ttl > r.maxCacheTTL {
		return r.maxCacheTTL
	}

	return ttl
}
