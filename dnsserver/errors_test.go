//nolint:testpackage // Tests validate internal error helpers without exporting extra API.
package dnsserver

import (
	"context"
	"errors"
	"testing"

	"github.com/miekg/dns"
)

func TestNormalizeZoneNameReturnsCodedSentinel(t *testing.T) {
	t.Parallel()

	_, err := NormalizeZoneName("")
	if !errors.Is(err, ErrZoneNameRequired) {
		t.Fatalf("expected ErrZoneNameRequired, got %v", err)
	}
	if !HasErrorCode(err, CodeZoneNameRequired) {
		t.Fatalf("expected error code %q, got %v", CodeZoneNameRequired, err)
	}
}

func TestNormalizeRecordReturnsCodedSentinel(t *testing.T) {
	t.Parallel()

	_, err := NormalizeRecord(Record{
		Zone: "example.com",
		Name: "www.other.com",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.1",
	})
	if !errors.Is(err, ErrRecordOutOfZone) {
		t.Fatalf("expected ErrRecordOutOfZone, got %v", err)
	}
	if !HasErrorCode(err, CodeRecordOutOfZone) {
		t.Fatalf("expected error code %q, got %v", CodeRecordOutOfZone, err)
	}
}

func TestManagerRequiresRepositoryWithCodedSentinel(t *testing.T) {
	t.Parallel()

	manager := NewManager(nil)
	_, err := manager.UpsertZone(context.Background(), Zone{Name: "example.com"})
	if !errors.Is(err, ErrRepositoryNotConfigured) {
		t.Fatalf("expected ErrRepositoryNotConfigured, got %v", err)
	}
	if !HasErrorCode(err, CodeRepositoryNotConfigured) {
		t.Fatalf("expected error code %q, got %v", CodeRepositoryNotConfigured, err)
	}
}

func TestServerStartDuplicateReturnsCodedSentinel(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	server := NewServerWithRepository(Config{Listen: "127.0.0.1:0"}, store)
	ctx := context.Background()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Fatalf("stop server: %v", err)
		}
	})

	err := server.Start(ctx)
	if !errors.Is(err, ErrServerAlreadyStarted) {
		t.Fatalf("expected ErrServerAlreadyStarted, got %v", err)
	}
	if !HasErrorCode(err, CodeServerAlreadyStarted) {
		t.Fatalf("expected error code %q, got %v", CodeServerAlreadyStarted, err)
	}
}
