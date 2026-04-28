package dnsserver

import (
	"fmt"
	"slices"

	"github.com/samber/lo"
)

type RRSet struct {
	Zone    string   `json:"zone"`
	Name    string   `json:"name"`
	Class   uint16   `json:"class"`
	Type    uint16   `json:"type"`
	Records []Record `json:"records"`
}

type ZoneSnapshot struct {
	Zone    Zone     `json:"zone"`
	Records []Record `json:"records"`
	RRSets  []RRSet  `json:"rrsets"`
}

func (snapshot ZoneSnapshot) Validate() error {
	return validateZoneRecords(snapshot.Zone.Name, snapshot.Records, zoneValidationOptions{requireApexNS: true})
}

func buildZoneSnapshot(zone string, records []Record) ZoneSnapshot {
	snapshotRecords := lo.Map(records, func(record Record, _ int) Record {
		return record
	})
	slices.SortFunc(snapshotRecords, compareRecords)

	return ZoneSnapshot{
		Zone:    Zone{Name: zone},
		Records: snapshotRecords,
		RRSets:  buildRRSets(snapshotRecords),
	}
}

func buildRRSets(records []Record) []RRSet {
	if len(records) == 0 {
		return nil
	}

	rrsetsByKey := make(map[string]*RRSet, len(records))
	for _, record := range records {
		key := rrsetKey(record)
		rrset, ok := rrsetsByKey[key]
		if !ok {
			rrset = &RRSet{
				Zone:  record.Zone,
				Name:  record.Name,
				Class: record.Class,
				Type:  record.Type,
			}
			rrsetsByKey[key] = rrset
		}

		rrset.Records = append(rrset.Records, record)
	}

	rrsets := lo.Map(lo.Values(rrsetsByKey), func(rrset *RRSet, _ int) RRSet {
		snapshotRRSet := RRSet{
			Zone:  rrset.Zone,
			Name:  rrset.Name,
			Class: rrset.Class,
			Type:  rrset.Type,
			Records: lo.Map(rrset.Records, func(record Record, _ int) Record {
				return record
			}),
		}
		slices.SortFunc(snapshotRRSet.Records, compareRecordsByData)
		return snapshotRRSet
	})

	slices.SortFunc(rrsets, compareRRSets)
	return rrsets
}

func rrsetKey(record Record) string {
	return fmt.Sprintf("%s|%s|%d|%d", record.Zone, record.Name, record.Class, record.Type)
}

func compareRRSets(left, right RRSet) int {
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
	case left.Class < right.Class:
		return -1
	case left.Class > right.Class:
		return 1
	default:
		return 0
	}
}
