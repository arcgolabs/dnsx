package dnsserver

import (
	"context"
	"strings"

	"github.com/miekg/dns"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

func (m *Manager) GetZoneSnapshot(ctx context.Context, name string) (mo.Option[ZoneSnapshot], error) {
	if err := m.requireRepository("manager_get_zone_snapshot"); err != nil {
		return mo.None[ZoneSnapshot](), err
	}

	zone, err := m.GetZone(ctx, name)
	if err != nil {
		return mo.None[ZoneSnapshot](), err
	}
	if zone.IsAbsent() {
		return mo.None[ZoneSnapshot](), nil
	}

	zoneValue := zone.MustGet()
	records, err := m.ListRecords(ctx, RecordFilter{Zone: zoneValue.Name})
	if err != nil {
		return mo.None[ZoneSnapshot](), oops.In("dnsserver").
			With("op", "manager_get_zone_snapshot", "zone", zoneValue.Name).
			Wrapf(err, "list zone records")
	}

	return mo.Some(buildZoneSnapshot(zoneValue.Name, records)), nil
}

func (m *Manager) ListRRSets(ctx context.Context, zone, name string) ([]RRSet, error) {
	if err := m.requireRepository("manager_list_rrsets"); err != nil {
		return nil, err
	}

	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "manager_list_rrsets", "zone", zone, "name", name).
			Wrapf(err, "normalize rrset zone")
	}

	filter, err := normalizeRRSetRecordFilter(normalizedZone, name)
	if err != nil {
		return nil, err
	}

	records, err := m.ListRecords(ctx, filter)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "manager_list_rrsets", "zone", normalizedZone, "name", filter.Name).
			Wrapf(err, "list rrset records")
	}

	return buildRRSets(records), nil
}

func normalizeRRSetRecordFilter(zone, name string) (RecordFilter, error) {
	filter := RecordFilter{Zone: zone}
	if strings.TrimSpace(name) == "" {
		return filter, nil
	}

	normalizedName := dns.Fqdn(strings.TrimSpace(strings.ToLower(name)))
	if !dns.IsSubDomain(zone, normalizedName) {
		return RecordFilter{}, errorBuilder("normalize_rrset_record_filter", CodeRecordOutOfZone, "zone", zone, "name", normalizedName).
			Wrapf(ErrRecordOutOfZone, "rrset %q is outside zone %q", normalizedName, zone)
	}

	filter.Name = normalizedName
	return filter, nil
}
