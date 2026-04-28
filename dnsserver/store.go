package dnsserver

import "context"

type RecordFilter struct {
	Zone  string `json:"zone,omitempty"`
	Name  string `json:"name,omitempty"`
	Type  uint16 `json:"type,omitempty"`
	Class uint16 `json:"class,omitempty"`
}

type Repository interface {
	SaveZone(ctx context.Context, zone Zone) error
	DeleteZone(ctx context.Context, zone string) error
	ListZones(ctx context.Context) ([]Zone, error)
	SaveRecord(ctx context.Context, record Record) error
	DeleteRecord(ctx context.Context, record Record) error
	ListRecords(ctx context.Context, filter RecordFilter) ([]Record, error)
	Lookup(ctx context.Context, zone string, name string, qtype uint16, qclass uint16) ([]Record, error)
	LookupAll(ctx context.Context, zone string, name string, qclass uint16) ([]Record, error)
}

type Revisioner interface {
	Revision() uint64
}
