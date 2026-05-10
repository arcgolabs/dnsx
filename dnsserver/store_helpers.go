package dnsserver

import (
	"strings"

	"github.com/miekg/dns"
	"github.com/samber/oops"
)

func NormalizeLookup(zone, name string) (string, string, error) {
	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return "", "", oops.In("dnsserver").
			With("op", "normalize_lookup", "zone", zone, "name", name).
			Wrapf(err, "normalize lookup zone")
	}

	normalizedName := dns.Fqdn(strings.TrimSpace(strings.ToLower(name)))
	if !dns.IsSubDomain(normalizedZone, normalizedName) {
		return "", "", oops.In("dnsserver").
			With("op", "normalize_lookup", "zone", normalizedZone, "name", normalizedName).
			Errorf("lookup name %q is outside zone %q", normalizedName, normalizedZone)
	}

	return normalizedZone, normalizedName, nil
}

func CompareRecords(left, right Record) int {
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
	case left.Data < right.Data:
		return -1
	case left.Data > right.Data:
		return 1
	default:
		return 0
	}
}

func compareRecords(left, right Record) int {
	return CompareRecords(left, right)
}

func CompareRecordsByData(left, right Record) int {
	switch {
	case left.Data < right.Data:
		return -1
	case left.Data > right.Data:
		return 1
	default:
		return 0
	}
}

func compareRecordsByData(left, right Record) int {
	return CompareRecordsByData(left, right)
}
