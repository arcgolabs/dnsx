package dnsserver

import (
	"context"
	"encoding/json"
	"os"
	"strconv"

	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/miekg/dns"
	"github.com/samber/oops"
)

type SeedData struct {
	Zones   []Zone   `json:"zones"`
	Records []Record `json:"records"`
}

func LoadSeedData(path string) (SeedData, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return SeedData{}, oops.In("dnsserver").
			With("op", "load_seed_data", "path", path).
			Wrapf(err, "read seed data")
	}

	var seed SeedData
	if err := json.Unmarshal(content, &seed); err != nil {
		return SeedData{}, oops.In("dnsserver").
			With("op", "load_seed_data", "path", path).
			Wrapf(err, "decode seed data")
	}

	return seed, nil
}

func ApplySeedData(ctx context.Context, repo Repository, seed SeedData) error {
	zoneNames := collectionset.NewOrderedSet[string]()
	records := make([]Record, 0, len(seed.Records))
	recordKeys := collectionset.NewOrderedSet[string]()

	for _, zone := range seed.Zones {
		normalized, err := NormalizeZoneName(zone.Name)
		if err != nil {
			return oops.In("dnsserver").
				With("op", "apply_seed_data", "section", "zones", "zone", zone.Name).
				Wrapf(err, "normalize seed zone")
		}

		zoneNames.Add(normalized)
	}

	for _, record := range seed.Records {
		normalized, err := NormalizeRecord(record)
		if err != nil {
			return oops.In("dnsserver").
				With("op", "apply_seed_data", "section", "records", "zone", record.Zone, "name", record.Name, "type", record.Type).
				Wrapf(err, "normalize seed record")
		}

		zoneNames.Add(normalized.Zone)
		if !recordKeys.Contains(normalized.Key()) {
			recordKeys.Add(normalized.Key())
			records = append(records, normalized)
		}
	}

	for _, zoneName := range zoneNames.Values() {
		if err := repo.SaveZone(ctx, Zone{Name: zoneName}); err != nil {
			return oops.In("dnsserver").
				With("op", "apply_seed_data", "section", "zones", "zone", zoneName).
				Wrapf(err, "save seed zone")
		}
	}

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
