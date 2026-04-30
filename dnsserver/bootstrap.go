package dnsserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/arcgolabs/collectionx/set"
	"github.com/miekg/dns"
	"github.com/samber/oops"
)

type SeedData struct {
	Zones   []Zone   `json:"zones"`
	Records []Record `json:"records"`
}

func LoadSeedData(path string) (SeedData, error) {
	cleanPath := filepath.Clean(path)
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return SeedData{}, oops.In("dnsserver").
			With("op", "load_seed_data", "path", cleanPath).
			Wrapf(err, "read seed data")
	}

	var seed SeedData
	if err := json.Unmarshal(content, &seed); err != nil {
		return SeedData{}, oops.In("dnsserver").
			With("op", "load_seed_data", "path", cleanPath).
			Wrapf(err, "decode seed data")
	}

	return seed, nil
}

func ApplySeedData(ctx context.Context, repo Repository, seed SeedData) error {
	zoneNames, err := collectSeedZones(seed)
	if err != nil {
		return err
	}

	records, err := collectSeedRecords(seed, zoneNames)
	if err != nil {
		return err
	}
	if err := validateRecordsByZone(records, zoneValidationOptions{requireApexNS: true}); err != nil {
		return oops.In("dnsserver").
			With("op", "apply_seed_data", "records", len(records)).
			Wrapf(err, "validate seed records")
	}

	if err := saveSeedZones(ctx, repo, zoneNames.Values()); err != nil {
		return err
	}

	return saveSeedRecords(ctx, repo, records)
}

func collectSeedZones(seed SeedData) (*set.OrderedSet[string], error) {
	zoneNames := set.NewOrderedSetWithCapacity[string](len(seed.Zones))

	for _, zone := range seed.Zones {
		normalized, err := NormalizeZoneName(zone.Name)
		if err != nil {
			return nil, oops.In("dnsserver").
				With("op", "apply_seed_data", "section", "zones", "zone", zone.Name).
				Wrapf(err, "normalize seed zone")
		}

		zoneNames.Add(normalized)
	}

	return zoneNames, nil
}

func collectSeedRecords(seed SeedData, zoneNames *set.OrderedSet[string]) ([]Record, error) {
	records := make([]Record, 0, len(seed.Records))
	recordKeys := set.NewOrderedSetWithCapacity[string](len(seed.Records))

	for _, record := range seed.Records {
		normalized, err := NormalizeRecord(record)
		if err != nil {
			return nil, oops.In("dnsserver").
				With("op", "apply_seed_data", "section", "records", "zone", record.Zone, "name", record.Name, "type", record.Type).
				Wrapf(err, "normalize seed record")
		}

		zoneNames.Add(normalized.Zone)
		if !recordKeys.Contains(normalized.Key()) {
			recordKeys.Add(normalized.Key())
			records = append(records, normalized)
		}
	}

	return records, nil
}

func saveSeedZones(ctx context.Context, repo Repository, zones []string) error {
	for _, zoneName := range zones {
		if err := repo.SaveZone(ctx, Zone{Name: zoneName}); err != nil {
			return oops.In("dnsserver").
				With("op", "apply_seed_data", "section", "zones", "zone", zoneName).
				Wrapf(err, "save seed zone")
		}
	}

	return nil
}

func saveSeedRecords(ctx context.Context, repo Repository, records []Record) error {
	for _, record := range records {
		if err := repo.SaveRecord(ctx, record); err != nil {
			return oops.In("dnsserver").
				With(
					"op", "apply_seed_data",
					"section", "records",
					"zone", record.Zone,
					"name", record.Name,
					"type", dnsTypeName(record.Type),
				).
				Wrapf(err, "save seed record")
		}
	}

	return nil
}

func dnsTypeName(rrtype uint16) string {
	if name := dns.TypeToString[rrtype]; name != "" {
		return name
	}

	return strconv.FormatUint(uint64(rrtype), 10)
}
