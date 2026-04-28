package dnsserver

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"sync/atomic"

	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/samber/oops"
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
		return nil, oops.In("dnsserver").
			With("op", "open_bbolt_store", "path", path).
			Wrapf(err, "open bbolt store")
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

	return oops.In("dnsserver").
		With("op", "close_bbolt_store").
		Wrapf(s.db.Close(), "close bbolt store")
}

func (s *BboltStore) SaveZone(ctx context.Context, zone Zone) error {
	normalized, err := NormalizeZoneName(zone.Name)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "save_zone", "zone", zone.Name).
			Wrapf(err, "normalize zone")
	}

	if err := s.zones.Put(ctx, normalized, Zone{Name: normalized}); err != nil {
		return oops.In("dnsserver").
			With("op", "save_zone", "zone", normalized).
			Wrapf(err, "save zone")
	}

	s.revision.Add(1)
	return nil
}

func (s *BboltStore) DeleteZone(ctx context.Context, zone string) error {
	normalized, err := NormalizeZoneName(zone)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "delete_zone", "zone", zone).
			Wrapf(err, "normalize zone")
	}

	if err := s.zones.Delete(ctx, normalized); err != nil {
		return oops.In("dnsserver").
			With("op", "delete_zone", "zone", normalized).
			Wrapf(err, "delete zone")
	}

	prefix := normalized + "|"
	if err := s.records.Update(ctx, func(tx bboltx.UpdateTx[string, Record]) error {
		var keys []string
		if err := tx.ScanPrefix([]byte(prefix), func(key string, _ Record) error {
			keys = append(keys, key)
			return nil
		}); err != nil {
			return oops.In("dnsserver").
				With("op", "delete_zone_records_scan", "zone", normalized).
				Wrapf(err, "scan zone records")
		}

		for _, key := range keys {
			if err := tx.Delete(key); err != nil {
				return oops.In("dnsserver").
					With("op", "delete_zone_record", "zone", normalized, "key", key).
					Wrapf(err, "delete zone record")
			}
		}

		return nil
	}); err != nil {
		return oops.In("dnsserver").
			With("op", "delete_zone_records", "zone", normalized).
			Wrapf(err, "delete zone records")
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
		return nil, oops.In("dnsserver").
			With("op", "list_zones").
			Wrapf(err, "list zones")
	}

	sort.Slice(zones, func(i int, j int) bool {
		return zones[i].Name < zones[j].Name
	})

	return zones, nil
}

func (s *BboltStore) SaveRecord(ctx context.Context, record Record) error {
	normalized, err := NormalizeRecord(record)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "save_record", "zone", record.Zone, "name", record.Name, "type", record.Type).
			Wrapf(err, "normalize record")
	}

	if err := s.SaveZone(ctx, Zone{Name: normalized.Zone}); err != nil {
		return err
	}

	if err := s.records.Put(ctx, normalized.Key(), normalized); err != nil {
		return oops.In("dnsserver").
			With("op", "save_record", "key", normalized.Key()).
			Wrapf(err, "save record")
	}

	s.revision.Add(1)
	return nil
}

func (s *BboltStore) DeleteRecord(ctx context.Context, record Record) error {
	normalized, err := NormalizeRecord(record)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "delete_record", "zone", record.Zone, "name", record.Name, "type", record.Type).
			Wrapf(err, "normalize record")
	}

	if err := s.records.Delete(ctx, normalized.Key()); err != nil {
		return oops.In("dnsserver").
			With("op", "delete_record", "key", normalized.Key()).
			Wrapf(err, "delete record")
	}

	s.revision.Add(1)
	return nil
}

var _ Repository = (*BboltStore)(nil)
var _ Revisioner = (*BboltStore)(nil)
var _ interface{ Close() error } = (*BboltStore)(nil)

func MustOpenBboltStore(path string) *BboltStore {
	store, err := OpenBboltStore(path, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err != nil {
		panic(oops.In("dnsserver").
			With("op", "must_open_bbolt_store", "path", path).
			Wrapf(err, "open bbolt store"))
	}

	return store
}
