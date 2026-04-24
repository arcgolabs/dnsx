package dnsserver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"sync/atomic"

	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/miekg/dns"
)

const (
	zonesBucketName   = "dnsx.zones"
	recordsBucketName = "dnsx.records"
)

type BboltStore struct {
	db       *bboltx.DB
	zones    *bboltx.Bucket[string, Zone]
	records  *bboltx.Bucket[string, Record]
	closed   atomic.Bool
	revision atomic.Uint64
}

func OpenBboltStore(path string, logger *slog.Logger) (*BboltStore, error) {
	db, err := bboltx.Open(path, 0o600, nil, bboltx.WithDBLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("open bbolt store: %w", err)
	}

	return NewBboltStore(db), nil
}

func NewBboltStore(db *bboltx.DB) *BboltStore {
	return &BboltStore{
		db: db,
		zones: bboltx.NewBucketWithDB(
			db,
			zonesBucketName,
			keycodec.String(),
			codec.JSON[Zone](),
		),
		records: bboltx.NewBucketWithDB(
			db,
			recordsBucketName,
			keycodec.String(),
			codec.JSON[Record](),
		),
	}
}

func (s *BboltStore) Revision() uint64 {
	if s == nil {
		return 0
	}

	return s.revision.Load()
}

func (s *BboltStore) Close() error {
	if s == nil || s.closed.Swap(true) {
		return nil
	}

	return s.db.Close()
}

func (s *BboltStore) SaveZone(ctx context.Context, zone Zone) error {
	normalized, err := NormalizeZoneName(zone.Name)
	if err != nil {
		return fmt.Errorf("save zone: %w", err)
	}

	if err := s.zones.Put(ctx, normalized, Zone{Name: normalized}); err != nil {
		return fmt.Errorf("save zone %q: %w", normalized, err)
	}

	s.revision.Add(1)
	return nil
}

func (s *BboltStore) DeleteZone(ctx context.Context, zone string) error {
	normalized, err := NormalizeZoneName(zone)
	if err != nil {
		return fmt.Errorf("delete zone: %w", err)
	}

	if err := s.zones.Delete(ctx, normalized); err != nil {
		return fmt.Errorf("delete zone %q: %w", normalized, err)
	}

	prefix := normalized + "|"
	if err := s.records.Update(ctx, func(tx bboltx.UpdateTx[string, Record]) error {
		var keys []string
		if err := tx.ScanPrefix([]byte(prefix), func(key string, _ Record) error {
			keys = append(keys, key)
			return nil
		}); err != nil {
			return err
		}

		for _, key := range keys {
			if err := tx.Delete(key); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("delete zone records %q: %w", normalized, err)
	}

	s.revision.Add(1)
	return nil
}

func (s *BboltStore) ListZones(ctx context.Context) ([]Zone, error) {
	zones := make([]Zone, 0)
	if err := s.zones.View(ctx, func(tx bboltx.ViewTx[string, Zone]) error {
		return tx.ForEach(func(_ string, zone Zone) error {
			zones = append(zones, zone)
			return nil
		})
	}); err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}

	sort.Slice(zones, func(i int, j int) bool {
		return zones[i].Name < zones[j].Name
	})

	return zones, nil
}

func (s *BboltStore) SaveRecord(ctx context.Context, record Record) error {
	normalized, err := NormalizeRecord(record)
	if err != nil {
		return fmt.Errorf("save record: %w", err)
	}

	if err := s.SaveZone(ctx, Zone{Name: normalized.Zone}); err != nil {
		return err
	}

	if err := s.records.Put(ctx, normalized.Key(), normalized); err != nil {
		return fmt.Errorf("save record %q: %w", normalized.Key(), err)
	}

	s.revision.Add(1)
	return nil
}

func (s *BboltStore) DeleteRecord(ctx context.Context, record Record) error {
	normalized, err := NormalizeRecord(record)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}

	if err := s.records.Delete(ctx, normalized.Key()); err != nil {
		return fmt.Errorf("delete record %q: %w", normalized.Key(), err)
	}

	s.revision.Add(1)
	return nil
}

func (s *BboltStore) Lookup(ctx context.Context, zone string, name string, qtype uint16, qclass uint16) ([]Record, error) {
	normalizedZone, normalizedName, err := normalizeLookup(zone, name)
	if err != nil {
		return nil, err
	}

	return s.lookupByPrefix(ctx, RecordPrefix(normalizedZone, normalizedName, qtype), qclass)
}

func (s *BboltStore) LookupAll(ctx context.Context, zone string, name string, qclass uint16) ([]Record, error) {
	normalizedZone, normalizedName, err := normalizeLookup(zone, name)
	if err != nil {
		return nil, err
	}

	return s.lookupByPrefix(ctx, RecordPrefix(normalizedZone, normalizedName, dns.TypeANY), qclass)
}

func (s *BboltStore) lookupByPrefix(ctx context.Context, prefix string, qclass uint16) ([]Record, error) {
	records := make([]Record, 0)
	if err := s.records.View(ctx, func(tx bboltx.ViewTx[string, Record]) error {
		return tx.ScanPrefix([]byte(prefix), func(_ string, record Record) error {
			if qclass != dns.ClassANY && record.Class != qclass {
				return nil
			}

			records = append(records, record)
			return nil
		})
	}); err != nil {
		return nil, fmt.Errorf("lookup records by prefix %q: %w", prefix, err)
	}

	slices.SortFunc(records, func(left Record, right Record) int {
		switch {
		case left.Data < right.Data:
			return -1
		case left.Data > right.Data:
			return 1
		default:
			return 0
		}
	})

	return records, nil
}

func normalizeLookup(zone string, name string) (string, string, error) {
	normalizedZone, err := NormalizeZoneName(zone)
	if err != nil {
		return "", "", fmt.Errorf("normalize lookup zone: %w", err)
	}

	normalizedName := dns.Fqdn(name)
	if !dns.IsSubDomain(normalizedZone, normalizedName) {
		return "", "", fmt.Errorf("lookup name %q is outside zone %q", normalizedName, normalizedZone)
	}

	return normalizedZone, normalizedName, nil
}

var _ Repository = (*BboltStore)(nil)
var _ Revisioner = (*BboltStore)(nil)
var _ interface{ Close() error } = (*BboltStore)(nil)

func MustOpenBboltStore(path string) *BboltStore {
	store, err := OpenBboltStore(path, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err != nil {
		panic(err)
	}

	return store
}
