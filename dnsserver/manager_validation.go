package dnsserver

import (
	"context"

	"github.com/arcgolabs/collectionx/set"
	"github.com/samber/oops"
)

type previewZoneState struct {
	exists  bool
	records []Record
}

func (m *Manager) validateRecordUpsert(ctx context.Context, record Record) error {
	records, err := m.previewZoneRecords(ctx, record.Zone, func(current []Record) []Record {
		return upsertRecords(current, []Record{record})
	})
	if err != nil {
		return err
	}

	return validateZoneRecords(record.Zone, records, zoneValidationOptions{requireApexNS: false})
}

func (m *Manager) validateRRSetUpsert(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}

	target := records[0]
	previewRecords, err := m.previewZoneRecords(ctx, target.Zone, func(current []Record) []Record {
		filtered := removeRecords(current, func(record Record) bool {
			return record.Name == target.Name && record.Type == target.Type && record.Class == target.Class
		})
		return upsertRecords(filtered, records)
	})
	if err != nil {
		return err
	}

	return validateZoneRecords(target.Zone, previewRecords, zoneValidationOptions{requireApexNS: false})
}

func (m *Manager) previewZoneRecords(
	ctx context.Context,
	zone string,
	mutate func([]Record) []Record,
) ([]Record, error) {
	records, err := m.ListRecords(ctx, RecordFilter{Zone: zone})
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "manager_preview_zone_records", "zone", zone).
			Wrapf(err, "list preview zone records")
	}

	return mutate(records), nil
}

func (m *Manager) validateChanges(ctx context.Context, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}

	states := make(map[string]*previewZoneState, len(changes))
	touchedZones := set.NewOrderedSet[string]()
	if err := m.previewChanges(ctx, changes, states, touchedZones); err != nil {
		return err
	}
	return validatePreviewZoneStates(states, touchedZones)
}

func (m *Manager) previewChanges(
	ctx context.Context,
	changes []Change,
	states map[string]*previewZoneState,
	touchedZones *set.OrderedSet[string],
) error {
	for index := range changes {
		change := &changes[index]
		if err := m.previewSingleChange(ctx, states, touchedZones, change); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) previewSingleChange(
	ctx context.Context,
	states map[string]*previewZoneState,
	touchedZones *set.OrderedSet[string],
	change *Change,
) error {
	normalizedZone, err := normalizeChangeZone(*change)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "manager_validate_changes", "kind", change.Kind).
			Wrapf(err, "normalize change zone")
	}

	state, err := m.loadPreviewZoneState(ctx, states, normalizedZone)
	if err != nil {
		return err
	}
	touchedZones.Add(normalizedZone)

	if err := applyChangePreview(state, normalizedZone, change); err != nil {
		return oops.In("dnsserver").
			With("op", "manager_validate_changes", "zone", normalizedZone, "kind", change.Kind).
			Wrapf(err, "preview change")
	}

	return nil
}

func validatePreviewZoneStates(
	states map[string]*previewZoneState,
	touchedZones *set.OrderedSet[string],
) error {
	for _, zone := range touchedZones.Values() {
		state := states[zone]
		if state == nil || !state.exists || len(state.records) == 0 {
			continue
		}

		snapshot := buildZoneSnapshot(zone, state.records)
		if err := snapshot.Validate(); err != nil {
			return oops.In("dnsserver").
				With("op", "manager_validate_changes", "zone", zone).
				Wrapf(err, "validate zone snapshot")
		}
	}

	return nil
}

func (m *Manager) loadPreviewZoneState(
	ctx context.Context,
	states map[string]*previewZoneState,
	zone string,
) (*previewZoneState, error) {
	if state, ok := states[zone]; ok {
		return state, nil
	}

	exists, err := m.HasZone(ctx, zone)
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "load_preview_zone_state", "zone", zone).
			Wrapf(err, "check zone existence")
	}

	records, err := m.ListRecords(ctx, RecordFilter{Zone: zone})
	if err != nil {
		return nil, oops.In("dnsserver").
			With("op", "load_preview_zone_state", "zone", zone).
			Wrapf(err, "list zone records")
	}

	state := &previewZoneState{exists: exists, records: records}
	states[zone] = state
	return state, nil
}

func applyChangePreview(state *previewZoneState, zone string, change *Change) error {
	switch change.Kind {
	case ChangeUpsertZone:
		state.exists = true
		return nil
	case ChangeDeleteZone:
		state.exists = false
		state.records = nil
		return nil
	case ChangeUpsertRecord:
		return applyUpsertRecordPreview(state, zone, *change)
	case ChangeDeleteRecord:
		return applyDeleteRecordPreview(state, zone, *change)
	case ChangeUpsertRRSet:
		return applyUpsertRRSetPreview(state, zone, *change)
	case ChangeDeleteRRSet:
		return applyDeleteRRSetPreview(state, zone, *change)
	case ChangeDeleteName:
		return applyDeleteNamePreview(state, zone, *change)
	default:
		return oops.In("dnsserver").
			With("op", "apply_change_preview", "zone", zone, "kind", change.Kind).
			Errorf("unsupported change kind %q", change.Kind)
	}
}

func applyUpsertRecordPreview(state *previewZoneState, zone string, change Change) error {
	record, err := NormalizeRecord(change.Record)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "apply_change_preview", "zone", zone, "kind", change.Kind).
			Wrapf(err, "normalize preview record")
	}

	state.exists = true
	state.records = upsertRecords(state.records, []Record{record})
	return nil
}

func applyDeleteRecordPreview(state *previewZoneState, zone string, change Change) error {
	record, err := NormalizeRecord(change.Record)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "apply_change_preview", "zone", zone, "kind", change.Kind).
			Wrapf(err, "normalize preview record")
	}

	state.records = removeRecords(state.records, func(candidate Record) bool {
		return candidate.Key() == record.Key()
	})
	return nil
}

func applyUpsertRRSetPreview(state *previewZoneState, zone string, change Change) error {
	records, err := normalizeRRSetRecords(zone, change.Name, change.Type, change.Records)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "apply_change_preview", "zone", zone, "kind", change.Kind, "name", change.Name, "type", change.Type).
			Wrapf(err, "normalize preview rrset")
	}

	state.exists = true
	target := records[0]
	filtered := removeRecords(state.records, func(record Record) bool {
		return record.Name == target.Name && record.Type == target.Type && record.Class == target.Class
	})
	state.records = upsertRecords(filtered, records)
	return nil
}

func applyDeleteRRSetPreview(state *previewZoneState, zone string, change Change) error {
	normalizedZone, normalizedName, err := normalizeRRSetLookup(zone, change.Name)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "apply_change_preview", "zone", zone, "kind", change.Kind, "name", change.Name, "type", change.Type).
			Wrapf(err, "normalize preview rrset lookup")
	}

	state.records = removeRecords(state.records, func(record Record) bool {
		return record.Zone == normalizedZone && record.Name == normalizedName && record.Type == change.Type
	})
	return nil
}

func applyDeleteNamePreview(state *previewZoneState, zone string, change Change) error {
	normalizedZone, normalizedName, err := normalizeRRSetLookup(zone, change.Name)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "apply_change_preview", "zone", zone, "kind", change.Kind, "name", change.Name).
			Wrapf(err, "normalize preview name lookup")
	}

	state.records = removeRecords(state.records, func(record Record) bool {
		return record.Zone == normalizedZone && record.Name == normalizedName
	})
	return nil
}
