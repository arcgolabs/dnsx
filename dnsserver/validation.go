package dnsserver

import (
	"github.com/arcgolabs/collectionx/set"
	"github.com/miekg/dns"
	"github.com/samber/lo"
)

type zoneValidationOptions struct {
	requireApexNS bool
}

func validateZoneRecords(zone string, records []Record, opts zoneValidationOptions) error {
	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}

	if err := validateSOAPlacement(normalizedZone, records); err != nil {
		return err
	}
	apexSOARecords := filterRecords(records, func(record Record) bool {
		return record.Name == normalizedZone && record.Type == dns.TypeSOA
	})
	if err := validateSOACount(normalizedZone, apexSOARecords); err != nil {
		return err
	}
	if err := validateApexNSRequirement(normalizedZone, records, apexSOARecords, opts); err != nil {
		return err
	}
	return validateCNAMEConflicts(normalizedZone, records)
}

func validateRecordsByZone(records []Record, opts zoneValidationOptions) error {
	zones := set.NewOrderedSetWithCapacity[string](len(records))
	for _, record := range records {
		zones.Add(record.Zone)
	}

	for _, zone := range zones.Values() {
		zoneRecords := lo.Filter(records, func(record Record, _ int) bool {
			return record.Zone == zone
		})
		if err := validateZoneRecords(zone, zoneRecords, opts); err != nil {
			return err
		}
	}

	return nil
}

func normalizeChangeZone(change Change) (string, error) {
	switch {
	case change.Zone.Name != "":
		return NormalizeZoneName(change.Zone.Name)
	case change.ZoneName != "":
		return NormalizeZoneName(change.ZoneName)
	case change.Record.Zone != "":
		return NormalizeZoneName(change.Record.Zone)
	case len(change.Records) > 0 && change.Records[0].Zone != "":
		return NormalizeZoneName(change.Records[0].Zone)
	default:
		return "", errorBuilder("normalize_change_zone", CodeZoneNameRequired, "kind", change.Kind).
			Wrapf(ErrZoneNameRequired, "change %q is missing a zone", change.Kind)
	}
}

func removeRecords(records []Record, predicate func(Record) bool) []Record {
	return lo.Reject(records, func(record Record, _ int) bool {
		return predicate(record)
	})
}

func upsertRecords(records, replacements []Record) []Record {
	keys := set.NewOrderedSetWithCapacity[string](len(records) + len(replacements))
	merged := make([]Record, 0, len(records)+len(replacements))

	for _, record := range append(records, replacements...) {
		if keys.Contains(record.Key()) {
			continue
		}
		keys.Add(record.Key())
		merged = append(merged, record)
	}

	return merged
}

func filterRecords(records []Record, predicate func(Record) bool) []Record {
	return lo.Filter(records, func(record Record, _ int) bool {
		return predicate(record)
	})
}

func validateSOAPlacement(zone string, records []Record) error {
	for _, record := range records {
		if record.Type != dns.TypeSOA || record.Name == zone {
			continue
		}

		return errorBuilder("validate_zone_records", CodeZoneSOANotAtApex, "zone", zone, "name", record.Name).
			Wrapf(ErrSOAMustBeAtZoneApex, "soa record %q must live at zone apex %q", record.Name, zone)
	}

	return nil
}

func validateSOACount(zone string, apexSOARecords []Record) error {
	if len(apexSOARecords) <= 1 {
		return nil
	}

	return errorBuilder("validate_zone_records", CodeZoneSOARecordCountInvalid, "zone", zone, "records", len(apexSOARecords)).
		Wrapf(ErrSOARecordCountInvalid, "zone %q must have at most one SOA record", zone)
}

func validateApexNSRequirement(
	zone string,
	records []Record,
	apexSOARecords []Record,
	opts zoneValidationOptions,
) error {
	if !opts.requireApexNS || len(apexSOARecords) == 0 {
		return nil
	}

	apexNSRecords := filterRecords(records, func(record Record) bool {
		return record.Name == zone && record.Type == dns.TypeNS
	})
	if len(apexNSRecords) > 0 {
		return nil
	}

	return errorBuilder("validate_zone_records", CodeZoneApexNSRequired, "zone", zone).
		Wrapf(ErrApexNSRequired, "zone %q must define at least one apex NS record", zone)
}

func validateCNAMEConflicts(zone string, records []Record) error {
	for name, nameRecords := range lo.GroupBy(records, func(record Record) string {
		return record.Name
	}) {
		cnameRecords := filterRecords(nameRecords, func(record Record) bool {
			return record.Type == dns.TypeCNAME
		})
		if err := validateCNAMECount(zone, name, cnameRecords); err != nil {
			return err
		}
		if err := validateCNAMECoexistence(zone, name, nameRecords, cnameRecords); err != nil {
			return err
		}
	}

	return nil
}

func validateCNAMECount(zone, name string, cnameRecords []Record) error {
	if len(cnameRecords) <= 1 {
		return nil
	}

	return errorBuilder("validate_zone_records", CodeZoneCNAMERecordCountInvalid, "zone", zone, "name", name, "records", len(cnameRecords)).
		Wrapf(ErrCNAMERecordCountInvalid, "name %q must have at most one CNAME record", name)
}

func validateCNAMECoexistence(zone, name string, nameRecords, cnameRecords []Record) error {
	if len(cnameRecords) == 0 || len(nameRecords) == len(cnameRecords) {
		return nil
	}

	return errorBuilder("validate_zone_records", CodeZoneCNAMEConflict, "zone", zone, "name", name).
		Wrapf(ErrCNAMEConflict, "name %q cannot mix CNAME with other record types", name)
}
