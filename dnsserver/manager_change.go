package dnsserver

import (
	"context"

	"github.com/samber/oops"
)

type ChangeKind string

const (
	ChangeUpsertZone   ChangeKind = "upsert_zone"
	ChangeDeleteZone   ChangeKind = "delete_zone"
	ChangeUpsertRecord ChangeKind = "upsert_record"
	ChangeDeleteRecord ChangeKind = "delete_record"
	ChangeUpsertRRSet  ChangeKind = "upsert_rrset"
	ChangeDeleteRRSet  ChangeKind = "delete_rrset"
	ChangeDeleteName   ChangeKind = "delete_name"
)

type Change struct {
	Kind     ChangeKind `json:"kind"`
	Zone     Zone       `json:"zone"`
	ZoneName string     `json:"zone_name,omitempty"`
	Record   Record     `json:"record"`
	Records  []Record   `json:"records,omitempty"`
	Name     string     `json:"name,omitempty"`
	Type     uint16     `json:"type,omitempty"`
}

type ChangeResult struct {
	Applied         int `json:"applied"`
	ZonesUpserted   int `json:"zones_upserted"`
	ZonesDeleted    int `json:"zones_deleted"`
	RecordsUpserted int `json:"records_upserted"`
	RecordsDeleted  int `json:"records_deleted"`
}

func (m *Manager) ApplyChanges(ctx context.Context, changes []Change) (ChangeResult, error) {
	if err := m.requireRepository("manager_apply_changes"); err != nil {
		return ChangeResult{}, err
	}
	if err := m.validateChanges(ctx, changes); err != nil {
		return ChangeResult{}, oops.In("dnsserver").
			With("op", "manager_apply_changes", "changes", len(changes)).
			Wrapf(err, "validate changes")
	}

	result := ChangeResult{}
	for index := range changes {
		if err := m.applyChange(ctx, &changes[index], &result); err != nil {
			return ChangeResult{}, err
		}
	}

	return result, nil
}

func (m *Manager) applyChange(ctx context.Context, change *Change, result *ChangeResult) error {
	switch change.Kind {
	case ChangeUpsertZone:
		return m.applyUpsertZoneChange(ctx, change, result)
	case ChangeDeleteZone:
		return m.applyDeleteZoneChange(ctx, change, result)
	case ChangeUpsertRecord:
		return m.applyUpsertRecordChange(ctx, change, result)
	case ChangeDeleteRecord:
		return m.applyDeleteRecordChange(ctx, change, result)
	case ChangeUpsertRRSet:
		return m.applyUpsertRRSetChange(ctx, change, result)
	case ChangeDeleteRRSet:
		return m.applyDeleteRRSetChange(ctx, change, result)
	case ChangeDeleteName:
		return m.applyDeleteNameChange(ctx, change, result)
	default:
		return oops.In("dnsserver").
			With("op", "manager_apply_change", "kind", change.Kind).
			Errorf("unsupported change kind %q", change.Kind)
	}
}

func (m *Manager) applyUpsertZoneChange(ctx context.Context, change *Change, result *ChangeResult) error {
	if _, err := m.UpsertZone(ctx, Zone{Name: change.zoneName()}); err != nil {
		return err
	}

	result.Applied++
	result.ZonesUpserted++
	return nil
}

func (m *Manager) applyDeleteZoneChange(ctx context.Context, change *Change, result *ChangeResult) error {
	if err := m.DeleteZone(ctx, change.zoneName()); err != nil {
		return err
	}

	result.Applied++
	result.ZonesDeleted++
	return nil
}

func (m *Manager) applyUpsertRecordChange(ctx context.Context, change *Change, result *ChangeResult) error {
	if _, err := m.UpsertRecord(ctx, change.Record); err != nil {
		return err
	}

	result.Applied++
	result.RecordsUpserted++
	return nil
}

func (m *Manager) applyDeleteRecordChange(ctx context.Context, change *Change, result *ChangeResult) error {
	if err := m.DeleteRecord(ctx, change.Record); err != nil {
		return err
	}

	result.Applied++
	result.RecordsDeleted++
	return nil
}

func (m *Manager) applyUpsertRRSetChange(ctx context.Context, change *Change, result *ChangeResult) error {
	records, err := m.UpsertRRSet(ctx, change.zoneName(), change.Name, change.Type, change.Records)
	if err != nil {
		return err
	}

	result.Applied++
	result.RecordsUpserted += len(records)
	return nil
}

func (m *Manager) applyDeleteRRSetChange(ctx context.Context, change *Change, result *ChangeResult) error {
	deleted, err := m.DeleteRRSet(ctx, change.zoneName(), change.Name, change.Type)
	if err != nil {
		return err
	}

	result.Applied++
	result.RecordsDeleted += deleted
	return nil
}

func (m *Manager) applyDeleteNameChange(ctx context.Context, change *Change, result *ChangeResult) error {
	deleted, err := m.DeleteName(ctx, change.zoneName(), change.Name)
	if err != nil {
		return err
	}

	result.Applied++
	result.RecordsDeleted += deleted
	return nil
}

func (change Change) zoneName() string {
	if change.Zone.Name != "" {
		return change.Zone.Name
	}

	return change.ZoneName
}
