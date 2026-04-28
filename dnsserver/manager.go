package dnsserver

import (
	"context"
	"log/slog"

	"github.com/samber/lo"
	"github.com/samber/oops"
)

type ManagerOption func(*Manager)

type ImportResult struct {
	Zones   int `json:"zones"`
	Records int `json:"records"`
}

type Manager struct {
	repo   Repository
	logger *slog.Logger
}

func NewManager(repo Repository, opts ...ManagerOption) *Manager {
	manager := &Manager{
		repo:   repo,
		logger: slog.Default(),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(manager)
		}
	}

	return manager
}

func WithManagerLogger(logger *slog.Logger) ManagerOption {
	return func(manager *Manager) {
		if logger != nil {
			manager.logger = logger
		}
	}
}

func (m *Manager) Repository() Repository {
	if m == nil {
		return nil
	}

	return m.repo
}

func (m *Manager) UpsertZone(ctx context.Context, zone Zone) (Zone, error) {
	if err := m.requireRepository("manager_upsert_zone"); err != nil {
		return Zone{}, err
	}

	normalizedName, err := NormalizeZoneName(zone.Name)
	if err != nil {
		return Zone{}, oops.In("dnsserver").
			With("op", "manager_upsert_zone", "zone", zone.Name).
			Wrapf(err, "normalize zone")
	}

	normalizedZone := Zone{Name: normalizedName}
	if err := m.repo.SaveZone(ctx, normalizedZone); err != nil {
		return Zone{}, oops.In("dnsserver").
			With("op", "manager_upsert_zone", "zone", normalizedName).
			Wrapf(err, "save zone")
	}

	m.logger.Debug("dns zone upserted", "zone", normalizedName)
	return normalizedZone, nil
}

func (m *Manager) DeleteZone(ctx context.Context, zone string) error {
	if err := m.requireRepository("manager_delete_zone"); err != nil {
		return err
	}

	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "manager_delete_zone", "zone", zone).
			Wrapf(err, "normalize zone")
	}

	if err := m.repo.DeleteZone(ctx, normalizedZone); err != nil {
		return oops.In("dnsserver").
			With("op", "manager_delete_zone", "zone", normalizedZone).
			Wrapf(err, "delete zone")
	}

	m.logger.Debug("dns zone deleted", "zone", normalizedZone)
	return nil
}

func (m *Manager) ListZones(ctx context.Context) ([]Zone, error) {
	if err := m.requireRepository("manager_list_zones"); err != nil {
		return nil, err
	}

	zones, err := m.repo.ListZones(ctx)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "manager_list_zones").
			Wrapf(err, "list zones")
	}

	return lo.Map(zones, func(zone Zone, _ int) Zone {
		return Zone{Name: zone.Name}
	}), nil
}

func (m *Manager) UpsertRecord(ctx context.Context, record Record) (Record, error) {
	if err := m.requireRepository("manager_upsert_record"); err != nil {
		return Record{}, err
	}

	normalizedRecord, err := NormalizeRecord(record)
	if err != nil {
		return Record{}, oops.In("dnsserver").
			With("op", "manager_upsert_record", "zone", record.Zone, "name", record.Name, "type", record.Type).
			Wrapf(err, "normalize record")
	}
	if err := m.validateRecordUpsert(ctx, normalizedRecord); err != nil {
		return Record{}, oops.In("dnsserver").
			With("op", "manager_upsert_record", "zone", normalizedRecord.Zone, "name", normalizedRecord.Name, "type", normalizedRecord.Type).
			Wrapf(err, "validate record upsert")
	}

	if err := m.repo.SaveRecord(ctx, normalizedRecord); err != nil {
		return Record{}, oops.In("dnsserver").
			With("op", "manager_upsert_record", "zone", normalizedRecord.Zone, "name", normalizedRecord.Name, "type", normalizedRecord.Type).
			Wrapf(err, "save record")
	}

	m.logger.Debug(
		"dns record upserted",
		"zone", normalizedRecord.Zone,
		"name", normalizedRecord.Name,
		"type", normalizedRecord.Type,
	)

	return normalizedRecord, nil
}

func (m *Manager) DeleteRecord(ctx context.Context, record Record) error {
	if err := m.requireRepository("manager_delete_record"); err != nil {
		return err
	}

	normalizedRecord, err := NormalizeRecord(record)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "manager_delete_record", "zone", record.Zone, "name", record.Name, "type", record.Type).
			Wrapf(err, "normalize record")
	}

	if err := m.repo.DeleteRecord(ctx, normalizedRecord); err != nil {
		return oops.In("dnsserver").
			With("op", "manager_delete_record", "zone", normalizedRecord.Zone, "name", normalizedRecord.Name, "type", normalizedRecord.Type).
			Wrapf(err, "delete record")
	}

	m.logger.Debug(
		"dns record deleted",
		"zone", normalizedRecord.Zone,
		"name", normalizedRecord.Name,
		"type", normalizedRecord.Type,
	)

	return nil
}

func (m *Manager) ListRecords(ctx context.Context, filter RecordFilter) ([]Record, error) {
	if err := m.requireRepository("manager_list_records"); err != nil {
		return nil, err
	}

	normalizedFilter, err := normalizeRecordFilter(filter)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "manager_list_records", "zone", filter.Zone, "name", filter.Name, "type", filter.Type, "class", filter.Class).
			Wrapf(err, "normalize record filter")
	}

	records, err := m.repo.ListRecords(ctx, normalizedFilter)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "manager_list_records", "zone", normalizedFilter.Zone, "name", normalizedFilter.Name, "type", normalizedFilter.Type, "class", normalizedFilter.Class).
			Wrapf(err, "list records")
	}

	return lo.Map(records, func(record Record, _ int) Record {
		return record
	}), nil
}

func (m *Manager) ImportSeedData(ctx context.Context, seed SeedData) (ImportResult, error) {
	if err := m.requireRepository("manager_import_seed_data"); err != nil {
		return ImportResult{}, err
	}

	zones, err := collectSeedZones(seed)
	if err != nil {
		return ImportResult{}, oops.In("dnsserver").
			With("op", "manager_import_seed_data").
			Wrapf(err, "collect seed zones")
	}

	records, err := collectSeedRecords(seed, zones)
	if err != nil {
		return ImportResult{}, oops.In("dnsserver").
			With("op", "manager_import_seed_data").
			Wrapf(err, "collect seed records")
	}
	if err := validateRecordsByZone(records, zoneValidationOptions{requireApexNS: true}); err != nil {
		return ImportResult{}, oops.In("dnsserver").
			With("op", "manager_import_seed_data", "records", len(records)).
			Wrapf(err, "validate seed records")
	}

	if err := saveSeedZones(ctx, m.repo, zones.Values()); err != nil {
		return ImportResult{}, oops.In("dnsserver").
			With("op", "manager_import_seed_data", "zones", zones.Len()).
			Wrapf(err, "save seed zones")
	}
	if err := saveSeedRecords(ctx, m.repo, records); err != nil {
		return ImportResult{}, oops.In("dnsserver").
			With("op", "manager_import_seed_data", "records", len(records)).
			Wrapf(err, "save seed records")
	}

	result := ImportResult{
		Zones:   zones.Len(),
		Records: len(records),
	}
	m.logger.Info("dns seed data imported", "zones", result.Zones, "records", result.Records)

	return result, nil
}
