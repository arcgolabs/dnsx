package dnsserver

import (
	"strings"

	"github.com/arcgolabs/collectionx/prefix"
	"github.com/miekg/dns"
	"github.com/samber/mo"
)

type zoneIndex struct {
	names []string
	trie  *prefix.Trie[string]
}

func newZoneIndex(zones []Zone) zoneIndex {
	names := uniqueSortedZoneNames(zones)
	trie := prefix.NewTrie[string]()
	for _, name := range names {
		trie.Put(reverseDNSName(name), name)
	}

	return zoneIndex{
		names: names,
		trie:  trie,
	}
}

func (index zoneIndex) Match(name string) mo.Option[string] {
	if index.trie == nil || len(index.names) == 0 {
		return mo.None[string]()
	}

	_, zone, ok := index.trie.LongestPrefix(reverseDNSName(name))
	if !ok {
		return mo.None[string]()
	}
	if !dns.IsSubDomain(zone, name) {
		return mo.None[string]()
	}

	return mo.Some(zone)
}

func reverseDNSName(name string) string {
	trimmed := strings.TrimSuffix(name, ".")
	if trimmed == "" {
		return ""
	}

	labels := strings.Split(trimmed, ".")
	for left, right := 0, len(labels)-1; left < right; left, right = left+1, right-1 {
		labels[left], labels[right] = labels[right], labels[left]
	}

	return strings.Join(labels, ".") + "."
}
