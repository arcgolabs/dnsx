package dnsserver

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/arcgolabs/collectionx/set"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

type ResolverOption func(*Resolver)

type Resolver struct {
	repo          Repository
	logger        *slog.Logger
	cache         *responseCache
	cacheConfig   CacheConfig
	maxCacheTTL   time.Duration
	negativeTTL   time.Duration
	maxCNAMEChain int
	revisionFunc  func() uint64

	zonesMu      sync.RWMutex
	zones        zoneIndex
	zonesLoaded  bool
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
		repo:          repo,
		logger:        slog.Default(),
		cacheConfig:   DefaultCacheConfig(),
		maxCacheTTL:   30 * time.Second,
		negativeTTL:   10 * time.Second,
		maxCNAMEChain: 8,
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

func WithMaxCNAMEChain(depth int) ResolverOption {
	return func(resolver *Resolver) {
		if depth > 0 {
			resolver.maxCNAMEChain = depth
		}
	}
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

func (r *Resolver) matchZone(ctx context.Context, name string) (mo.Option[string], error) {
	revision := r.revisionFunc()

	r.zonesMu.RLock()
	if r.zonesVersion == revision && r.zonesLoaded {
		zones := r.zones
		r.zonesMu.RUnlock()
		return zones.Match(name), nil
	}
	r.zonesMu.RUnlock()

	zones, err := r.listZones(ctx)
	if err != nil {
		return mo.None[string](), err
	}

	index := newZoneIndex(zones)

	r.zonesMu.Lock()
	r.zones = index
	r.zonesLoaded = true
	r.zonesVersion = revision
	r.zonesMu.Unlock()

	return index.Match(name), nil
}

func (r *Resolver) listZones(ctx context.Context) ([]Zone, error) {
	zones, err := r.repo.ListZones(ctx)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "list_zones_for_resolver").
			Wrapf(err, "list zones")
	}

	return zones, nil
}

func uniqueSortedZoneNames(zones []Zone) []string {
	zoneSet := set.NewOrderedSetWithCapacity[string](len(zones))
	lo.ForEach(zones, func(zone Zone, _ int) {
		zoneSet.Add(zone.Name)
	})

	names := zoneSet.Values()
	slices.SortFunc(names, func(left, right string) int {
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

	return names
}

func recordsToRRs(records []Record) ([]dns.RR, error) {
	rrs := make([]dns.RR, 0, len(records))
	for _, record := range records {
		rr, err := record.RR()
		if err != nil {
			return nil, oops.In("dnsserver").
				With("op", "records_to_rrs", "name", record.Name, "type", record.Type).
				Wrapf(err, "build rr from record")
		}

		rrs = append(rrs, rr)
	}

	return rrs, nil
}

func (response Resolution) IsNegative() bool {
	return response.RCode == dns.RcodeNameError || (response.RCode == dns.RcodeSuccess && len(response.Answer) == 0)
}

func CombineErrors(errs ...error) error {
	return errors.Join(errs...)
}
