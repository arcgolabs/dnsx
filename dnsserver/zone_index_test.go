//nolint:testpackage // Tests cover resolver index internals without expanding public API.
package dnsserver

import "testing"

func TestZoneIndexMatchesLongestSuffix(t *testing.T) {
	t.Parallel()

	index := newZoneIndex([]Zone{
		{Name: "com."},
		{Name: "example.com."},
		{Name: "svc.example.com."},
	})

	zone := index.Match("api.svc.example.com.")
	if zone.IsAbsent() {
		t.Fatal("expected matching zone")
	}
	if got := zone.MustGet(); got != "svc.example.com." {
		t.Fatalf("expected longest zone match, got %q", got)
	}
}

func TestZoneIndexMissesUnrelatedName(t *testing.T) {
	t.Parallel()

	index := newZoneIndex([]Zone{{Name: "example.com."}})
	if zone := index.Match("example.org."); zone.IsPresent() {
		t.Fatalf("expected no zone match, got %q", zone.MustGet())
	}
}

func TestZoneIndexMatchesRootZone(t *testing.T) {
	t.Parallel()

	index := newZoneIndex([]Zone{{Name: "."}})
	zone := index.Match("example.org.")
	if zone.IsAbsent() {
		t.Fatal("expected root zone match")
	}
	if got := zone.MustGet(); got != "." {
		t.Fatalf("expected root zone, got %q", got)
	}
}
