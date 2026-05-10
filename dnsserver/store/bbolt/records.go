package bbolt

import (
	"context"
	"slices"

	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/arcgolabs/storx/bboltx"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

func (s *Store) ListRecords(ctx context.Context, filter dnsserver.RecordFilter) ([]dnsserver.Record, error) {
	normalizedFilter, err := dnsserver.NormalizeRecordFilter(filter)
	if err != nil {
		return nil, oops.In("dnsserver/store/bbolt").
			With("op", "list_records", "zone", filter.Zone, "name", filter.Name, "type", filter.Type, "class", filter.Class).
			Wrapf(err, "normalize record filter")
	}

	records, err := s.scanRecordsByPrefix(ctx, dnsserver.RecordListPrefix(normalizedFilter))
	if err != nil {
		return nil, err
	}

	return filterAndSortRecords(records, normalizedFilter), nil
}

func (s *Store) Lookup(ctx context.Context, zone, name string, qtype, qclass uint16) ([]dnsserver.Record, error) {
	normalizedZone, normalizedName, err := dnsserver.NormalizeLookup(zone, name)
	if err != nil {
		return nil, oops.In("dnsserver/store/bbolt").
			With("op", "lookup", "zone", zone, "name", name, "type", qtype, "class", qclass).
			Wrapf(err, "normalize lookup")
	}

	return s.lookupByPrefix(ctx, dnsserver.RecordPrefix(normalizedZone, normalizedName, qtype), qclass)
}

func (s *Store) LookupAll(ctx context.Context, zone, name string, qclass uint16) ([]dnsserver.Record, error) {
	normalizedZone, normalizedName, err := dnsserver.NormalizeLookup(zone, name)
	if err != nil {
		return nil, oops.In("dnsserver/store/bbolt").
			With("op", "lookup_all", "zone", zone, "name", name, "class", qclass).
			Wrapf(err, "normalize lookup")
	}

	return s.lookupByPrefix(ctx, dnsserver.RecordPrefix(normalizedZone, normalizedName, dns.TypeANY), qclass)
}

func (s *Store) lookupByPrefix(ctx context.Context, prefix string, qclass uint16) ([]dnsserver.Record, error) {
	records, err := s.scanRecordsByPrefix(ctx, prefix)
	if err != nil {
		return nil, oops.In("dnsserver/store/bbolt").
			With("op", "lookup_by_prefix", "prefix", prefix, "class", qclass).
			Wrapf(err, "lookup records by prefix")
	}

	filtered := lo.Filter(records, func(record dnsserver.Record, _ int) bool {
		return qclass == dns.ClassANY || record.Class == qclass
	})
	slices.SortFunc(filtered, dnsserver.CompareRecordsByData)

	return filtered, nil
}

func (s *Store) scanRecordsByPrefix(ctx context.Context, prefix string) ([]dnsserver.Record, error) {
	options := make([]bboltx.ListOption[string], 0, 1)
	if prefix != "" {
		options = append(options, bboltx.WithPrefix[string]([]byte(prefix)))
	}

	records, err := s.records.Values(ctx, options...)
	if err != nil {
		return nil, oops.In("dnsserver/store/bbolt").
			With("op", "scan_records_by_prefix", "prefix", prefix).
			Wrapf(err, "scan records by prefix")
	}

	return records, nil
}

func filterAndSortRecords(records []dnsserver.Record, filter dnsserver.RecordFilter) []dnsserver.Record {
	filtered := lo.Filter(records, func(record dnsserver.Record, _ int) bool {
		return dnsserver.RecordMatchesFilter(record, filter)
	})

	slices.SortFunc(filtered, dnsserver.CompareRecords)
	return filtered
}
