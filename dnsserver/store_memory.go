package dnsserver

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type MemoryStore struct {
	zones    *mapping.ShardedConcurrentMap[string, Zone]
	records  *mapping.ConcurrentTable[string, string, Record]
	revision atomic.Uint64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		zones:   mapping.NewShardedConcurrentMap[string, Zone](32, mapping.HashString),
		records: mapping.NewConcurrentTable[string, string, Record](),
	}
}

func (s *MemoryStore) Revision() uint64 {
	if s == nil {
		return 0
	}

	return s.revision.Load()
}

func (s *MemoryStore) Close() error {
	return nil
}

func (s *MemoryStore) SaveZone(_ context.Context, zone Zone) error {
	normalized, err := NormalizeZoneName(zone.Name)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "memory_save_zone", "zone", zone.Name).
			Wrapf(err, "normalize zone")
	}

	s.zones.Set(normalized, Zone{Name: normalized})
	s.revision.Add(1)
	return nil
}

func (s *MemoryStore) DeleteZone(_ context.Context, zone string) error {
	normalized, err := NormalizeZoneName(zone)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "memory_delete_zone", "zone", zone).
			Wrapf(err, "normalize zone")
	}

	s.zones.Delete(normalized)
	s.deleteRecordRowsByZone(normalized)
	s.revision.Add(1)
	return nil
}

func (s *MemoryStore) ListZones(_ context.Context) ([]Zone, error) {
	zones := s.zones.Values()
	slices.SortFunc(zones, func(left, right Zone) int {
		return strings.Compare(left.Name, right.Name)
	})

	return zones, nil
}

func (s *MemoryStore) SaveRecord(_ context.Context, record Record) error {
	normalized, err := NormalizeRecord(record)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "memory_save_record", "zone", record.Zone, "name", record.Name, "type", record.Type).
			Wrapf(err, "normalize record")
	}

	s.zones.Set(normalized.Zone, Zone{Name: normalized.Zone})
	s.records.Put(recordNameRowKey(normalized.Zone, normalized.Name), normalized.Key(), normalized)
	s.revision.Add(1)
	return nil
}

func (s *MemoryStore) DeleteRecord(_ context.Context, record Record) error {
	normalized, err := NormalizeRecord(record)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "memory_delete_record", "zone", record.Zone, "name", record.Name, "type", record.Type).
			Wrapf(err, "normalize record")
	}

	s.records.Delete(recordNameRowKey(normalized.Zone, normalized.Name), normalized.Key())
	s.revision.Add(1)
	return nil
}

func (s *MemoryStore) ListRecords(_ context.Context, filter RecordFilter) ([]Record, error) {
	normalizedFilter, err := NormalizeRecordFilter(filter)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "memory_list_records", "zone", filter.Zone, "name", filter.Name, "type", filter.Type, "class", filter.Class).
			Wrapf(err, "normalize record filter")
	}

	records := lo.Filter(s.recordCandidates(normalizedFilter), func(record Record, _ int) bool {
		return RecordMatchesFilter(record, normalizedFilter)
	})
	slices.SortFunc(records, CompareRecords)

	return records, nil
}

func (s *MemoryStore) Lookup(_ context.Context, zone, name string, qtype, qclass uint16) ([]Record, error) {
	normalizedZone, normalizedName, err := NormalizeLookup(zone, name)
	if err != nil {
		return nil, err
	}

	return s.lookupRecords(normalizedZone, normalizedName, qtype, qclass), nil
}

func (s *MemoryStore) LookupAll(_ context.Context, zone, name string, qclass uint16) ([]Record, error) {
	normalizedZone, normalizedName, err := NormalizeLookup(zone, name)
	if err != nil {
		return nil, err
	}

	return s.lookupRecords(normalizedZone, normalizedName, dns.TypeANY, qclass), nil
}

func (s *MemoryStore) lookupRecords(zone, name string, qtype, qclass uint16) []Record {
	row := s.records.Row(recordNameRowKey(zone, name))
	values := lo.Filter(lo.Values(row), func(record Record, _ int) bool {
		return (qtype == dns.TypeANY || record.Type == qtype) && (qclass == dns.ClassANY || record.Class == qclass)
	})
	slices.SortFunc(values, CompareRecordsByData)

	return values
}

func (s *MemoryStore) recordCandidates(filter RecordFilter) []Record {
	if filter.Zone != "" && filter.Name != "" {
		return lo.Values(s.records.Row(recordNameRowKey(filter.Zone, filter.Name)))
	}

	if filter.Zone != "" {
		records := make([]Record, 0)
		rowPrefix := filter.Zone + "|"
		for _, rowKey := range s.records.RowKeys() {
			if strings.HasPrefix(rowKey, rowPrefix) {
				records = append(records, lo.Values(s.records.Row(rowKey))...)
			}
		}
		return records
	}

	records := make([]Record, 0, s.records.Len())
	s.records.Range(func(_ string, _ string, record Record) bool {
		records = append(records, record)
		return true
	})

	return records
}

func (s *MemoryStore) deleteRecordRowsByZone(zone string) {
	rowPrefix := zone + "|"
	for _, rowKey := range s.records.RowKeys() {
		if strings.HasPrefix(rowKey, rowPrefix) {
			s.records.DeleteRow(rowKey)
		}
	}
}

func recordNameRowKey(zone, name string) string {
	return RecordPrefix(zone, name, dns.TypeANY)
}

var _ Repository = (*MemoryStore)(nil)
var _ Revisioner = (*MemoryStore)(nil)
var _ interface{ Close() error } = (*MemoryStore)(nil)
