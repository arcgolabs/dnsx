package dnsserver

import (
	"strings"

	"github.com/miekg/dns"
	"github.com/samber/oops"
)

func normalizeRecordFilter(filter RecordFilter) (RecordFilter, error) {
	normalized := filter
	if normalized.Zone != "" {
		zone, err := NormalizeZoneName(normalized.Zone)
		if err != nil {
			return RecordFilter{}, oops.In("dnsserver").
				With("op", "normalize_record_filter", "zone", filter.Zone, "name", filter.Name).
				Wrapf(err, "normalize filter zone")
		}
		normalized.Zone = zone
	}

	if normalized.Name != "" {
		normalized.Name = dns.Fqdn(strings.TrimSpace(strings.ToLower(normalized.Name)))
		if normalized.Name == "." {
			return RecordFilter{}, oops.In("dnsserver").
				With("op", "normalize_record_filter", "zone", normalized.Zone).
				New("record filter name is required")
		}
	}

	if normalized.Zone != "" && normalized.Name != "" && !dns.IsSubDomain(normalized.Zone, normalized.Name) {
		return RecordFilter{}, oops.In("dnsserver").
			With("op", "normalize_record_filter", "zone", normalized.Zone, "name", normalized.Name).
			Errorf("record filter %q is outside zone %q", normalized.Name, normalized.Zone)
	}

	if normalized.Type == dns.TypeANY {
		normalized.Type = 0
	}
	if normalized.Class == dns.ClassANY {
		normalized.Class = 0
	}

	return normalized, nil
}

func recordListPrefix(filter RecordFilter) string {
	switch {
	case filter.Zone == "":
		return ""
	case filter.Name == "":
		return filter.Zone + "|"
	case filter.Type == 0:
		return RecordPrefix(filter.Zone, filter.Name, dns.TypeANY)
	default:
		return RecordPrefix(filter.Zone, filter.Name, filter.Type)
	}
}

func recordMatchesFilter(record Record, filter RecordFilter) bool {
	switch {
	case filter.Zone != "" && record.Zone != filter.Zone:
		return false
	case filter.Name != "" && record.Name != filter.Name:
		return false
	case filter.Type != 0 && record.Type != filter.Type:
		return false
	case filter.Class != 0 && record.Class != filter.Class:
		return false
	default:
		return true
	}
}
