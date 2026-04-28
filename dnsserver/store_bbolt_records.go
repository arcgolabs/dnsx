package dnsserver

import (
	"context"
	"slices"

	"github.com/arcgolabs/storx/bboltx"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

func (s *BboltStore) ListRecords(ctx context.Context, filter RecordFilter) ([]Record, error) {
	normalizedFilter, err := normalizeRecordFilter(filter)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "list_records", "zone", filter.Zone, "name", filter.Name, "type", filter.Type, "class", filter.Class).
			Wrapf(err, "normalize record filter")
	}

	records, err := s.scanRecordsByPrefix(ctx, recordListPrefix(normalizedFilter))
	if err != nil {
		return nil, err
	}

	return filterAndSortRecords(records, normalizedFilter), nil
}

func (s *BboltStore) Lookup(ctx context.Context, zone, name string, qtype, qclass uint16) ([]Record, error) {
	normalizedZone, normalizedName, err := normalizeLookup(zone, name)
	if err != nil {
		return nil, err
	}

	return s.lookupByPrefix(ctx, RecordPrefix(normalizedZone, normalizedName, qtype), qclass)
}

func (s *BboltStore) LookupAll(ctx context.Context, zone, name string, qclass uint16) ([]Record, error) {
	normalizedZone, normalizedName, err := normalizeLookup(zone, name)
	if err != nil {
		return nil, err
	}

	return s.lookupByPrefix(ctx, RecordPrefix(normalizedZone, normalizedName, dns.TypeANY), qclass)
}

func (s *BboltStore) lookupByPrefix(ctx context.Context, prefix string, qclass uint16) ([]Record, error) {
	records, err := s.scanRecordsByPrefix(ctx, prefix)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "lookup_by_prefix", "prefix", prefix, "class", qclass).
			Wrapf(err, "lookup records by prefix")
	}

	filtered := lo.Filter(records, func(record Record, _ int) bool {
		return qclass == dns.ClassANY || record.Class == qclass
	})
	slices.SortFunc(filtered, compareRecordsByData)

	return filtered, nil
}

func (s *BboltStore) scanRecordsByPrefix(ctx context.Context, prefix string) ([]Record, error) {
	records := make([]Record, 0)
	if err := s.records.View(ctx, func(tx bboltx.ViewTx[string, Record]) error {
		if prefix == "" {
			return tx.ForEach(func(_ string, record Record) error {
				records = append(records, record)
				return nil
			})
		}

		return tx.ScanPrefix([]byte(prefix), func(_ string, record Record) error {
			records = append(records, record)
			return nil
		})
	}); err != nil {
		return nil, oops.In("dnsserver").
			With("op", "scan_records_by_prefix", "prefix", prefix).
			Wrapf(err, "scan records by prefix")
	}

	return records, nil
}

func filterAndSortRecords(records []Record, filter RecordFilter) []Record {
	filtered := lo.Filter(records, func(record Record, _ int) bool {
		return recordMatchesFilter(record, filter)
	})

	slices.SortFunc(filtered, compareRecords)
	return filtered
}

func compareRecords(left, right Record) int {
	switch {
	case left.Zone < right.Zone:
		return -1
	case left.Zone > right.Zone:
		return 1
	case left.Name < right.Name:
		return -1
	case left.Name > right.Name:
		return 1
	case left.Type < right.Type:
		return -1
	case left.Type > right.Type:
		return 1
	case left.Data < right.Data:
		return -1
	case left.Data > right.Data:
		return 1
	default:
		return 0
	}
}

func compareRecordsByData(left, right Record) int {
	switch {
	case left.Data < right.Data:
		return -1
	case left.Data > right.Data:
		return 1
	default:
		return 0
	}
}

func normalizeLookup(zone, name string) (string, string, error) {
	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return "", "", oops.In("dnsserver").
			With("op", "normalize_lookup", "zone", zone, "name", name).
			Wrapf(err, "normalize lookup zone")
	}

	normalizedName := dns.Fqdn(name)
	if !dns.IsSubDomain(normalizedZone, normalizedName) {
		return "", "", oops.In("dnsserver").
			With("op", "normalize_lookup", "zone", normalizedZone, "name", normalizedName).
			Errorf("lookup name %q is outside zone %q", normalizedName, normalizedZone)
	}

	return normalizedZone, normalizedName, nil
}
