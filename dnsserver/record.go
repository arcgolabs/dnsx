package dnsserver

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type Zone struct {
	Name string `json:"name"`
}

type Record struct {
	Zone  string `json:"zone"`
	Name  string `json:"name"`
	TTL   uint32 `json:"ttl"`
	Class uint16 `json:"class"`
	Type  uint16 `json:"type"`
	Data  string `json:"data"`
}

func NormalizeZoneName(name string) (string, error) {
	normalized := dns.Fqdn(strings.TrimSpace(strings.ToLower(name)))
	if normalized == "." {
		return "", oops.In("dnsserver").
			With("op", "normalize_zone_name").
			New("zone name is required")
	}

	return normalized, nil
}

func NormalizeRecord(record Record) (Record, error) {
	zone, err := NormalizeZoneName(record.Zone)
	if err != nil {
		return Record{}, oops.In("dnsserver").
			With("op", "normalize_record", "zone", record.Zone, "name", record.Name, "type", record.Type).
			Wrapf(err, "normalize record zone")
	}

	name := dns.Fqdn(strings.TrimSpace(strings.ToLower(record.Name)))
	if name == "." {
		return Record{}, oops.In("dnsserver").
			With("op", "normalize_record", "zone", zone).
			New("record name is required")
	}
	if !dns.IsSubDomain(zone, name) {
		return Record{}, oops.In("dnsserver").
			With("op", "normalize_record", "zone", zone, "name", name).
			Errorf("record %q is outside zone %q", name, zone)
	}

	record.Zone = zone
	record.Name = name
	record.Class = lo.Ternary(record.Class == 0, uint16(dns.ClassINET), record.Class)
	record.Data = strings.TrimSpace(record.Data)
	if record.Data == "" {
		return Record{}, oops.In("dnsserver").
			With("op", "normalize_record", "zone", zone, "name", name, "type", record.Type).
			New("record data is required")
	}
	if record.Type == 0 {
		return Record{}, oops.In("dnsserver").
			With("op", "normalize_record", "zone", zone, "name", name).
			New("record type is required")
	}

	return record, nil
}

func (r Record) Key() string {
	fingerprint := sha1.Sum([]byte(strings.Join([]string{
		strconv.FormatUint(uint64(r.Class), 10),
		strings.ToLower(r.Data),
	}, "|")))

	return fmt.Sprintf(
		"%s|%s|%05d|%s",
		r.Zone,
		r.Name,
		r.Type,
		hex.EncodeToString(fingerprint[:8]),
	)
}

func RecordPrefix(zone string, name string, qtype uint16) string {
	prefix := fmt.Sprintf("%s|%s|", zone, name)
	if qtype == dns.TypeANY {
		return prefix
	}

	return prefix + fmt.Sprintf("%05d|", qtype)
}

func (r Record) WithName(name string) Record {
	r.Name = name
	return r
}

func (r Record) RR() (dns.RR, error) {
	typeName, ok := dns.TypeToString[r.Type]
	if !ok {
		typeName = strconv.FormatUint(uint64(r.Type), 10)
	}

	className, ok := dns.ClassToString[r.Class]
	if !ok {
		className = strconv.FormatUint(uint64(r.Class), 10)
	}

	return dns.NewRR(fmt.Sprintf("%s %d %s %s %s", r.Name, r.TTL, className, typeName, r.Data))
}

func (r Record) CNAME() string {
	if r.Type != dns.TypeCNAME {
		return ""
	}

	return dns.Fqdn(strings.TrimSpace(strings.ToLower(r.Data)))
}

func RecordFromRR(zone string, rr dns.RR) (Record, error) {
	if rr == nil {
		return Record{}, oops.In("dnsserver").
			With("op", "record_from_rr").
			New("dns rr is nil")
	}

	fields := strings.Fields(rr.String())
	if len(fields) < 5 {
		return Record{}, oops.In("dnsserver").
			With("op", "record_from_rr", "rr", rr.String()).
			Errorf("dns rr %q is missing rdata", rr.String())
	}

	record := Record{
		Zone:  zone,
		Name:  rr.Header().Name,
		TTL:   rr.Header().Ttl,
		Class: rr.Header().Class,
		Type:  rr.Header().Rrtype,
		Data:  strings.Join(fields[4:], " "),
	}

	return NormalizeRecord(record)
}
