package dnsserver

import (
	"context"
	"strings"

	"github.com/arcgolabs/collectionx/set"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

func (m *Manager) GetZone(ctx context.Context, name string) (mo.Option[Zone], error) {
	if err := m.requireRepository("manager_get_zone"); err != nil {
		return mo.None[Zone](), err
	}

	normalizedName, err := NormalizeZoneName(name)
	if err != nil {
		return mo.None[Zone](), oops.In("dnsserver").
			With("op", "manager_get_zone", "zone", name).
			Wrapf(err, "normalize zone")
	}

	zones, err := m.ListZones(ctx)
	if err != nil {
		return mo.None[Zone](), err
	}

	for _, zone := range zones {
		if zone.Name == normalizedName {
			return mo.Some(zone), nil
		}
	}

	return mo.None[Zone](), nil
}

func (m *Manager) HasZone(ctx context.Context, name string) (bool, error) {
	zone, err := m.GetZone(ctx, name)
	if err != nil {
		return false, err
	}

	return zone.IsPresent(), nil
}

func (m *Manager) GetRecords(ctx context.Context, filter RecordFilter) ([]Record, error) {
	return m.ListRecords(ctx, filter)
}

func (m *Manager) UpsertRRSet(
	ctx context.Context,
	zone, name string,
	rrtype uint16,
	records []Record,
) ([]Record, error) {
	if err := m.requireRepository("manager_upsert_rrset"); err != nil {
		return nil, err
	}

	normalizedRecords, err := normalizeRRSetRecords(zone, name, rrtype, records)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "manager_upsert_rrset", "zone", zone, "name", name, "type", rrtype).
			Wrapf(err, "normalize rrset records")
	}
	if err := m.validateRRSetUpsert(ctx, normalizedRecords); err != nil {
		return nil, oops.In("dnsserver").
			With("op", "manager_upsert_rrset", "zone", zone, "name", name, "type", rrtype).
			Wrapf(err, "validate rrset upsert")
	}

	normalizedZone := normalizedRecords[0].Zone
	normalizedName := normalizedRecords[0].Name
	if _, deleteErr := m.DeleteRRSet(ctx, normalizedZone, normalizedName, rrtype); deleteErr != nil {
		return nil, oops.In("dnsserver").
			With("op", "manager_upsert_rrset", "zone", normalizedZone, "name", normalizedName, "type", rrtype).
			Wrapf(deleteErr, "replace existing rrset")
	}

	savedRecords := make([]Record, 0, len(normalizedRecords))
	for _, record := range normalizedRecords {
		savedRecord, saveErr := m.UpsertRecord(ctx, record)
		if saveErr != nil {
			return nil, oops.In("dnsserver").
				With("op", "manager_upsert_rrset", "zone", normalizedZone, "name", normalizedName, "type", rrtype).
				Wrapf(saveErr, "save rrset record")
		}
		savedRecords = append(savedRecords, savedRecord)
	}

	m.logger.Debug("dns rrset upserted", "zone", normalizedZone, "name", normalizedName, "type", rrtype, "records", len(savedRecords))
	return savedRecords, nil
}

func (m *Manager) DeleteRRSet(ctx context.Context, zone, name string, rrtype uint16) (int, error) {
	if err := m.requireRepository("manager_delete_rrset"); err != nil {
		return 0, err
	}

	normalizedZone, normalizedName, err := normalizeRRSetLookup(zone, name)
	if err != nil {
		return 0, oops.In("dnsserver").
			With("op", "manager_delete_rrset", "zone", zone, "name", name, "type", rrtype).
			Wrapf(err, "normalize rrset lookup")
	}

	records, err := m.repo.Lookup(ctx, normalizedZone, normalizedName, rrtype, dns.ClassANY)
	if err != nil {
		return 0, oops.In("dnsserver").
			With("op", "manager_delete_rrset", "zone", normalizedZone, "name", normalizedName, "type", rrtype).
			Wrapf(err, "lookup rrset")
	}

	deleted, err := m.deleteRecords(ctx, records, "manager_delete_rrset")
	if err != nil {
		return 0, err
	}

	m.logger.Debug("dns rrset deleted", "zone", normalizedZone, "name", normalizedName, "type", rrtype, "records", deleted)
	return deleted, nil
}

func (m *Manager) DeleteName(ctx context.Context, zone, name string) (int, error) {
	if err := m.requireRepository("manager_delete_name"); err != nil {
		return 0, err
	}

	normalizedZone, normalizedName, err := normalizeRRSetLookup(zone, name)
	if err != nil {
		return 0, oops.In("dnsserver").
			With("op", "manager_delete_name", "zone", zone, "name", name).
			Wrapf(err, "normalize name lookup")
	}

	records, err := m.repo.LookupAll(ctx, normalizedZone, normalizedName, dns.ClassANY)
	if err != nil {
		return 0, oops.In("dnsserver").
			With("op", "manager_delete_name", "zone", normalizedZone, "name", normalizedName).
			Wrapf(err, "lookup name records")
	}

	deleted, err := m.deleteRecords(ctx, records, "manager_delete_name")
	if err != nil {
		return 0, err
	}

	m.logger.Debug("dns name deleted", "zone", normalizedZone, "name", normalizedName, "records", deleted)
	return deleted, nil
}

func (m *Manager) requireRepository(op string) error {
	if m == nil || m.repo == nil {
		return errorBuilder(op, CodeRepositoryNotConfigured).Wrap(ErrRepositoryNotConfigured)
	}

	return nil
}

func (m *Manager) deleteRecords(ctx context.Context, records []Record, op string) (int, error) {
	deleted := 0
	for _, record := range records {
		if err := m.DeleteRecord(ctx, record); err != nil {
			return 0, oops.In("dnsserver").
				With("op", op, "zone", record.Zone, "name", record.Name, "type", record.Type).
				Wrapf(err, "delete managed record")
		}
		deleted++
	}

	return deleted, nil
}

func normalizeRRSetLookup(zone, name string) (string, string, error) {
	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return "", "", oops.In("dnsserver").
			With("op", "normalize_rrset_lookup", "zone", zone, "name", name).
			Wrapf(err, "normalize rrset zone")
	}

	normalizedName := dns.Fqdn(strings.TrimSpace(strings.ToLower(name)))
	if normalizedName == "." {
		return "", "", errorBuilder("normalize_rrset_lookup", CodeRRSetNameRequired, "zone", normalizedZone).
			Wrap(ErrRRSetNameRequired)
	}
	if !dns.IsSubDomain(normalizedZone, normalizedName) {
		return "", "", errorBuilder("normalize_rrset_lookup", CodeRecordOutOfZone, "zone", normalizedZone, "name", normalizedName).
			Wrapf(ErrRecordOutOfZone, "rrset %q is outside zone %q", normalizedName, normalizedZone)
	}

	return normalizedZone, normalizedName, nil
}

func normalizeRRSetRecords(zone, name string, rrtype uint16, records []Record) ([]Record, error) {
	if len(records) == 0 {
		return nil, errorBuilder("normalize_rrset_records", CodeRRSetRecordsRequired, "zone", zone, "name", name, "type", rrtype).
			Wrap(ErrRRSetRecordsRequired)
	}

	normalizedZone, normalizedName, err := normalizeRRSetLookup(zone, name)
	if err != nil {
		return nil, err
	}
	if rrtype == 0 {
		return nil, errorBuilder("normalize_rrset_records", CodeRRSetTypeRequired, "zone", normalizedZone, "name", normalizedName).
			Wrap(ErrRRSetTypeRequired)
	}

	recordKeys := set.NewOrderedSetWithCapacity[string](len(records))
	normalizedRecords := make([]Record, 0, len(records))
	for _, record := range records {
		normalizedRecord, normalizeErr := normalizeRRSetRecord(record, normalizedZone, normalizedName, rrtype)
		if normalizeErr != nil {
			return nil, normalizeErr
		}

		if !recordKeys.Contains(normalizedRecord.Key()) {
			recordKeys.Add(normalizedRecord.Key())
			normalizedRecords = append(normalizedRecords, normalizedRecord)
		}
	}

	return normalizedRecords, nil
}

func normalizeRRSetRecord(record Record, zone, name string, rrtype uint16) (Record, error) {
	candidate := record
	candidate.Zone = lo.Ternary(candidate.Zone == "", zone, candidate.Zone)
	candidate.Name = lo.Ternary(candidate.Name == "", name, candidate.Name)
	candidate.Type = lo.Ternary(candidate.Type == 0, rrtype, candidate.Type)

	normalizedRecord, err := NormalizeRecord(candidate)
	if err != nil {
		return Record{}, oops.In("dnsserver").
			With("op", "normalize_rrset_record", "zone", candidate.Zone, "name", candidate.Name, "type", candidate.Type).
			Wrapf(err, "normalize rrset record")
	}
	if normalizedRecord.Zone != zone || normalizedRecord.Name != name || normalizedRecord.Type != rrtype {
		return Record{}, errorBuilder("normalize_rrset_record", CodeRRSetMismatch, "zone", zone, "name", name, "type", rrtype).
			Wrapf(ErrRRSetMismatch, "rrset record %q/%q/%d does not match target rrset", normalizedRecord.Zone, normalizedRecord.Name, normalizedRecord.Type)
	}

	return normalizedRecord, nil
}
