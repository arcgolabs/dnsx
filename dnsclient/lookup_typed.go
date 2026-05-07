package dnsclient

import (
	"context"
	"strings"

	"github.com/miekg/dns"
	"github.com/samber/lo"
)

type MXRecord struct {
	Host       string `json:"host"`
	Preference uint16 `json:"preference"`
}

type SRVRecord struct {
	Target   string `json:"target"`
	Port     uint16 `json:"port"`
	Priority uint16 `json:"priority"`
	Weight   uint16 `json:"weight"`
}

func (c *Client) LookupAAAA(ctx context.Context, name string) ([]string, error) {
	answer, err := c.Lookup(ctx, name, dns.TypeAAAA)
	if err != nil {
		return nil, err
	}

	return lo.FilterMap(answer, func(rr dns.RR, _ int) (string, bool) {
		record, ok := rr.(*dns.AAAA)
		if !ok {
			return "", false
		}

		return record.AAAA.String(), true
	}), nil
}

func (c *Client) LookupCNAME(ctx context.Context, name string) ([]string, error) {
	answer, err := c.Lookup(ctx, name, dns.TypeCNAME)
	if err != nil {
		return nil, err
	}

	return lo.FilterMap(answer, func(rr dns.RR, _ int) (string, bool) {
		record, ok := rr.(*dns.CNAME)
		if !ok {
			return "", false
		}

		return record.Target, true
	}), nil
}

func (c *Client) LookupNS(ctx context.Context, name string) ([]string, error) {
	answer, err := c.Lookup(ctx, name, dns.TypeNS)
	if err != nil {
		return nil, err
	}

	return lo.FilterMap(answer, func(rr dns.RR, _ int) (string, bool) {
		record, ok := rr.(*dns.NS)
		if !ok {
			return "", false
		}

		return record.Ns, true
	}), nil
}

func (c *Client) LookupMX(ctx context.Context, name string) ([]MXRecord, error) {
	answer, err := c.Lookup(ctx, name, dns.TypeMX)
	if err != nil {
		return nil, err
	}

	return lo.FilterMap(answer, func(rr dns.RR, _ int) (MXRecord, bool) {
		record, ok := rr.(*dns.MX)
		if !ok {
			return MXRecord{}, false
		}

		return MXRecord{Host: record.Mx, Preference: record.Preference}, true
	}), nil
}

func (c *Client) LookupTXT(ctx context.Context, name string) ([]string, error) {
	answer, err := c.Lookup(ctx, name, dns.TypeTXT)
	if err != nil {
		return nil, err
	}

	return lo.FilterMap(answer, func(rr dns.RR, _ int) (string, bool) {
		record, ok := rr.(*dns.TXT)
		if !ok {
			return "", false
		}

		return strings.Join(record.Txt, ""), true
	}), nil
}

func (c *Client) LookupSRV(ctx context.Context, name string) ([]SRVRecord, error) {
	answer, err := c.Lookup(ctx, name, dns.TypeSRV)
	if err != nil {
		return nil, err
	}

	return lo.FilterMap(answer, func(rr dns.RR, _ int) (SRVRecord, bool) {
		record, ok := rr.(*dns.SRV)
		if !ok {
			return SRVRecord{}, false
		}

		return SRVRecord{
			Target:   record.Target,
			Port:     record.Port,
			Priority: record.Priority,
			Weight:   record.Weight,
		}, true
	}), nil
}
