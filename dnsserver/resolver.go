package dnsserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/mo"
)

type ResolverOption func(*Resolver)

type Resolver struct {
	repo         Repository
	logger       *slog.Logger
	cache        *responseCache
	cacheConfig  CacheConfig
	maxCacheTTL  time.Duration
	negativeTTL  time.Duration
	revisionFunc func() uint64

	zonesMu      sync.RWMutex
	zones        []string
	zonesVersion uint64
}

type Resolution struct {
	RCode         int
	Answer        []dns.RR
	Authority     []dns.RR
	Extra         []dns.RR
	Authoritative bool
}

type queryCacheKey struct {
	Revision uint64
	Name     string
	Type     uint16
	Class    uint16
}

type cachedResponse struct {
	RCode         int
	Answer        []string
	Authority     []string
	Extra         []string
	Authoritative bool
}

func NewResolver(repo Repository, opts ...ResolverOption) *Resolver {
	resolver := &Resolver{
		repo:        repo,
		logger:      slog.Default(),
		cacheConfig: DefaultCacheConfig(),
		maxCacheTTL: 30 * time.Second,
		negativeTTL: 10 * time.Second,
		revisionFunc: func() uint64 {
			return 0
		},
	}
	resolver.cache = newResponseCache(resolver.cacheConfig)

	if revisioner, ok := repo.(Revisioner); ok {
		resolver.revisionFunc = revisioner.Revision
	}

	for _, opt := range opts {
		if opt != nil {
			opt(resolver)
		}
	}

	return resolver
}

func (r *Resolver) Repository() Repository {
	if r == nil {
		return nil
	}

	return r.repo
}

func WithResolverLogger(logger *slog.Logger) ResolverOption {
	return func(resolver *Resolver) {
		if logger != nil {
			resolver.logger = logger
		}
	}
}

func WithHotCache(capacity int, maxTTL time.Duration) ResolverOption {
	return func(resolver *Resolver) {
		config := resolver.cacheConfig
		config.Capacity = capacity
		resolver.cacheConfig = config
		resolver.cache = newResponseCache(config)
		if maxTTL > 0 {
			resolver.maxCacheTTL = maxTTL
		}
	}
}

func WithCache(config CacheConfig) ResolverOption {
	return func(resolver *Resolver) {
		if config.Capacity <= 0 {
			config.Capacity = resolver.cacheConfig.Capacity
		}
		if config.Algorithm == "" {
			config.Algorithm = resolver.cacheConfig.Algorithm
		}
		resolver.cacheConfig = config
		resolver.cache = newResponseCache(config)
	}
}

func WithMaxCacheTTL(ttl time.Duration) ResolverOption {
	return func(resolver *Resolver) {
		if ttl > 0 {
			resolver.maxCacheTTL = ttl
		}
	}
}

func WithNegativeCacheTTL(ttl time.Duration) ResolverOption {
	return func(resolver *Resolver) {
		if ttl > 0 {
			resolver.negativeTTL = ttl
		}
	}
}

func (r *Resolver) ServeDNS(writer dns.ResponseWriter, request *dns.Msg) {
	reply := new(dns.Msg)
	reply.SetReply(request)
	reply.Authoritative = false
	reply.RecursionAvailable = false

	if request.Opcode != dns.OpcodeQuery || len(request.Question) == 0 {
		reply.Rcode = dns.RcodeFormatError
		_ = writer.WriteMsg(reply)
		return
	}

	resolution, err := r.Resolve(context.Background(), request.Question[0])
	if err != nil {
		r.logger.Error("dns resolve failed", "err", err, "name", request.Question[0].Name, "type", request.Question[0].Qtype)
		reply.Rcode = dns.RcodeServerFailure
		_ = writer.WriteMsg(reply)
		return
	}

	reply.Rcode = resolution.RCode
	reply.Authoritative = resolution.Authoritative
	reply.Answer = resolution.Answer
	reply.Ns = resolution.Authority
	reply.Extra = resolution.Extra
	_ = writer.WriteMsg(reply)
}

func (r *Resolver) Resolve(ctx context.Context, question dns.Question) (Resolution, error) {
	name := dns.Fqdn(strings.ToLower(strings.TrimSpace(question.Name)))
	if name == "." {
		return Resolution{RCode: dns.RcodeFormatError}, nil
	}

	zoneOption, err := r.matchZone(ctx, name)
	if err != nil {
		return Resolution{}, err
	}

	if zoneOption.IsAbsent() {
		return Resolution{
			RCode:         dns.RcodeRefused,
			Authoritative: false,
		}, nil
	}

	zone, _ := zoneOption.Get()
	cacheKey := queryCacheKey{
		Revision: r.revisionFunc(),
		Name:     name,
		Type:     question.Qtype,
		Class:    question.Qclass,
	}

	if cached, ok := r.cache.Get(cacheKey); ok {
		return cached.materialize()
	}

	resolution, err := r.resolveAuthoritative(ctx, zone, name, question.Qtype, question.Qclass)
	if err != nil {
		return Resolution{}, err
	}

	cacheTTL := r.cacheTTLFor(resolution)
	if cacheTTL > 0 {
		r.cache.Set(cacheKey, freezeResolution(resolution), cacheTTL)
	}

	return resolution, nil
}

func (r *Resolver) resolveAuthoritative(ctx context.Context, zone string, name string, qtype uint16, qclass uint16) (Resolution, error) {
	if qclass != dns.ClassINET && qclass != dns.ClassANY {
		return Resolution{
			RCode:         dns.RcodeNotImplemented,
			Authoritative: true,
		}, nil
	}

	if qtype == dns.TypeANY {
		records, err := r.repo.LookupAll(ctx, zone, name, qclass)
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

	exact, err := r.repo.Lookup(ctx, zone, name, qtype, qclass)
	if err != nil {
		return Resolution{}, err
	}
	if len(exact) > 0 {
		answer, err := recordsToRRs(exact)
		if err != nil {
			return Resolution{}, err
		}

		return Resolution{
			RCode:         dns.RcodeSuccess,
			Answer:        answer,
			Authoritative: true,
		}, nil
	}

	cnames, err := r.repo.Lookup(ctx, zone, name, dns.TypeCNAME, qclass)
	if err != nil {
		return Resolution{}, err
	}
	if len(cnames) > 0 {
		answer, err := recordsToRRs(cnames)
		if err != nil {
			return Resolution{}, err
		}

		target := lo.LastOrEmpty(cnames).CNAME()
		if target != "" && qtype != dns.TypeCNAME && dns.IsSubDomain(zone, target) {
			targetRecords, err := r.repo.Lookup(ctx, zone, target, qtype, qclass)
			if err != nil {
				return Resolution{}, err
			}
			if len(targetRecords) > 0 {
				targetAnswer, err := recordsToRRs(targetRecords)
				if err != nil {
					return Resolution{}, err
				}
				answer = append(answer, targetAnswer...)
			}
		}

		return Resolution{
			RCode:         dns.RcodeSuccess,
			Answer:        answer,
			Authoritative: true,
		}, nil
	}

	return r.negativeResponse(ctx, zone, name, qclass)
}

func (r *Resolver) negativeResponse(ctx context.Context, zone string, name string, qclass uint16) (Resolution, error) {
	all, err := r.repo.LookupAll(ctx, zone, name, qclass)
	if err != nil {
		return Resolution{}, err
	}

	soa, err := r.repo.Lookup(ctx, zone, zone, dns.TypeSOA, qclass)
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

func (r *Resolver) matchZone(ctx context.Context, name string) (mo.Option[string], error) {
	revision := r.revisionFunc()

	r.zonesMu.RLock()
	if r.zonesVersion == revision && len(r.zones) > 0 {
		zones := append([]string(nil), r.zones...)
		r.zonesMu.RUnlock()
		return findZone(zones, name), nil
	}
	r.zonesMu.RUnlock()

	zones, err := r.repo.ListZones(ctx)
	if err != nil {
		return mo.None[string](), fmt.Errorf("list zones: %w", err)
	}

	zoneSet := collectionset.NewOrderedSet[string]()
	lo.ForEach(zones, func(zone Zone, _ int) {
		zoneSet.Add(zone.Name)
	})
	names := zoneSet.Values()
	slices.SortFunc(names, func(left string, right string) int {
		switch {
		case len(left) > len(right):
			return -1
		case len(left) < len(right):
			return 1
		case left < right:
			return -1
		case left > right:
			return 1
		default:
			return 0
		}
	})

	r.zonesMu.Lock()
	r.zones = names
	r.zonesVersion = revision
	r.zonesMu.Unlock()

	return findZone(names, name), nil
}

func findZone(zones []string, name string) mo.Option[string] {
	for _, zone := range zones {
		if dns.IsSubDomain(zone, name) {
			return mo.Some(zone)
		}
	}

	return mo.None[string]()
}

func recordsToRRs(records []Record) ([]dns.RR, error) {
	rrs := make([]dns.RR, 0, len(records))
	for _, record := range records {
		rr, err := record.RR()
		if err != nil {
			return nil, fmt.Errorf("build rr for %s type=%d: %w", record.Name, record.Type, err)
		}

		rrs = append(rrs, rr)
	}

	return rrs, nil
}

func freezeResolution(resolution Resolution) cachedResponse {
	return cachedResponse{
		RCode:         resolution.RCode,
		Answer:        lo.Map(resolution.Answer, func(rr dns.RR, _ int) string { return rr.String() }),
		Authority:     lo.Map(resolution.Authority, func(rr dns.RR, _ int) string { return rr.String() }),
		Extra:         lo.Map(resolution.Extra, func(rr dns.RR, _ int) string { return rr.String() }),
		Authoritative: resolution.Authoritative,
	}
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
			return nil, fmt.Errorf("parse cached rr %q: %w", line, err)
		}

		result = append(result, rr)
	}

	return result, nil
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

var _ dns.Handler = (*Resolver)(nil)

func (response Resolution) IsNegative() bool {
	return response.RCode == dns.RcodeNameError || (response.RCode == dns.RcodeSuccess && len(response.Answer) == 0)
}

func CombineErrors(errs ...error) error {
	return errors.Join(errs...)
}
