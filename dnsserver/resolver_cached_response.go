package dnsserver

import (
	"github.com/miekg/dns"
	"github.com/samber/oops"
)

func (response cachedResponse) materialize() (Resolution, error) {
	answer, err := parseRRStrings(response.Answer)
	if err != nil {
		return Resolution{}, err
	}

	authority, err := parseRRStrings(response.Authority)
	if err != nil {
		return Resolution{}, err
	}

	extra, err := parseRRStrings(response.Extra)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{
		RCode:         response.RCode,
		Answer:        answer,
		Authority:     authority,
		Extra:         extra,
		Authoritative: response.Authoritative,
	}, nil
}

func parseRRStrings(lines []string) ([]dns.RR, error) {
	result := make([]dns.RR, 0, len(lines))
	for _, line := range lines {
		rr, err := dns.NewRR(line)
		if err != nil {
			return nil, oops.In("dnsserver").
				With("op", "parse_rr_strings", "line", line).
				Wrapf(err, "parse cached rr")
		}

		result = append(result, rr)
	}

	return result, nil
}
